package costcalc

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// --- pure unit -----------------------------------------------------------

func TestTriggerJob_UnsupportedScope_ReturnsError(t *testing.T) {
	t.Parallel()
	h := NewTriggerJobHandler(nil) // pure-input validation, no svc deref
	_, err := h.Handle(context.Background(), TriggerCommand{
		Period:       "202605",
		CalcType:     costcalcdom.CalcTypeActual,
		Scope:        costcalcdom.ScopeAll,
		ProductSysID: 123,
		Actor:        "test",
		TriggeredBy:  "TEST",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrScopeNotYetSupported))
}

func TestTriggerJob_SingleProductMissingID_ReturnsError(t *testing.T) {
	t.Parallel()
	h := NewTriggerJobHandler(nil)
	_, err := h.Handle(context.Background(), TriggerCommand{
		Period:      "202605",
		CalcType:    costcalcdom.CalcTypeActual,
		Scope:       costcalcdom.ScopeSingleProduct,
		Actor:       "test",
		TriggeredBy: "TEST",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrProductRequired))
}

// --- integration ----------------------------------------------------------

// TriggerSuite layers the trigger-handler end-to-end on top of the same
// fixture seeded by ProcessChunkSuite. Run separately so the DB state from
// process_chunk_test doesn't bleed in.
func TestTriggerSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	// Re-use the fixture machinery by embedding ProcessChunkSuite — gives us
	// product + route + formula + CAPP + RM costs already wired.
	s := new(triggerSuite)
	s.ProcessChunkSuite = new(ProcessChunkSuite)
	s.SetT(t)
	s.SetupSuite()
	defer s.TearDownSuite()

	t.Run("SingleProduct_E2E", s.TestTrigger_SingleProduct_E2E)
	t.Run("MissingRoute_JobBlocked", s.TestTrigger_MissingRoute_JobBlocked)
}

type triggerSuite struct {
	*ProcessChunkSuite
}

// TestTrigger_SingleProduct_E2E runs the full inline path and checks the job +
// chunk + product + cost rows all land in the expected terminal states.
func (s *triggerSuite) TestTrigger_SingleProduct_E2E(_ *testing.T) {
	// Use a fresh period so this test doesn't supersede earlier rows in the
	// shared fixture DB.
	period := "999992"
	_, err := s.raw.ExecContext(s.ctx, `
		INSERT INTO cst_rm_cost (
			period, rm_code, rm_type, item_code, cost_val,
			flag_valuation, flag_marketing, flag_simulation,
			flag_valuation_used, flag_marketing_used, flag_simulation_used,
			created_by
		) VALUES ($1, $2, 'ITEM', NULL, 25.0,
			'CONS','CONS','CONS','CONS','CONS','CONS',
			$3)`, period, s.codePrefix+"-ITM", s.actor)
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
	require.Equal(s.T(), costcalcdom.JobStatusSuccess, job.Status())
	require.Equal(s.T(), 1, job.TotalProducts())
	require.Equal(s.T(), 1, job.SuccessCount())
	require.Equal(s.T(), 0, job.FailedCount())
	require.Equal(s.T(), 0, job.BlockedCount())

	// Job row persisted.
	fetched, err := s.svc.jobRepo.GetByID(s.ctx, job.ID())
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalcdom.JobStatusSuccess, fetched.Status())

	// Cost row exists.
	active, err := s.svc.resultRepo.GetActive(s.ctx, s.productID, period, s.calcType)
	require.NoError(s.T(), err)
	require.InDelta(s.T(), 32.0, active.CostPerUnit(), 0.001)

	// Job-product row at SUCCESS, pointing at the cost row.
	jp, err := s.svc.productRepo.GetByJobAndProduct(s.ctx, job.ID(), s.productID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalcdom.JobProductStatusSuccess, jp.Status())
	require.Equal(s.T(), active.ID(), jp.CostID())

	// Exactly one chunk row was created for this job, in SUCCESS status.
	chunks, total, err := s.svc.chunkRepo.ListByJob(s.ctx, job.ID(), nil, nil, 1, 50)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, total)
	require.Len(s.T(), chunks, 1)
	require.Equal(s.T(), costcalcdom.ChunkStatusSuccess, chunks[0].Status())
}

// TestTrigger_MissingRoute_JobBlocked seeds a product with NO active route and
// verifies the handler ends the job in FAILED with blocked_count=1 instead of
// crashing.
func (s *triggerSuite) TestTrigger_MissingRoute_JobBlocked(_ *testing.T) {
	// Create a sibling product master row with no route at all.
	var typeID int
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type ORDER BY cpt_type_id LIMIT 1`,
	).Scan(&typeID))
	var orphanID int64
	require.NoError(s.T(), s.raw.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, 'no-route test', $3, $3) RETURNING cpm_product_sys_id`,
		s.codePrefix+"-NOROUTE", typeID, s.actor,
	).Scan(&orphanID))
	defer func() {
		_, _ = s.raw.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id = $1`, orphanID)
	}()

	h := NewTriggerJobHandler(s.svc)
	job, err := h.Handle(s.ctx, TriggerCommand{
		Period:       s.period,
		CalcType:     s.calcType,
		Scope:        costcalcdom.ScopeSingleProduct,
		ProductSysID: orphanID,
		Actor:        s.actor,
		TriggeredBy:  "TEST",
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalcdom.JobStatusFailed, job.Status())
	require.Equal(s.T(), 0, job.SuccessCount())
	require.Equal(s.T(), 1, job.BlockedCount())
}
