// Package postgres_test provides integration tests for the costcalc repositories.
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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// CostCalcReposSuite covers the 5 costcalc postgres repositories.
type CostCalcReposSuite struct {
	suite.Suite
	db           *postgres.DB
	jobRepo      *postgres.CostCalcJobRepository
	chunkRepo    *postgres.CostCalcChunkRepository
	productRepo  *postgres.CostCalcJobProductRepository
	resultRepo   *postgres.CostResultRepository
	auditRepo    *postgres.CostAuditHistoryRepository
	ctx          context.Context
	productSysID int64
	routeHeadID  int64
}

func TestCostCalcReposSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(CostCalcReposSuite))
}

func (s *CostCalcReposSuite) SetupSuite() {
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
	s.jobRepo = postgres.NewCostCalcJobRepository(s.db)
	s.chunkRepo = postgres.NewCostCalcChunkRepository(s.db)
	s.productRepo = postgres.NewCostCalcJobProductRepository(s.db)
	s.resultRepo = postgres.NewCostResultRepository(s.db)
	s.auditRepo = postgres.NewCostAuditHistoryRepository(s.db)

	s.productSysID, s.routeHeadID = s.seedProductAndRoute()
}

func (s *CostCalcReposSuite) TearDownSuite() {
	if s.db != nil {
		// Cleanup is best-effort — failures here would only affect re-runs.
		_, _ = s.db.ExecContext(s.ctx, `DELETE FROM aud_cost_history WHERE ach_product_sys_id = $1`, s.productSysID)
		_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cst_product_cost WHERE cpc_product_sys_id = $1`, s.productSysID)
		_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_head WHERE crh_head_id = $1`, s.routeHeadID)
		_, _ = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id = $1`, s.productSysID)
		_ = s.db.Close()
	}
}

// seedProductAndRoute inserts a throwaway product master + route head pair so
// FK constraints on cal_job_product and cst_product_cost are satisfiable.
func (s *CostCalcReposSuite) seedProductAndRoute() (int64, int64) {
	// Use a high-numbered product type known to exist locally (FG=4 typically).
	// Fall back: pick the first available type id at runtime.
	var typeID int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type ORDER BY cpt_type_id LIMIT 1`,
	).Scan(&typeID))

	code := fmt.Sprintf("CCT-%d", time.Now().UnixNano()%100000000)
	var productSysID int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (
			cpm_product_code, cpm_product_type_id, cpm_product_name,
			cpm_created_by, cpm_updated_by
		) VALUES ($1, $2, 'calc engine test product', 'integ-test', 'integ-test')
		RETURNING cpm_product_sys_id`,
		code, typeID,
	).Scan(&productSysID))

	var headID int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_head (
			crh_product_sys_id, crh_routing_status, crh_version,
			crh_created_by, crh_updated_by
		) VALUES ($1, 'DRAFT', 1, 'integ-test', 'integ-test')
		RETURNING crh_head_id`,
		productSysID,
	).Scan(&headID))

	return productSysID, headID
}

// ---------------------------------------------------------------------------
// Job repository
// ---------------------------------------------------------------------------

func (s *CostCalcReposSuite) TestJob_CreateGetList() {
	job, err := costcalc.NewJob("202605", costcalc.CalcTypeActual, costcalc.ScopeAll, nil, "MANUAL", "integ-test")
	require.NoError(s.T(), err)

	require.NoError(s.T(), s.jobRepo.Create(s.ctx, job))
	require.NotZero(s.T(), job.ID())
	require.Regexp(s.T(), `^JOB-\d{6}-\d{4}$`, job.Code())

	fetched, err := s.jobRepo.GetByID(s.ctx, job.ID())
	require.NoError(s.T(), err)
	require.Equal(s.T(), job.Code(), fetched.Code())
	require.Equal(s.T(), costcalc.JobStatusQueued, fetched.Status())

	list, total, err := s.jobRepo.List(s.ctx, costcalc.JobFilter{Period: "202605", PageSize: 50})
	require.NoError(s.T(), err)
	require.GreaterOrEqual(s.T(), total, 1)
	require.NotEmpty(s.T(), list)
}

