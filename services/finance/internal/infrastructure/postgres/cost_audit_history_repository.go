package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/pkg/costcalc/metrics"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// CostAuditHistoryRepository persists recompute audit rows to `aud_cost_history`.
type CostAuditHistoryRepository struct {
	db *DB
}

// NewCostAuditHistoryRepository constructs a CostAuditHistoryRepository.
func NewCostAuditHistoryRepository(db *DB) *CostAuditHistoryRepository {
	return &CostAuditHistoryRepository{db: db}
}

var _ costcalc.AuditHistoryRepository = (*CostAuditHistoryRepository)(nil)

// Write inserts a single aud_cost_history row. Zero IDs are stored as NULL
// (e.g. the first cost result has no previous cost id). A nil
// AuditHistoryEntry.VariancePct is likewise stored as NULL, meaning the variance was not
// computable (the previous total was 0); a stored 0.0 means the cost truly did not change.
func (r *CostAuditHistoryRepository) Write(ctx context.Context, e *costcalc.AuditHistoryEntry) error {
	start := time.Now()
	defer func() {
		metrics.DBTxSeconds.WithLabelValues("audit").Observe(time.Since(start).Seconds())
	}()
	if e == nil {
		return fmt.Errorf("write audit history: nil entry")
	}
	const q = `
		INSERT INTO aud_cost_history (
			ach_product_sys_id, ach_period, ach_calc_type,
			ach_old_cost_id, ach_new_cost_id,
			ach_old_total, ach_new_total, ach_variance_pct,
			ach_old_job_id, ach_new_job_id,
			ach_change_reason, ach_changed_by
		) VALUES (
			$1, $2, $3,
			NULLIF($4, 0)::BIGINT, NULLIF($5, 0)::BIGINT,
			$6, $7, $8,
			NULLIF($9, 0)::BIGINT, NULLIF($10, 0)::BIGINT,
			NULLIF($11, ''), $12
		)`
	if _, err := r.db.ExecContext(ctx, q,
		e.ProductSysID, e.Period, string(e.CalcType),
		e.OldCostID, e.NewCostID,
		e.OldTotal, e.NewTotal, variancePctArg(e.VariancePct),
		e.OldJobID, e.NewJobID,
		e.ChangeReason, e.ChangedBy,
	); err != nil {
		return fmt.Errorf("insert audit history: %w", err)
	}
	metrics.AuditWritesTotal.Inc()
	return nil
}

// variancePctArg converts the optional variance into a driver argument: nil becomes a
// typed NULL, a value becomes that value. database/sql already maps a nil *float64 to
// NULL, but going through sql.NullFloat64 states the intent at the call site.
func variancePctArg(v *float64) sql.NullFloat64 {
	if v == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *v, Valid: true}
}
