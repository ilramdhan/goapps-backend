package costcalc

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// --- pure unit tests (no DB) ---------------------------------------------

func TestGetJobHandler_InvalidID(t *testing.T) {
	t.Parallel()
	h := NewGetJobHandler(nil)
	_, err := h.Handle(context.Background(), GetJobQuery{JobID: 0})
	require.Error(t, err)
}

func TestListChunksHandler_InvalidJobID(t *testing.T) {
	t.Parallel()
	h := NewListChunksHandler(nil)
	_, err := h.Handle(context.Background(), ListChunksQuery{JobID: 0})
	require.Error(t, err)
}

func TestListJobProductsHandler_InvalidJobID(t *testing.T) {
	t.Parallel()
	h := NewListJobProductsHandler(nil)
	_, err := h.Handle(context.Background(), ListJobProductsQuery{JobID: 0})
	require.Error(t, err)
}

func TestCancelJobHandler_InvalidID(t *testing.T) {
	t.Parallel()
	h := NewCancelJobHandler(nil)
	_, err := h.Handle(context.Background(), CancelJobCommand{JobID: 0, Actor: "x"})
	require.Error(t, err)
}

func TestGetCostResultHandler_InvalidInputs(t *testing.T) {
	t.Parallel()
	h := NewGetCostResultHandler(nil)
	_, err := h.Handle(context.Background(), GetCostResultQuery{ProductSysID: 0, Period: "202605", CalcType: costcalcdom.CalcTypeActual})
	require.Error(t, err)
	_, err = h.Handle(context.Background(), GetCostResultQuery{ProductSysID: 1, Period: "bad", CalcType: costcalcdom.CalcTypeActual})
	require.Error(t, err)
}

func TestGetCostBreakdownHandler_InvalidInputs(t *testing.T) {
	t.Parallel()
	h := NewGetCostBreakdownHandler(nil)
	_, err := h.Handle(context.Background(), GetCostBreakdownQuery{ProductSysID: 0, Period: "202605", CalcType: costcalcdom.CalcTypeActual})
	require.Error(t, err)
	_, err = h.Handle(context.Background(), GetCostBreakdownQuery{ProductSysID: 1, Period: "x", CalcType: costcalcdom.CalcTypeActual})
	require.Error(t, err)
}

func TestListCostHistoryHandler_InvalidID(t *testing.T) {
	t.Parallel()
	h := NewListCostHistoryHandler(nil)
	_, err := h.Handle(context.Background(), ListCostHistoryQuery{ProductSysID: 0})
	require.Error(t, err)
}

func TestVerifyCostHandler_InvalidInputs(t *testing.T) {
	t.Parallel()
	h := NewVerifyCostHandler(nil)
	require.Error(t, h.Handle(context.Background(), VerifyCostCommand{CostID: 0, Actor: "x"}))
	require.Error(t, h.Handle(context.Background(), VerifyCostCommand{CostID: 1, Actor: ""}))
}

func TestApproveCostHandler_InvalidInputs(t *testing.T) {
	t.Parallel()
	h := NewApproveCostHandler(nil)
	require.Error(t, h.Handle(context.Background(), ApproveCostCommand{CostID: 0, Actor: "x"}))
	require.Error(t, h.Handle(context.Background(), ApproveCostCommand{CostID: 1, Actor: ""}))
}

func TestNormalizePagination(t *testing.T) {
	t.Parallel()
	cases := []struct {
		inPage, inSize     int
		wantPage, wantSize int
	}{
		{0, 0, 1, 20},
		{-5, -5, 1, 20},
		{1, 50, 1, 50},
		{2, 100, 2, 100},
		{3, 200, 3, 100}, // clamped
	}
	for _, c := range cases {
		gp, gs := normalizePagination(c.inPage, c.inSize)
		require.Equal(t, c.wantPage, gp)
		require.Equal(t, c.wantSize, gs)
	}
}

func TestDecodeJSONBlob_Empty(t *testing.T) {
	t.Parallel()
	dest := []LevelContribution{}
	require.NoError(t, decodeJSONBlob(nil, &dest))
	require.NoError(t, decodeJSONBlob([]byte{}, &dest))
	require.Empty(t, dest)
}