func (s *CostCalcReposSuite) TestJob_StateTransitions() {
	job, err := costcalc.NewJob("202605", costcalc.CalcTypeActual, costcalc.ScopeAll, nil, "MANUAL", "integ-test")
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.jobRepo.Create(s.ctx, job))

	require.NoError(s.T(), s.jobRepo.UpdateStatus(s.ctx, job.ID(), costcalc.JobStatusPlanning))
	require.NoError(s.T(), s.jobRepo.UpdateTotals(s.ctx, job.ID(), 10, 2, 1))
	require.NoError(s.T(), s.jobRepo.UpdateProgress(s.ctx, job.ID(), 1, 5, 0, 0))
	require.NoError(s.T(), s.jobRepo.UpdateCompletion(s.ctx, job.ID(), costcalc.JobStatusSuccess, 10, 0, 0, 1234, []byte(`{}`)))

	fetched, err := s.jobRepo.GetByID(s.ctx, job.ID())
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalc.JobStatusSuccess, fetched.Status())
	require.Equal(s.T(), 10, fetched.TotalProducts())
	require.Equal(s.T(), int64(1234), fetched.DurationMs())
}

func (s *CostCalcReposSuite) TestJob_GetByID_NotFound() {
	_, err := s.jobRepo.GetByID(s.ctx, 999_999_999)
	require.ErrorIs(s.T(), err, costcalc.ErrJobNotFound)
}

// ---------------------------------------------------------------------------
// Chunk repository
// ---------------------------------------------------------------------------

func (s *CostCalcReposSuite) TestChunk_CreateAndRetry() {
	job := s.createJob("202605")

	chunk := costcalc.NewChunk(job.ID(), 1, 1, []int64{s.productSysID})
	require.NoError(s.T(), s.chunkRepo.Create(s.ctx, chunk))
	require.NotZero(s.T(), chunk.ID())

	fetched, err := s.chunkRepo.GetByID(s.ctx, chunk.ID())
	require.NoError(s.T(), err)
	require.Equal(s.T(), []int64{s.productSysID}, fetched.ProductIDs())

	n, err := s.chunkRepo.IncrementRetry(s.ctx, chunk.ID())
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, n)

	require.NoError(s.T(), s.chunkRepo.UpdateStatus(s.ctx, chunk.ID(), costcalc.ChunkStatusDispatched, "worker-1"))
	require.NoError(s.T(), s.chunkRepo.UpdateResult(s.ctx, chunk.ID(), costcalc.ChunkStatusSuccess, 1, 0, 250, ""))

	list, total, err := s.chunkRepo.ListByJob(s.ctx, job.ID(), nil, nil, 1, 50)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, total)
	require.Len(s.T(), list, 1)
	require.Equal(s.T(), costcalc.ChunkStatusSuccess, list[0].Status())
}

// ---------------------------------------------------------------------------
// JobProduct repository
// ---------------------------------------------------------------------------

func (s *CostCalcReposSuite) TestJobProduct_BulkAndTransitions() {
	job := s.createJob("202605")

	items := []*costcalc.JobProduct{
		costcalc.NewJobProduct(job.ID(), s.productSysID, s.routeHeadID, 1),
	}
	require.NoError(s.T(), s.productRepo.BulkCreate(s.ctx, items))
	require.NotZero(s.T(), items[0].ID())

	chunk := costcalc.NewChunk(job.ID(), 1, 1, []int64{s.productSysID})
	require.NoError(s.T(), s.chunkRepo.Create(s.ctx, chunk))

	require.NoError(s.T(), s.productRepo.AssignChunk(s.ctx, job.ID(), s.productSysID, chunk.ID()))
	require.NoError(s.T(), s.productRepo.MarkSuccess(s.ctx, job.ID(), s.productSysID, 0, 350, []byte(`{"ok":true}`)))

	jp, err := s.productRepo.GetByJobAndProduct(s.ctx, job.ID(), s.productSysID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalc.JobProductStatusSuccess, jp.Status())
	require.Equal(s.T(), chunk.ID(), jp.ChunkID())
}

