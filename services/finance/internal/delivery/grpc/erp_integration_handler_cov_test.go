package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	periodlockapp "github.com/mutugading/goapps-backend/services/finance/internal/application/periodlock"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	domainperiodlock "github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
)

// covSteps is a configurable erpStepTrigger.
type covSteps struct{ err error }

func (f covSteps) ex() (*job.Execution, error) {
	if f.err != nil {
		return nil, f.err
	}
	return job.NewExecution(job.TypeErpIntegration, "x", "202606", "tester", 5, nil)
}
func (f covSteps) Handle(context.Context, erpapp.StepTriggerCommand) (*job.Execution, error) {
	return f.ex()
}
func (f covSteps) TriggerPush(context.Context, erpapp.PushTriggerCommand) (*job.Execution, error) {
	return f.ex()
}
func (f covSteps) TriggerLockBatch(context.Context, erpapp.LockBatchTriggerCommand) (*job.Execution, error) {
	return f.ex()
}
func (f covSteps) TriggerAdjExecute(context.Context, erpapp.AdjTriggerCommand) (*job.Execution, error) {
	return f.ex()
}
func (f covSteps) TriggerSubtype(context.Context, string, string, any, string) (*job.Execution, error) {
	return f.ex()
}

type covBatches struct {
	list []*domain.Batch
	err  error
}

func (f covBatches) GetByID(context.Context, int64) (*domain.Batch, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.list[0], nil
}
func (f covBatches) ListByPeriod(context.Context, string) ([]*domain.Batch, error) {
	return f.list, f.err
}

type covPushPrev struct{ err error }

func (f covPushPrev) Handle(context.Context, erpapp.PushPreviewQuery) (erpapp.PushPreview, error) {
	return erpapp.PushPreview{RowCount: 4, SumStd: "9.5", RowsMD5: "abc"}, f.err
}

type covValPrev struct{ err error }

func (f covValPrev) Handle(context.Context, erpapp.ValuationPreviewCommand) (erpapp.ValuationPreviewResult, error) {
	return erpapp.ValuationPreviewResult{Preview: domain.ValuationPreview{ID: "pv1"}}, f.err
}

type covLock struct{ err error }

func (f covLock) Handle(context.Context, periodlockapp.LockCommand) (*domainperiodlock.PeriodLock, error) {
	return covPL(), f.err
}

type covUnlock struct{ err error }

func (f covUnlock) Handle(context.Context, periodlockapp.UnlockCommand) (periodlockapp.UnlockResult, error) {
	return periodlockapp.UnlockResult{Lock: covPL()}, f.err
}

type covPLGet struct{ err error }

func (f covPLGet) Handle(context.Context, periodlockapp.GetQuery) (*domainperiodlock.PeriodLock, error) {
	return covPL(), f.err
}

func covPL() *domainperiodlock.PeriodLock {
	return domainperiodlock.Reconstruct("202606", "ACTUAL", time.Now(), "tester", "why", nil, nil, "", "")
}

func covBatch(t *testing.T, mode domain.BatchMode) *domain.Batch {
	t.Helper()
	b, err := domain.NewBatch("202606", mode, "tester", time.Now())
	require.NoError(t, err)
	return b
}