func TestDecodeJSONBlob_Valid(t *testing.T) {
	t.Parallel()
	var dest []LevelContribution
	require.NoError(t, decodeJSONBlob([]byte(`[{"level":1,"rm_cost":5.0,"conversion":2.0}]`), &dest))
	require.Len(t, dest, 1)
	require.Equal(t, int32(1), dest[0].Level)
	require.InDelta(t, 5.0, dest[0].RMCost, 0.001)
}

// --- integration tests (require INTEGRATION_TEST=true) -------------------

func TestHandlersIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	s := new(handlersSuite)
	s.ProcessChunkSuite = new(ProcessChunkSuite)
	s.SetT(t)
	s.SetupSuite()
	defer s.TearDownSuite()

	t.Run("GetJob_NotFound", s.testGetJobNotFound)
	t.Run("CancelJob_HappyPath", s.testCancelJobHappyPath)
	t.Run("CancelJob_NotFound", s.testCancelJobNotFound)
	t.Run("ListJobs_Pagination", s.testListJobsPagination)
	t.Run("ListChunks_FiltersAndPagination", s.testListChunksFilters)
	t.Run("ListJobProducts_HappyPath", s.testListJobProductsHappy)
	t.Run("CostResult_GetActiveAndBreakdownAndHistory", s.testCostResultEndToEnd)
	t.Run("VerifyAndApprove_Lifecycle", s.testVerifyApproveLifecycle)
}

type handlersSuite struct {
	*ProcessChunkSuite
}

// seedJobViaTrigger reuses the trigger handler to create a real end-to-end
// SUCCESS job + cost in the suite's period and returns the job.
func (s *handlersSuite) seedJobViaTrigger(period string) *costcalcdom.Job {
	// Ensure an RM cost row exists for the trigger's RM lookup at this period.
	_, err := s.raw.ExecContext(s.ctx, `
		INSERT INTO cst_rm_cost (
			period, rm_code, rm_type, item_code, cost_val,
			flag_valuation, flag_marketing, flag_simulation,
			flag_valuation_used, flag_marketing_used, flag_simulation_used,
			created_by
		) VALUES ($1, $2, 'ITEM', NULL, 25.0,
			'CONS','CONS','CONS','CONS','CONS','CONS',
			$3)
		ON CONFLICT DO NOTHING`, period, s.codePrefix+"-ITM", s.actor)
	require.NoError(s.T(), err)

	h := NewTriggerJobHandler(s.svc)
	job, err := h.Handle(s.ctx, TriggerCommand{
		Period:       period,
		CalcType:     s.calcType,
		Scope:        costcalcdom.ScopeSingleProduct,
		ProductSysID: s.productID,
		Actor:        s.actor,
		TriggeredBy:  "TEST",
	})
	require.NoError(s.T(), err)
	return job
}

func (s *handlersSuite) testGetJobNotFound(_ *testing.T) {
	h := NewGetJobHandler(s.svc)
	_, err := h.Handle(s.ctx, GetJobQuery{JobID: 9_999_999_999})
	require.Error(s.T(), err)
	require.True(s.T(), errors.Is(err, costcalcdom.ErrJobNotFound))
}

func (s *handlersSuite) testCancelJobHappyPath(_ *testing.T) {
	// Create a fresh QUEUED job directly so we can cancel it.
	job, err := costcalcdom.NewJob("999970", s.calcType, costcalcdom.ScopeSingleProduct, nil, "TEST", s.actor)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.svc.jobRepo.Create(s.ctx, job))

	h := NewCancelJobHandler(s.svc)
	got, err := h.Handle(s.ctx, CancelJobCommand{JobID: job.ID(), Actor: s.actor, Reason: "test cancel"})
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalcdom.JobStatusCancelled, got.Status())

	// Re-fetch from DB to confirm persisted.
	fetched, err := s.svc.jobRepo.GetByID(s.ctx, job.ID())
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalcdom.JobStatusCancelled, fetched.Status())
}

func (s *handlersSuite) testCancelJobNotFound(_ *testing.T) {
	h := NewCancelJobHandler(s.svc)
	_, err := h.Handle(s.ctx, CancelJobCommand{JobID: 9_999_999_998, Actor: s.actor, Reason: "x"})
	require.Error(s.T(), err)
}

