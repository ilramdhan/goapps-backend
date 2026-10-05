package erpintegration

import (
	"context"
	"strings"
	"time"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// AbandonBatchCommand abandons a non-terminal batch (AbandonErpBatch).
type AbandonBatchCommand struct {
	BatchID int64
	Reason  string
	Actor   string
}

// AbandonBatchHandler moves a batch to FAILED under the row lock. PG only:
// it never touches Oracle. The domain refuses LOCKED / terminal batches
// (ErrInvalidTransition).
type AbandonBatchHandler struct {
	runner domain.BatchTxRunner
	now    func() time.Time
}

// NewAbandonBatchHandler builds the handler.
func NewAbandonBatchHandler(runner domain.BatchTxRunner) *AbandonBatchHandler {
	return &AbandonBatchHandler{runner: runner, now: time.Now}
}

// Handle abandons the batch and returns it.
func (h *AbandonBatchHandler) Handle(ctx context.Context, cmd AbandonBatchCommand) (*domain.Batch, error) {
	reason := strings.TrimSpace(cmd.Reason)
	if reason == "" {
		reason = "abandoned by operator"
	}
	var out *domain.Batch
	err := h.runner.RunLocked(ctx, cmd.BatchID, func(ctx context.Context, st domain.BatchStore) error {
		b, err := st.GetForUpdate(ctx)
		if err != nil {
			return err
		}
		prev := b.Status()
		if err := b.Fail(reason, cmd.Actor, h.now()); err != nil {
			return err
		}
		if err := st.Save(ctx, b, prev, domain.Invalidation{}); err != nil {
			return err
		}
		out = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
