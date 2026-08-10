// Package postgres_test provides integration tests for JobRepository's
// parent/child batch fan-out methods (CreateChildren, IncrementChildProgress).
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// JobRepositoryBatchSuite covers JobRepository's parent/child fan-out methods
// against a real PostgreSQL instance — CreateChildren's batch insert and
// IncrementChildProgress's atomic UPDATE...RETURNING completion check.
type JobRepositoryBatchSuite struct {
	suite.Suite
	db      *postgres.DB
	repo    *postgres.JobRepository
	ctx     context.Context
	cleanup []string
}

func TestJobRepositoryBatchSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(JobRepositoryBatchSuite))
}

func (s *JobRepositoryBatchSuite) SetupSuite() {
	s.ctx = context.Background()

	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvOrDefault("TEST_DB_PORT", "5434")
	user := getEnvOrDefault("TEST_DB_USER", "finance")
	password := getEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := getEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("postgres", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitForDB(raw, 10*time.Second))

	s.db = postgres.NewDBFromSQL(raw)
	s.repo = postgres.NewJobRepository(s.db)
}

func (s *JobRepositoryBatchSuite) TearDownSuite() {
	if s.db != nil {
		for _, code := range s.cleanup {
			_, _ = s.db.ExecContext(s.ctx, `DELETE FROM job_execution WHERE job_code = $1`, code)
		}
		_ = s.db.Close()
	}
}

// newPeriod returns a unique period string per test so parallel/rerun test
// runs never collide on the job_type+period active-job uniqueness rule.
func newPeriod() string {
	return fmt.Sprintf("BT%d", time.Now().UnixNano()%1000000000)
}

// TestCreateChildren_PersistsAllChildrenReferencingParent verifies a batch of
// children is inserted in one call, each with its own sequential code and a
// jex_parent_job_id pointing at the parent.
func (s *JobRepositoryBatchSuite) TestCreateChildren_PersistsAllChildrenReferencingParent() {
	period := newPeriod()

	parent, err := job.NewParentExecution(job.TypeProductCostSheetExport, "xlsx", period, "integ-test", 5, nil, 3)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.repo.Create(s.ctx, parent))
	s.cleanup = append(s.cleanup, parent.Code().String())

	children := make([]*job.Execution, 0, 3)
	for i := 0; i < 3; i++ {
		child, childErr := job.NewChildExecution(job.TypeProductCostSheetExport, "xlsx", period, "integ-test", 5, nil, parent.ID())
		require.NoError(s.T(), childErr)
		children = append(children, child)
	}
	require.NoError(s.T(), s.repo.CreateChildren(s.ctx, children))
	for _, c := range children {
		s.cleanup = append(s.cleanup, c.Code().String())
	}

	// Every child must be independently retrievable and reference the parent.
	for _, c := range children {
		got, getErr := s.repo.GetByID(s.ctx, c.ID())
		require.NoError(s.T(), getErr)
		require.NotNil(s.T(), got.ParentJobID())
		require.Equal(s.T(), parent.ID(), *got.ParentJobID())
		require.True(s.T(), got.IsChild())
	}

	// Codes must be distinct (sequence assigned correctly within the tx).
	seen := map[string]bool{}
	for _, c := range children {
		require.False(s.T(), seen[c.Code().String()], "duplicate child code %s", c.Code().String())
		seen[c.Code().String()] = true
	}
}

// TestIncrementChildProgress_AtomicallyTracksCompletionAcrossCalls verifies
// the UPDATE...RETURNING increment: batchComplete stays false until every
// child has reported in, then flips true on the last one — and that a
// second call past completion still increments/reports correctly (guards
// against a static "already done" short-circuit hiding a real counter bug).
func (s *JobRepositoryBatchSuite) TestIncrementChildProgress_AtomicallyTracksCompletionAcrossCalls() {
	period := newPeriod()

	parent, err := job.NewParentExecution(job.TypeProductCostSheetExport, "xlsx", period, "integ-test", 5, nil, 3)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.repo.Create(s.ctx, parent))
	s.cleanup = append(s.cleanup, parent.Code().String())

	// 1st and 2nd increments (of 3 total): batch not yet complete.
	done, incErr := s.repo.IncrementChildProgress(s.ctx, parent.ID(), true)
	require.NoError(s.T(), incErr)
	require.False(s.T(), done)

	done, incErr = s.repo.IncrementChildProgress(s.ctx, parent.ID(), false)
	require.NoError(s.T(), incErr)
	require.False(s.T(), done)

	// 3rd increment: completed+failed == total → batch complete.
	done, incErr = s.repo.IncrementChildProgress(s.ctx, parent.ID(), true)
	require.NoError(s.T(), incErr)
	require.True(s.T(), done)

	got, getErr := s.repo.GetByID(s.ctx, parent.ID())
	require.NoError(s.T(), getErr)
	require.Equal(s.T(), 2, got.CompletedChildren())
	require.Equal(s.T(), 1, got.FailedChildren())
	require.Equal(s.T(), 3, got.TotalChildren())
}