func TestErpHandler_Triggers(t *testing.T) {
	run := func(name string, f func(h *ErpIntegrationHandler) (bool, string, string)) {
		t.Run(name+" ok", func(t *testing.T) {
			ok, code, id := f(NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Steps: covSteps{}}))
			assert.True(t, ok)
			assert.Equal(t, "200", code)
			assert.NotEmpty(t, id)
		})
		t.Run(name+" err", func(t *testing.T) {
			ok, code, id := f(NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Steps: covSteps{err: erpapp.ErrStepNotAllowed}}))
			assert.False(t, ok)
			assert.Equal(t, "412", code)
			assert.Empty(t, id)
		})
	}
	c := ctxWith()
	run("load", func(h *ErpIntegrationHandler) (bool, string, string) {
		r, _ := h.LoadErpDemand(c, &financev1.LoadErpDemandRequest{BatchId: 1})
		return r.Base.IsSuccess, r.Base.StatusCode, r.JobId
	})
	run("coverage", func(h *ErpIntegrationHandler) (bool, string, string) {
		r, _ := h.RunErpCoverage(c, &financev1.RunErpCoverageRequest{BatchId: 1})
		return r.Base.IsSuccess, r.Base.StatusCode, r.JobId
	})
	run("derive", func(h *ErpIntegrationHandler) (bool, string, string) {
		r, _ := h.RunErpDerive(c, &financev1.RunErpDeriveRequest{BatchId: 1})
		return r.Base.IsSuccess, r.Base.StatusCode, r.JobId
	})
	run("validate", func(h *ErpIntegrationHandler) (bool, string, string) {
		r, _ := h.ValidateErpBatch(c, &financev1.ValidateErpBatchRequest{BatchId: 1})
		return r.Base.IsSuccess, r.Base.StatusCode, r.JobId
	})
	run("recon", func(h *ErpIntegrationHandler) (bool, string, string) {
		r, _ := h.ReconErpBatch(c, &financev1.ReconErpBatchRequest{BatchId: 1})
		return r.Base.IsSuccess, r.Base.StatusCode, r.JobId
	})
	run("push", func(h *ErpIntegrationHandler) (bool, string, string) {
		r, _ := h.PushErpBatch(c, &financev1.PushErpBatchRequest{BatchId: 1})
		return r.Base.IsSuccess, r.Base.StatusCode, r.JobId
	})
	run("attr backfill", func(h *ErpIntegrationHandler) (bool, string, string) {
		r, _ := h.RunErpAttributeBackfill(c, &financev1.RunErpAttributeBackfillRequest{DryRun: true})
		return r.Base.IsSuccess, r.Base.StatusCode, r.JobId
	})
	run("backtest", func(h *ErpIntegrationHandler) (bool, string, string) {
		r, _ := h.RunErpBacktest(c, &financev1.RunErpBacktestRequest{Period: "202606"})
		return r.Base.IsSuccess, r.Base.StatusCode, r.JobId
	})
}

