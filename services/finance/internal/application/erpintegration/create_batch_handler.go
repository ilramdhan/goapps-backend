package erpintegration

// create_batch_handler.go implements CreateBatch (plan-04 P3-T4 step 1;
// design Part 1 §2 step 4, §5.3; Part 2 §8 CreateErpBatch).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// CreateBatchCommand creates a DRAFT batch for a period. Mode "" is LIVE.
type CreateBatchCommand struct {
	Period string
	Mode   domain.BatchMode
	Actor  string
}

// CreateBatchHandler creates DRAFT batches.
//
// A LIVE batch is refused when:
//   - (P, ACTUAL) is not locked in cst_period_lock (G10: without the lock the
//     cost could still move) -> ErrPeriodNotLocked;
//   - the read-only probe reports posted ADJ heads (V-10) -> ErrPeriodPosted;
//   - a LIVE batch is already in flight (DRAFT..PUSHED) -> ErrActiveBatchExists
//     (the uix_ceib_inflight index closes the race).
//
// A missing lock checker or probe fails closed. A SHADOW batch (historical
// backtest, design §10.1) is PG-only and never pushable, so it skips the lock,
// posted and in-flight guards.
type CreateBatchHandler struct {
	batches domain.BatchRepository
	locks   PeriodLockChecker
	prober  domain.ErpAdjHeadProber
	now     func() time.Time
}

// NewCreateBatchHandler builds the handler. locks and prober may be nil, in
// which case a LIVE create fails closed.
func NewCreateBatchHandler(batches domain.BatchRepository, locks PeriodLockChecker, prober domain.ErpAdjHeadProber) *CreateBatchHandler {
	return &CreateBatchHandler{batches: batches, locks: locks, prober: prober, now: time.Now}
}

// WithClock overrides the clock (tests).
func (h *CreateBatchHandler) WithClock(now func() time.Time) *CreateBatchHandler {
	h.now = now
	return h
}

// Handle creates the batch and returns it with id and seq set.
func (h *CreateBatchHandler) Handle(ctx context.Context, cmd CreateBatchCommand) (*domain.Batch, error) {
	mode := cmd.Mode
	if mode == "" {
		mode = domain.ModeLive
	}
	period := strings.TrimSpace(cmd.Period)
	b, err := domain.NewBatch(period, mode, cmd.Actor, h.now())
	if err != nil {
		return nil, err
	}
	if mode == domain.ModeLive {
		if err := h.checkLive(ctx, period); err != nil {
			return nil, err
		}
	}
	created, err := h.batches.Create(ctx, b)
	if err != nil {
		return nil, fmt.Errorf("persist erp batch: %w", err)
	}
	return created, nil
}

func (h *CreateBatchHandler) checkLive(ctx context.Context, period string) error {
	if h.locks == nil {
		return ErrPeriodLockNotConfigured
	}
	locked, err := h.locks.IsLocked(ctx, period, periodLockCalcType)
	if err != nil {
		return fmt.Errorf("check period lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("%w: %s ACTUAL", domain.ErrPeriodNotLocked, period)
	}
	if h.prober == nil {
		return ErrPostedProbeNotConfigured
	}
	posted, err := h.prober.ProbePosted(ctx, period)
	if err != nil {
		return fmt.Errorf("probe posted ADJ heads: %w", err)
	}
	if posted > 0 {
		return fmt.Errorf("%w: %d posted head(s) for %s", domain.ErrPeriodPosted, posted, period)
	}
	existing, err := h.batches.FindInFlight(ctx, period)
	switch {
	case err == nil:
		return fmt.Errorf("%w: batch %d (%s)", domain.ErrActiveBatchExists, existing.ID(), existing.Status())
	case errors.Is(err, domain.ErrBatchNotFound):
		return nil
	default:
		return fmt.Errorf("find in-flight batch: %w", err)
	}
}