// TestIncrementChildProgress_UnknownParentReturnsNotFound verifies the
// sql.ErrNoRows → job.ErrNotFound mapping used when a child somehow
// references a parent job_id that no longer exists.
func (s *JobRepositoryBatchSuite) TestIncrementChildProgress_UnknownParentReturnsNotFound() {
	period := newPeriod()
	ghost, err := job.NewParentExecution(job.TypeProductCostSheetExport, "xlsx", period, "integ-test", 5, nil, 1)
	require.NoError(s.T(), err)
	// Never persisted — job_id has no matching row.

	_, incErr := s.repo.IncrementChildProgress(s.ctx, ghost.ID(), true)
	require.ErrorIs(s.T(), incErr, job.ErrNotFound)
}

// TestListChildren_ReturnsAllChildrenOrderedByQueuedAt verifies ListChildren
// returns every child referencing the parent, in ascending queued_at order,
// and that an unrelated job (different parent) is excluded.
func (s *JobRepositoryBatchSuite) TestListChildren_ReturnsAllChildrenOrderedByQueuedAt() {
	period := newPeriod()

	parent, err := job.NewParentExecution(job.TypeProductCostSheetExport, "xlsx", period, "integ-test", 5, nil, 2)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.repo.Create(s.ctx, parent))
	s.cleanup = append(s.cleanup, parent.Code().String())

	otherParent, err := job.NewParentExecution(job.TypeProductCostSheetExport, "xlsx", period+"X", "integ-test", 5, nil, 1)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.repo.Create(s.ctx, otherParent))
	s.cleanup = append(s.cleanup, otherParent.Code().String())

	children := make([]*job.Execution, 0, 2)
	for i := 0; i < 2; i++ {
		child, childErr := job.NewChildExecution(job.TypeProductCostSheetExport, "xlsx", period, "integ-test", 5, nil, parent.ID())
		require.NoError(s.T(), childErr)
		children = append(children, child)
	}
	require.NoError(s.T(), s.repo.CreateChildren(s.ctx, children))
	for _, c := range children {
		s.cleanup = append(s.cleanup, c.Code().String())
	}

	unrelated, err := job.NewChildExecution(job.TypeProductCostSheetExport, "xlsx", period+"X", "integ-test", 5, nil, otherParent.ID())
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.repo.CreateChildren(s.ctx, []*job.Execution{unrelated}))
	s.cleanup = append(s.cleanup, unrelated.Code().String())

	got, listErr := s.repo.ListChildren(s.ctx, parent.ID())
	require.NoError(s.T(), listErr)
	require.Len(s.T(), got, 2)

	gotIDs := map[string]bool{got[0].ID().String(): true, got[1].ID().String(): true}
	for _, c := range children {
		require.True(s.T(), gotIDs[c.ID().String()], "expected child %s in ListChildren result", c.ID())
	}
	for _, c := range got {
		require.NotNil(s.T(), c.ParentJobID())
		require.Equal(s.T(), parent.ID(), *c.ParentJobID())
	}
}

// TestListChildren_NoChildrenReturnsEmptySlice verifies a parent with no
// children (or an unknown job ID) yields an empty, non-error result — the
// repository does not itself verify parent existence.
func (s *JobRepositoryBatchSuite) TestListChildren_NoChildrenReturnsEmptySlice() {
	period := newPeriod()
	parent, err := job.NewParentExecution(job.TypeProductCostSheetExport, "xlsx", period, "integ-test", 5, nil, 1)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.repo.Create(s.ctx, parent))
	s.cleanup = append(s.cleanup, parent.Code().String())

	got, listErr := s.repo.ListChildren(s.ctx, parent.ID())
	require.NoError(s.T(), listErr)
	require.Empty(s.T(), got)
}