func TestErpHandler_BatchesAndLocks(t *testing.T) {
	c := ctxWith("finance.cost.erpintegration.push")
	live, shadow := covBatch(t, domain.ModeLive), covBatch(t, domain.ModeShadow)
	missing := covBatches{err: domain.ErrBatchNotFound}
	h := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{
		Batches: covBatches{list: []*domain.Batch{live, shadow}}, Steps: covSteps{},
		PushPrev: covPushPrev{}, ValPreview: covValPrev{}, Lock: covLock{}, Unlock: covUnlock{},
		PeriodLock: covPLGet{},
	})
	hm := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{
		Batches: missing, Steps: covSteps{err: domain.ErrFeatureDisabled}, PushPrev: covPushPrev{err: erpapp.ErrPushPermissionDenied},
		ValPreview: covValPrev{err: domain.ErrPreviewRequired}, Lock: covLock{err: domainperiodlock.ErrPeriodLocked},
		Unlock: covUnlock{err: domainperiodlock.ErrPeriodLocked}, PeriodLock: covPLGet{err: errors.New("boom")},
	})

	t.Run("get", func(t *testing.T) {
		r, _ := h.GetErpBatch(c, &financev1.GetErpBatchRequest{BatchId: 1})
		assert.Equal(t, "202606", r.Data.Period)
		r, _ = hm.GetErpBatch(c, &financev1.GetErpBatchRequest{BatchId: 1})
		assert.Equal(t, "404", r.Base.StatusCode)
	})
	t.Run("list", func(t *testing.T) {
		r, _ := h.ListErpBatches(c, &financev1.ListErpBatchesRequest{})
		assert.Equal(t, "400", r.Base.StatusCode)
		r, _ = h.ListErpBatches(c, &financev1.ListErpBatchesRequest{Period: "202606"})
		assert.Len(t, r.Data, 2)
		r, _ = h.ListErpBatches(c, &financev1.ListErpBatchesRequest{Period: "202606", Mode: financev1.ErpBatchMode_ERP_BATCH_MODE_SHADOW})
		assert.Len(t, r.Data, 1)
		r, _ = h.ListErpBatches(c, &financev1.ListErpBatchesRequest{Period: "202606", Status: financev1.ErpBatchStatus_ERP_BATCH_STATUS_FAILED})
		assert.Empty(t, r.Data)
		r, _ = hm.ListErpBatches(c, &financev1.ListErpBatchesRequest{Period: "202606"})
		assert.Equal(t, "404", r.Base.StatusCode)
	})
	t.Run("preview push", func(t *testing.T) {
		r, _ := h.PreviewErpPush(c, &financev1.PreviewErpPushRequest{BatchId: 1})
		assert.Equal(t, int64(4), r.RowCount)
		r, _ = hm.PreviewErpPush(c, &financev1.PreviewErpPushRequest{BatchId: 1})
		assert.Equal(t, "403", r.Base.StatusCode)
	})
	t.Run("preview adj", func(t *testing.T) {
		for _, op := range []financev1.ErpAdjOperation{
			financev1.ErpAdjOperation_ERP_ADJ_OPERATION_APPROVE, financev1.ErpAdjOperation_ERP_ADJ_OPERATION_RESTORE,
			financev1.ErpAdjOperation_ERP_ADJ_OPERATION_VALUATE,
		} {
			r, _ := h.PreviewErpAdjOperation(c, &financev1.PreviewErpAdjOperationRequest{BatchId: 1, Operation: op})
			assert.Equal(t, "pv1", r.PreviewId)
		}
		r, _ := hm.PreviewErpAdjOperation(c, &financev1.PreviewErpAdjOperationRequest{BatchId: 1})
		assert.Equal(t, "400", r.Base.StatusCode)
	})
	t.Run("lock batch", func(t *testing.T) {
		r, _ := h.LockErpBatch(c, &financev1.LockErpBatchRequest{BatchId: 1})
		assert.True(t, r.Base.IsSuccess)
		r, _ = hm.LockErpBatch(c, &financev1.LockErpBatchRequest{BatchId: 1})
		assert.Equal(t, "412", r.Base.StatusCode)
		// trigger ok but batch re-read fails
		hr := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Steps: covSteps{}, Batches: missing})
		r, _ = hr.LockErpBatch(c, &financev1.LockErpBatchRequest{BatchId: 1})
		assert.Equal(t, "404", r.Base.StatusCode)
	})
	t.Run("period lock", func(t *testing.T) {
		l, _ := h.LockErpPeriod(c, &financev1.LockErpPeriodRequest{Period: "202606"})
		assert.True(t, l.Data.Locked || l.Data.Period == "202606")
		l, _ = hm.LockErpPeriod(c, &financev1.LockErpPeriodRequest{Period: "202606"})
		assert.Equal(t, "412", l.Base.StatusCode)
		u, _ := h.UnlockErpPeriod(c, &financev1.UnlockErpPeriodRequest{Period: "202606"})
		assert.Equal(t, "202606", u.Data.Period)
		u, _ = hm.UnlockErpPeriod(c, &financev1.UnlockErpPeriodRequest{Period: "202606"})
		assert.Equal(t, "412", u.Base.StatusCode)
		g, _ := h.GetErpPeriodLock(c, &financev1.GetErpPeriodLockRequest{Period: "202606"})
		assert.Equal(t, "202606", g.Data.Period)
		g, _ = hm.GetErpPeriodLock(c, &financev1.GetErpPeriodLockRequest{Period: "202606"})
		assert.Equal(t, "500", g.Base.StatusCode)
		assert.Nil(t, erpLockToProto(nil))
		assert.Nil(t, erpBatchToProto(nil))
	})
}

func TestErpHandler_ScheduleModes(t *testing.T) {
	modes := []domain.ScheduleMode{domain.ScheduleEndOfMonth, domain.ScheduleStartOfMonth, domain.ScheduleDayOfMonth, domain.ScheduleSpecificDate, domain.ScheduleCron}
	for _, m := range modes {
		assert.Equal(t, m, erpScheduleModeFromProto(erpScheduleModeToProto(m)))
	}
	assert.Equal(t, domain.ScheduleMode(""), erpScheduleModeFromProto(financev1.ErpScheduleMode_ERP_SCHEDULE_MODE_UNSPECIFIED))
	assert.Equal(t, financev1.ErpScheduleMode_ERP_SCHEDULE_MODE_UNSPECIFIED, erpScheduleModeToProto("nope"))
	assert.Equal(t, "", erpNullDec(decimal.NullDecimal{}))
	assert.Equal(t, "1.5", erpNullDec(decimal.NullDecimal{Decimal: decimal.RequireFromString("1.5"), Valid: true}))
}