func (s *CostCalcReposSuite) TestJobProduct_MarkSkippedForJob() {
	job := s.createJob("202605")
	items := []*costcalc.JobProduct{
		costcalc.NewJobProduct(job.ID(), s.productSysID, s.routeHeadID, 1),
	}
	require.NoError(s.T(), s.productRepo.BulkCreate(s.ctx, items))

	require.NoError(s.T(), s.productRepo.MarkSkippedForJob(s.ctx, job.ID()))
	jp, err := s.productRepo.GetByJobAndProduct(s.ctx, job.ID(), s.productSysID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalc.JobProductStatusSkipped, jp.Status())

	// A SUCCESS row must not be flipped to SKIPPED on a second pass.
	job2 := s.createJob("202605")
	items2 := []*costcalc.JobProduct{
		costcalc.NewJobProduct(job2.ID(), s.productSysID, s.routeHeadID, 1),
	}
	require.NoError(s.T(), s.productRepo.BulkCreate(s.ctx, items2))
	require.NoError(s.T(), s.productRepo.MarkSuccess(s.ctx, job2.ID(), s.productSysID, 0, 100, []byte(`{}`)))
	require.NoError(s.T(), s.productRepo.MarkSkippedForJob(s.ctx, job2.ID()))
	jp2, err := s.productRepo.GetByJobAndProduct(s.ctx, job2.ID(), s.productSysID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalc.JobProductStatusSuccess, jp2.Status())
}

// ---------------------------------------------------------------------------
// Result repository
// ---------------------------------------------------------------------------

func (s *CostCalcReposSuite) TestResult_UpsertWithSupersedeVersionBump() {
	job := s.createJob("202605")

	first := costcalc.NewResult(
		s.productSysID, "202605", costcalc.CalcTypeActual, s.routeHeadID, 1,
		100.0, 60.0, 40.0, 100.0, 0, "IDR",
		nil, nil, nil, nil, "hash-1", job.ID(), "integ-test",
	)
	newID, prevVer, _, prevID, err := s.resultRepo.UpsertWithSupersede(s.ctx, first)
	require.NoError(s.T(), err)
	require.NotZero(s.T(), newID)
	require.Equal(s.T(), 0, prevVer)
	require.Equal(s.T(), int64(0), prevID)

	second := costcalc.NewResult(
		s.productSysID, "202605", costcalc.CalcTypeActual, s.routeHeadID, 1,
		120.0, 70.0, 50.0, 120.0, 0, "IDR",
		nil, nil, nil, nil, "hash-2", job.ID(), "integ-test",
	)
	newID2, prevVer2, prevTotal2, prevID2, err := s.resultRepo.UpsertWithSupersede(s.ctx, second)
	require.NoError(s.T(), err)
	require.NotEqual(s.T(), newID, newID2)
	require.Equal(s.T(), 1, prevVer2)
	require.InDelta(s.T(), 100.0, prevTotal2, 0.001)
	require.Equal(s.T(), newID, prevID2)

	active, err := s.resultRepo.GetActive(s.ctx, s.productSysID, "202605", costcalc.CalcTypeActual)
	require.NoError(s.T(), err)
	require.Equal(s.T(), newID2, active.ID())
	require.Equal(s.T(), 2, active.Version())
}

// ---------------------------------------------------------------------------
// Audit history repository
// ---------------------------------------------------------------------------

func (s *CostCalcReposSuite) TestAuditHistory_Write() {
	job := s.createJob("202605")
	entry := &costcalc.AuditHistoryEntry{
		ProductSysID: s.productSysID,
		Period:       "202605",
		CalcType:     costcalc.CalcTypeActual,
		OldCostID:    0,
		NewCostID:    0,
		OldTotal:     0,
		NewTotal:     100.0,
		VariancePct:  0,
		OldJobID:     0,
		NewJobID:     job.ID(),
		ChangeReason: "INITIAL_CALC",
		ChangedBy:    "integ-test",
	}
	require.NoError(s.T(), s.auditRepo.Write(s.ctx, entry))

	var count int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT count(*) FROM aud_cost_history WHERE ach_product_sys_id = $1 AND ach_new_job_id = $2`,
		s.productSysID, job.ID(),
	).Scan(&count))
	require.GreaterOrEqual(s.T(), count, 1)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (s *CostCalcReposSuite) createJob(period string) *costcalc.Job {
	job, err := costcalc.NewJob(period, costcalc.CalcTypeActual, costcalc.ScopeAll, nil, "MANUAL", "integ-test")
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.jobRepo.Create(s.ctx, job))
	return job
}
