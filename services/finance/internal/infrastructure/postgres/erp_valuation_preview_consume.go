package postgres

// erp_valuation_preview_consume.go implements erpintegration.PreviewConsumer
// (plan-06 P5-T5; design Part 1 §3.2 G5): a preview flips OPEN -> CONSUMED
// exactly once, in its own autocommit statement. PG side only.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

var _ erpintegration.PreviewConsumer = (*ErpValuationPreviewRepository)(nil)

const erpPreviewConsumeSQL = `
	UPDATE cst_erp_valuation_preview
	   SET cevp_status = 'CONSUMED', cevp_consumed_at = $2, cevp_consumed_by = $3
	 WHERE cevp_preview_id = $1::uuid AND cevp_status = 'OPEN'`

// Consume implements erpintegration.PreviewConsumer. It returns
// ErrPreviewConsumed when no OPEN preview with that id exists.
func (r *ErpValuationPreviewRepository) Consume(ctx context.Context, previewID, actor string, at time.Time) error {
	if _, err := uuid.Parse(previewID); err != nil {
		return erpintegration.ErrPreviewConsumed
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return erpintegration.ErrActorRequired
	}
	res, err := r.db.ExecContext(ctx, erpPreviewConsumeSQL, previewID, at, actor)
	if err != nil {
		return fmt.Errorf("erp preview consume: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("erp preview consume rows affected: %w", err)
	}
	if n != 1 {
		return erpintegration.ErrPreviewConsumed
	}
	return nil
}
