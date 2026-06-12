// Package postgres_test provides integration tests for the fill-assignment repositories.
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

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// FillAssignmentRepoSuite covers CostFillConfigRepository and CostFillTaskRepository.
type FillAssignmentRepoSuite struct {
	suite.Suite
	db           *postgres.DB
	configRepo   *postgres.CostFillConfigRepository
	taskRepo     *postgres.CostFillTaskRepository
	ctx          context.Context
	productSysID int64
	routeHeadID  int64
	requestID    int64
}

func TestFillAssignmentRepoSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(FillAssignmentRepoSuite))
}

func (s *FillAssignmentRepoSuite) SetupSuite() {
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
	s.configRepo = postgres.NewCostFillConfigRepository(s.db)
	s.taskRepo = postgres.NewCostFillTaskRepository(s.db)

	s.productSysID, s.routeHeadID = s.seedProductAndRoute()
	s.requestID = s.seedRequest()
}

func (s *FillAssignmentRepoSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	// Best-effort cleanup: ignore errors so test output is not polluted.
	if s.requestID != 0 {
		_, _ = s.db.ExecContext(s.ctx,
			`DELETE FROM cost_fill_task WHERE cft_request_id=$1`, s.requestID)
		_, _ = s.db.ExecContext(s.ctx,
			`DELETE FROM cost_product_request WHERE cpr_request_id=$1`, s.requestID)
	}
	_, _ = s.db.ExecContext(s.ctx,
		`DELETE FROM cost_level_assignment_config WHERE clac_created_by='integ-fill-test'`)
	_, _ = s.db.ExecContext(s.ctx,
		`DELETE FROM cost_product_level_assignment WHERE cpla_created_by='integ-fill-test'`)
	if s.routeHeadID != 0 {
		_, _ = s.db.ExecContext(s.ctx,
			`DELETE FROM cost_route_head WHERE crh_head_id=$1`, s.routeHeadID)
	}
	if s.productSysID != 0 {
		_, _ = s.db.ExecContext(s.ctx,
			`DELETE FROM cost_product_master WHERE cpm_product_sys_id=$1`, s.productSysID)
	}
	_ = s.db.Close()
}

// seedProductAndRoute inserts a throwaway product master + route head so FK
// constraints on cost_fill_task are satisfiable.
func (s *FillAssignmentRepoSuite) seedProductAndRoute() (int64, int64) {
	var typeID int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type ORDER BY cpt_type_id LIMIT 1`,
	).Scan(&typeID))

	code := fmt.Sprintf("FILL-TEST-%d", time.Now().UnixNano()%100_000_000)
	var productSysID int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (
			cpm_product_code, cpm_product_type_id, cpm_product_name,
			cpm_created_by, cpm_updated_by
		) VALUES ($1, $2, 'fill assignment integ test', 'integ-fill-test', 'integ-fill-test')
		RETURNING cpm_product_sys_id`,
		code, typeID,
	).Scan(&productSysID))

	var headID int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_head (
			crh_product_sys_id, crh_routing_status, crh_version,
			crh_created_by, crh_updated_by
		) VALUES ($1, 'DRAFT', 1, 'integ-fill-test', 'integ-fill-test')
		RETURNING crh_head_id`,
		productSysID,
	).Scan(&headID))

	return productSysID, headID
}

// seedRequest inserts a minimal cost_product_request row so that fill tasks can
// reference a real request FK. Skips gracefully if no request type exists.
func (s *FillAssignmentRepoSuite) seedRequest() int64 {
	var typeID int
	if err := s.db.QueryRowContext(s.ctx,
		`SELECT crt_type_id FROM cost_request_type ORDER BY crt_type_id LIMIT 1`,
	).Scan(&typeID); err != nil {
		s.T().Skipf("no cost_request_type found — skipping task tests: %v", err)
		return 0
	}

	requestNo := fmt.Sprintf("FILL-INTEG-%d", time.Now().UnixNano()%10_000_000)
	var id int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_request (
			cpr_request_no, cpr_request_type_id, cpr_title,
			cpr_customer_name, cpr_product_classification,
			cpr_requester_user_id, cpr_status
		) VALUES ($1,$2,'Fill integ test request','Test Customer','new','integ-fill-test','ROUTING_DEFINED')
		RETURNING cpr_request_id`,
		requestNo, typeID,
	).Scan(&id))
	return id
}