func (s *handlersSuite) testListJobsPagination(_ *testing.T) {
	// Seed at least one job.
	_ = s.seedJobViaTrigger("999971")

	h := NewListJobsHandler(s.svc)
	res, err := h.Handle(s.ctx, ListJobsQuery{Page: 0, PageSize: 0}) // defaults
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, res.Page)
	require.Equal(s.T(), 20, res.PageSize)
	require.GreaterOrEqual(s.T(), res.Total, 1)
	require.NotEmpty(s.T(), res.Items)

	// Clamp test.
	res2, err := h.Handle(s.ctx, ListJobsQuery{Page: 1, PageSize: 500})
	require.NoError(s.T(), err)
	require.Equal(s.T(), 100, res2.PageSize)
}

func (s *handlersSuite) testListChunksFilters(_ *testing.T) {
	job := s.seedJobViaTrigger("999972")

	h := NewListChunksHandler(s.svc)
	res, err := h.Handle(s.ctx, ListChunksQuery{JobID: job.ID()})
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, res.Total)
	require.Len(s.T(), res.Items, 1)

	// Filter by status that no chunk has -> empty.
	failed := costcalcdom.ChunkStatusFailed
	res2, err := h.Handle(s.ctx, ListChunksQuery{JobID: job.ID(), Status: &failed})
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, res2.Total)
}

func (s *handlersSuite) testListJobProductsHappy(_ *testing.T) {
	job := s.seedJobViaTrigger("999973")

	h := NewListJobProductsHandler(s.svc)
	res, err := h.Handle(s.ctx, ListJobProductsQuery{JobID: job.ID()})
	require.NoError(s.T(), err)
	require.GreaterOrEqual(s.T(), res.Total, 1)
	require.NotEmpty(s.T(), res.Items)
	require.Equal(s.T(), costcalcdom.JobProductStatusSuccess, res.Items[0].Status())
}

func (s *handlersSuite) testCostResultEndToEnd(_ *testing.T) {
	period := "999974"
	_ = s.seedJobViaTrigger(period)

	// GetCostResult
	getH := NewGetCostResultHandler(s.svc)
	result, err := getH.Handle(s.ctx, GetCostResultQuery{
		ProductSysID: s.productID, Period: period, CalcType: s.calcType,
	})
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)
	require.Equal(s.T(), costcalcdom.ResultStatusCalculated, result.Status())

	// GetCostBreakdown
	brkH := NewGetCostBreakdownHandler(s.svc)
	view, err := brkH.Handle(s.ctx, GetCostBreakdownQuery{
		ProductSysID: s.productID, Period: period, CalcType: s.calcType,
	})
	require.NoError(s.T(), err)
	require.NotNil(s.T(), view.Result)
	require.NotNil(s.T(), view.ParamSnapshot)
	require.NotNil(s.T(), view.CostByLevel)

	// ListCostHistory
	histH := NewListCostHistoryHandler(s.svc)
	hist, err := histH.Handle(s.ctx, ListCostHistoryQuery{
		ProductSysID: s.productID, CalcType: s.calcType,
	})
	require.NoError(s.T(), err)
	require.GreaterOrEqual(s.T(), hist.Total, 1)
}

func (s *handlersSuite) testVerifyApproveLifecycle(_ *testing.T) {
	period := "999975"
	_ = s.seedJobViaTrigger(period)
	result, err := s.svc.resultRepo.GetActive(s.ctx, s.productID, period, s.calcType)
	require.NoError(s.T(), err)
	costID := result.ID()

	// Verify CALCULATED -> VERIFIED
	vH := NewVerifyCostHandler(s.svc)
	require.NoError(s.T(), vH.Handle(s.ctx, VerifyCostCommand{CostID: costID, Actor: s.actor}))

	// Confirm state.
	r2, err := s.svc.resultRepo.GetByID(s.ctx, costID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalcdom.ResultStatusVerified, r2.Status())

	// Approve VERIFIED -> APPROVED
	aH := NewApproveCostHandler(s.svc)
	require.NoError(s.T(), aH.Handle(s.ctx, ApproveCostCommand{CostID: costID, Actor: s.actor}))

	r3, err := s.svc.resultRepo.GetByID(s.ctx, costID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalcdom.ResultStatusApproved, r3.Status())

	// Re-verify already-approved should fail (invalid status transition).
	err = vH.Handle(s.ctx, VerifyCostCommand{CostID: costID, Actor: s.actor})
	require.Error(s.T(), err)
}