// ---------------------------------------------------------------------------
// Config repository — global tier
// ---------------------------------------------------------------------------

// TestGlobalConfig_UpsertGetListDelete verifies the full happy-path for global
// config: upsert creates a new active row, get fetches it, list includes it,
// a second upsert deactivates the old row, and delete marks the active row inactive.
func (s *FillAssignmentRepoSuite) TestGlobalConfig_UpsertGetListDelete() {
	const testLevel = int32(99) // high value avoids colliding with seeded levels 1-3.
	fillerType := domain.ActorUser
	fillerValue := "integ-test-user-1"
	slaFill := int32(24)
	slaApprove := int32(12)

	cfg := &domain.Config{
		Tier:            domain.TierGlobal,
		RouteLevel:      testLevel,
		FillerType:      &fillerType,
		FillerValue:     &fillerValue,
		SLAFillHours:    &slaFill,
		SLAApproveHours: &slaApprove,
	}

	// Upsert creates the active row.
	require.NoError(s.T(), s.configRepo.UpsertGlobal(s.ctx, cfg, "integ-fill-test"))

	// GetGlobal returns the row with correct values.
	got, err := s.configRepo.GetGlobal(s.ctx, testLevel)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), got)
	require.Equal(s.T(), testLevel, got.RouteLevel)
	require.NotNil(s.T(), got.FillerType)
	require.Equal(s.T(), domain.ActorUser, *got.FillerType)
	require.NotNil(s.T(), got.FillerValue)
	require.Equal(s.T(), fillerValue, *got.FillerValue)

	// ListGlobal includes the inserted level.
	list, err := s.configRepo.ListGlobal(s.ctx)
	require.NoError(s.T(), err)
	found := false
	for _, c := range list {
		if c.RouteLevel == testLevel {
			found = true
		}
	}
	require.True(s.T(), found, "level 99 must appear in ListGlobal")

	// A second upsert deactivates the prior row and inserts a fresh one.
	require.NoError(s.T(), s.configRepo.UpsertGlobal(s.ctx, cfg, "integ-fill-test"))

	// Exactly one active row should exist after the second upsert.
	var activeCount int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT COUNT(*) FROM cost_level_assignment_config
		  WHERE clac_route_level=$1 AND clac_is_active=true`, testLevel,
	).Scan(&activeCount))
	require.Equal(s.T(), 1, activeCount, "exactly one active row after second upsert")

	// DeleteGlobal deactivates the remaining active row.
	require.NoError(s.T(), s.configRepo.DeleteGlobal(s.ctx, testLevel))

	// GetGlobal after delete must return ErrConfigNotFound.
	_, err = s.configRepo.GetGlobal(s.ctx, testLevel)
	require.ErrorIs(s.T(), err, domain.ErrConfigNotFound)
}

// TestGlobalConfig_GetNotFound confirms GetGlobal returns ErrConfigNotFound for
// a level that has never been inserted.
func (s *FillAssignmentRepoSuite) TestGlobalConfig_GetNotFound() {
	_, err := s.configRepo.GetGlobal(s.ctx, 999_999)
	require.ErrorIs(s.T(), err, domain.ErrConfigNotFound)
}

// ---------------------------------------------------------------------------
// Config repository — product tier
// ---------------------------------------------------------------------------

// TestProductOverride_UpsertGet verifies that UpsertProduct creates a row and
// GetProduct retrieves it with the correct filler type.
func (s *FillAssignmentRepoSuite) TestProductOverride_UpsertGet() {
	fillerType := domain.ActorDept
	fillerValue := "ENG"
	cfg := &domain.Config{
		Tier:         domain.TierProduct,
		ProductSysID: s.productSysID,
		RouteLevel:   1,
		FillerType:   &fillerType,
		FillerValue:  &fillerValue,
	}
	require.NoError(s.T(), s.configRepo.UpsertProduct(s.ctx, cfg, "integ-fill-test"))

	got, err := s.configRepo.GetProduct(s.ctx, s.productSysID, 1)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), got)
	require.Equal(s.T(), domain.TierProduct, got.Tier)
	require.NotNil(s.T(), got.FillerType)
	require.Equal(s.T(), domain.ActorDept, *got.FillerType)
}

// TestProductOverride_GetNotExists confirms GetProduct returns nil (no override)
// when no row exists for the given product+level.
func (s *FillAssignmentRepoSuite) TestProductOverride_GetNotExists() {
	got, err := s.configRepo.GetProduct(s.ctx, 999_999_999, 99)
	require.NoError(s.T(), err, "GetProduct must return nil, nil when no row exists")
	require.Nil(s.T(), got, "no override row → result must be nil")
}

// TestProductOverride_UpsertIdempotent verifies that calling UpsertProduct twice
// for the same (product, level) does not error and updates the row.
func (s *FillAssignmentRepoSuite) TestProductOverride_UpsertIdempotent() {
	fillerType := domain.ActorUser
	fillerValue := "user-first"
	cfg := &domain.Config{
		Tier:         domain.TierProduct,
		ProductSysID: s.productSysID,
		RouteLevel:   2,
		FillerType:   &fillerType,
		FillerValue:  &fillerValue,
	}
	require.NoError(s.T(), s.configRepo.UpsertProduct(s.ctx, cfg, "integ-fill-test"))

	// Second upsert with different value.
	fillerValue2 := "user-second"
	cfg.FillerValue = &fillerValue2
	require.NoError(s.T(), s.configRepo.UpsertProduct(s.ctx, cfg, "integ-fill-test"))

	got, err := s.configRepo.GetProduct(s.ctx, s.productSysID, 2)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), got)
	require.Equal(s.T(), "user-second", *got.FillerValue, "second upsert must overwrite filler value")
}

// ---------------------------------------------------------------------------
// Task repository — lifecycle tests
// ---------------------------------------------------------------------------

// TestTask_BulkInsertAndGetAndList verifies task creation, retrieval by ID,
// retrieval by (request, level), and list by request.
func (s *FillAssignmentRepoSuite) TestTask_BulkInsertAndGetAndList() {
	if s.requestID == 0 {
		s.T().Skip("no request ID available — skipping task tests")
	}
	rc := domain.ResolvedConfig{
		RouteLevel:      10,
		FillerType:      domain.ActorUser,
		FillerValue:     "filler-bulk",
		SLAFillHours:    48,
		SLAApproveHours: 24,
	}
	task := domain.NewTask(s.requestID, s.routeHeadID, rc, 5)
	require.NoError(s.T(), s.taskRepo.BulkInsert(s.ctx, []*domain.Task{task}))

	// GetByRequestLevel returns the task with status ACTIVE.
	byLevel, err := s.taskRepo.GetByRequestLevel(s.ctx, s.requestID, 10)
	require.NoError(s.T(), err)
	require.Equal(s.T(), domain.StatusActive, byLevel.Status())
	require.Equal(s.T(), int32(10), byLevel.RouteLevel)
	require.NotZero(s.T(), byLevel.TaskID)

	// GetByID returns the same task.
	byID, err := s.taskRepo.GetByID(s.ctx, byLevel.TaskID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), byLevel.TaskID, byID.TaskID)
	require.Equal(s.T(), int32(5), byID.TotalParams)

	// ListByRequest includes the inserted task.
	list, err := s.taskRepo.ListByRequest(s.ctx, s.requestID)
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), list)
	found := false
	for _, t := range list {
		if t.TaskID == byLevel.TaskID {
			found = true
		}
	}
	require.True(s.T(), found, "BulkInsert task must appear in ListByRequest")
}

// TestTask_ClaimLifecycle verifies the atomic Claim operation: a first claim
// succeeds and flips the task to FILLING, a second claim by a different user
// returns false without error.
func (s *FillAssignmentRepoSuite) TestTask_ClaimLifecycle() {
	if s.requestID == 0 {
		s.T().Skip("no request ID available — skipping task tests")
	}
	rc := domain.ResolvedConfig{
		RouteLevel:   20,
		FillerType:   domain.ActorUser,
		FillerValue:  "filler-claim",
		SLAFillHours: 48,
	}
	task := domain.NewTask(s.requestID, s.routeHeadID, rc, 3)
	require.NoError(s.T(), s.taskRepo.BulkInsert(s.ctx, []*domain.Task{task}))

	inserted, err := s.taskRepo.GetByRequestLevel(s.ctx, s.requestID, 20)
	require.NoError(s.T(), err)

	// First claim succeeds.
	ok, err := s.taskRepo.Claim(s.ctx, inserted.TaskID, "filler-claim")
	require.NoError(s.T(), err)
	require.True(s.T(), ok, "first Claim must return true")

	// Second claim by a different user must return false (already owned).
	ok2, err := s.taskRepo.Claim(s.ctx, inserted.TaskID, "intruder-99")
	require.NoError(s.T(), err)
	require.False(s.T(), ok2, "second Claim on a taken task must return false")

	// DB row reflects FILLING status and the correct claimer.
	updated, err := s.taskRepo.GetByID(s.ctx, inserted.TaskID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), domain.StatusFilling, updated.Status())
	require.Equal(s.T(), "filler-claim", updated.ClaimedBy)
}

// TestTask_IncrementFilled verifies that IncrementFilled accumulates counts and
// caps at TotalParams.
func (s *FillAssignmentRepoSuite) TestTask_IncrementFilled() {
	if s.requestID == 0 {
		s.T().Skip("no request ID available — skipping task tests")
	}
	rc := domain.ResolvedConfig{
		RouteLevel:   30,
		FillerType:   domain.ActorUser,
		FillerValue:  "filler-inc",
		SLAFillHours: 48,
	}
	task := domain.NewTask(s.requestID, s.routeHeadID, rc, 10)
	require.NoError(s.T(), s.taskRepo.BulkInsert(s.ctx, []*domain.Task{task}))

	// Increment by 3 → filled_params should be 3.
	updated, err := s.taskRepo.IncrementFilled(s.ctx, s.requestID, 30, 3)
	require.NoError(s.T(), err)
	require.Equal(s.T(), int32(3), updated.FilledParams)

	// Increment by 4 → total 7.
	updated2, err := s.taskRepo.IncrementFilled(s.ctx, s.requestID, 30, 4)
	require.NoError(s.T(), err)
	require.Equal(s.T(), int32(7), updated2.FilledParams)

	// Increment by 100 → must cap at total_params (10), not 107.
	updated3, err := s.taskRepo.IncrementFilled(s.ctx, s.requestID, 30, 100)
	require.NoError(s.T(), err)
	require.Equal(s.T(), int32(10), updated3.FilledParams, "FilledParams must be capped at TotalParams")
}

// TestTask_GetByID_NotFound confirms ErrTaskNotFound is returned for a
// non-existent task ID.
func (s *FillAssignmentRepoSuite) TestTask_GetByID_NotFound() {
	_, err := s.taskRepo.GetByID(s.ctx, 999_999_999_999)
	require.ErrorIs(s.T(), err, domain.ErrTaskNotFound)
}

// TestTask_GetByRequestLevel_NotFound confirms ErrTaskNotFound is returned when
// no task exists for the given (request, level).
func (s *FillAssignmentRepoSuite) TestTask_GetByRequestLevel_NotFound() {
	if s.requestID == 0 {
		s.T().Skip("no request ID available — skipping task tests")
	}
	_, err := s.taskRepo.GetByRequestLevel(s.ctx, s.requestID, 9_999)
	require.ErrorIs(s.T(), err, domain.ErrTaskNotFound)
}

// TestTask_SaveStatusAndFilledAt verifies Save persists status and filled_at
// after calling domain.Submit on a claimed task.
func (s *FillAssignmentRepoSuite) TestTask_SaveStatusAndFilledAt() {
	if s.requestID == 0 {
		s.T().Skip("no request ID available — skipping task tests")
	}
	rc := domain.ResolvedConfig{
		RouteLevel:   40,
		FillerType:   domain.ActorUser,
		FillerValue:  "filler-save",
		SLAFillHours: 48,
	}
	task := domain.NewTask(s.requestID, s.routeHeadID, rc, 2)
	require.NoError(s.T(), s.taskRepo.BulkInsert(s.ctx, []*domain.Task{task}))

	inserted, err := s.taskRepo.GetByRequestLevel(s.ctx, s.requestID, 40)
	require.NoError(s.T(), err)

	// Simulate repo claim via Claim() then domain Submit (no approver → APPROVED).
	_, _ = s.taskRepo.Claim(s.ctx, inserted.TaskID, "filler-save")
	inserted.ClaimedBy = "filler-save"
	// Manually advance domain state to FILLING then Submit.
	require.NoError(s.T(), inserted.Claim("filler-save"))
	inserted.FilledParams = inserted.TotalParams // simulate all parameters filled
	require.NoError(s.T(), inserted.Submit())
	require.Equal(s.T(), domain.StatusApproved, inserted.Status(), "no approver → should go straight to APPROVED")

	require.NoError(s.T(), s.taskRepo.Save(s.ctx, inserted))

	persisted, err := s.taskRepo.GetByID(s.ctx, inserted.TaskID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), domain.StatusApproved, persisted.Status())
	require.NotNil(s.T(), persisted.FilledAt, "FilledAt must be set after Submit")
}

// TestTask_ApprovalCycle verifies the full lifecycle: bulk insert → claim →
// submit (with approver) → add approval → list approvals → count non-approved.
func (s *FillAssignmentRepoSuite) TestTask_ApprovalCycle() {
	if s.requestID == 0 {
		s.T().Skip("no request ID available — skipping task tests")
	}
	rc := domain.ResolvedConfig{
		RouteLevel:      50,
		FillerType:      domain.ActorUser,
		FillerValue:     "filler-appr",
		ApproverType:    domain.ActorUser,
		ApproverValue:   "approver-appr",
		SLAFillHours:    48,
		SLAApproveHours: 24,
	}
	task := domain.NewTask(s.requestID, s.routeHeadID, rc, 2)
	require.NoError(s.T(), s.taskRepo.BulkInsert(s.ctx, []*domain.Task{task}))

	inserted, err := s.taskRepo.GetByRequestLevel(s.ctx, s.requestID, 50)
	require.NoError(s.T(), err)

	// Claim and advance domain state.
	_, _ = s.taskRepo.Claim(s.ctx, inserted.TaskID, "filler-appr")
	require.NoError(s.T(), inserted.Claim("filler-appr"))
	inserted.FilledParams = inserted.TotalParams // simulate all parameters filled
	require.NoError(s.T(), inserted.Submit())    // → APPROVAL_PENDING (has approver)
	require.Equal(s.T(), domain.StatusApprovalPending, inserted.Status())
	require.NoError(s.T(), s.taskRepo.Save(s.ctx, inserted))

	// AddApproval records the decision.
	a := &domain.Approval{
		TaskID:    inserted.TaskID,
		Decision:  domain.DecisionApproved,
		DecidedBy: "approver-appr",
		Trigger:   domain.TriggerInitial,
	}
	require.NoError(s.T(), s.taskRepo.AddApproval(s.ctx, a))

	// ListApprovals returns the recorded decision.
	approvals, err := s.taskRepo.ListApprovals(s.ctx, inserted.TaskID)
	require.NoError(s.T(), err)
	require.Len(s.T(), approvals, 1)
	require.Equal(s.T(), domain.DecisionApproved, approvals[0].Decision)
	require.Equal(s.T(), "approver-appr", approvals[0].DecidedBy)
	require.Equal(s.T(), domain.TriggerInitial, approvals[0].Trigger)

	// CountNonApproved must be >= 0 and not error.
	n, err := s.taskRepo.CountNonApproved(s.ctx, s.requestID)
	require.NoError(s.T(), err)
	require.GreaterOrEqual(s.T(), n, 0)
}

// TestTask_OverdueAndNotified verifies ListOverdue finds tasks past their SLA
// and that MarkNotified suppresses them from re-appearing before the gap expires.
func (s *FillAssignmentRepoSuite) TestTask_OverdueAndNotified() {
	if s.requestID == 0 {
		s.T().Skip("no request ID available — skipping task tests")
	}
	rc := domain.ResolvedConfig{
		RouteLevel:   60,
		FillerType:   domain.ActorUser,
		FillerValue:  "filler-sla",
		SLAFillHours: 1, // 1-hour SLA → easy to force overdue
	}
	task := domain.NewTask(s.requestID, s.routeHeadID, rc, 1)
	require.NoError(s.T(), s.taskRepo.BulkInsert(s.ctx, []*domain.Task{task}))

	inserted, err := s.taskRepo.GetByRequestLevel(s.ctx, s.requestID, 60)
	require.NoError(s.T(), err)

	// Force activated_at 2 hours into the past so SLA of 1h is already exceeded.
	_, err = s.db.ExecContext(s.ctx,
		`UPDATE cost_fill_task
		    SET cft_activated_at = NOW() - INTERVAL '2 hours'
		  WHERE cft_task_id=$1`, inserted.TaskID)
	require.NoError(s.T(), err)

	// ListOverdue (gap=0) must include this task.
	overdue, err := s.taskRepo.ListOverdue(s.ctx, 0)
	require.NoError(s.T(), err)
	foundOverdue := false
	for _, t := range overdue {
		if t.TaskID == inserted.TaskID {
			foundOverdue = true
		}
	}
	require.True(s.T(), foundOverdue, "overdue task must appear in ListOverdue")

	// MarkNotified stamps last_notified_at to NOW().
	require.NoError(s.T(), s.taskRepo.MarkNotified(s.ctx, inserted.TaskID))

	// ListOverdue with a 4-hour reminder gap must exclude the just-notified task.
	overdue2, err := s.taskRepo.ListOverdue(s.ctx, 4)
	require.NoError(s.T(), err)
	for _, t := range overdue2 {
		require.NotEqual(s.T(), inserted.TaskID, t.TaskID,
			"recently notified task must not reappear before gap expires")
	}
}

// TestTask_ListForUser verifies that tasks assigned to a specific user ID are
// returned and tasks assigned to a different user are excluded.
func (s *FillAssignmentRepoSuite) TestTask_ListForUser() {
	if s.requestID == 0 {
		s.T().Skip("no request ID available — skipping task tests")
	}
	const myUser = "my-specific-user-integ"
	rc := domain.ResolvedConfig{
		RouteLevel:   70,
		FillerType:   domain.ActorUser,
		FillerValue:  myUser,
		SLAFillHours: 48,
	}
	task := domain.NewTask(s.requestID, s.routeHeadID, rc, 1)
	require.NoError(s.T(), s.taskRepo.BulkInsert(s.ctx, []*domain.Task{task}))

	inserted, err := s.taskRepo.GetByRequestLevel(s.ctx, s.requestID, 70)
	require.NoError(s.T(), err)

	// ListForUser with myUser must return this task.
	mine, err := s.taskRepo.ListForUser(s.ctx, myUser, nil)
	require.NoError(s.T(), err)
	found := false
	for _, t := range mine {
		if t.TaskID == inserted.TaskID {
			found = true
		}
	}
	require.True(s.T(), found, "task assigned to user must appear in ListForUser")

	// ListForUser with a different user must not return this task.
	others, err := s.taskRepo.ListForUser(s.ctx, "other-user-integ", nil)
	require.NoError(s.T(), err)
	for _, t := range others {
		require.NotEqual(s.T(), inserted.TaskID, t.TaskID,
			"task assigned to myUser must not appear for other-user")
	}
}

// TestTask_BulkInsert_Empty confirms that BulkInsert with no tasks is a no-op
// and does not error.
func (s *FillAssignmentRepoSuite) TestTask_BulkInsert_Empty() {
	require.NoError(s.T(), s.taskRepo.BulkInsert(s.ctx, nil))
	require.NoError(s.T(), s.taskRepo.BulkInsert(s.ctx, []*domain.Task{}))
}
