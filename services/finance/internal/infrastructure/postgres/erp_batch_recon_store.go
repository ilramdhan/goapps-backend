package postgres

// erp_batch_recon_store.go adds the recon writes to the transaction-scoped
// batch store (plan-06 P5-T6; design §4.2 cesc_erp_* / cesc_recon_status,
// C-4). PG side only: nothing here talks to Oracle.

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

var _ erpintegration.ReconStore = (*erpBatchStore)(nil)

const erpReconClearSQL = `
	UPDATE cst_erp_std_cost
	   SET cesc_erp_rate = NULL, cesc_erp_rate_variants = NULL, cesc_erp_qty_kg = NULL,
	       cesc_erp_value = NULL, cesc_erp_flex13 = NULL, cesc_recon_status = NULL
	 WHERE cesc_batch_id = $1`

// erpReconApplySQL writes the recon columns from parallel arrays matched by
// key. Decimals travel as text and are cast server-side (no float).
const erpReconApplySQL = `
	UPDATE cst_erp_std_cost c
	   SET cesc_erp_rate = u.rate, cesc_erp_rate_variants = u.variants, cesc_erp_qty_kg = u.qty,
	       cesc_erp_value = u.val, cesc_erp_flex13 = u.flex, cesc_recon_status = u.status
	  FROM unnest($2::text[], $3::text[], $4::text[], $5::numeric[], $6::int[],
	              $7::numeric[], $8::numeric[], $9::text[], $10::text[])
	       AS u(item, grade, shade, rate, variants, qty, val, flex, status)
	 WHERE c.cesc_batch_id = $1
	   AND c.cesc_item_code = u.item AND c.cesc_grade_code = u.grade AND c.cesc_shade_code = u.shade`

// ApplyRecon clears the batch's recon columns and writes rows by key.
func (s *erpBatchStore) ApplyRecon(ctx context.Context, rows []erpintegration.ReconRow) (int64, error) {
	if _, err := s.tx.ExecContext(ctx, erpReconClearSQL, s.batchID); err != nil {
		return 0, fmt.Errorf("erp recon clear: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	n := len(rows)
	item, grade, shade, status := make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	rate, qty, val, flex := make([]sql.NullString, n), make([]sql.NullString, n), make([]sql.NullString, n), make([]sql.NullString, n)
	variants := make([]sql.NullInt32, n)
	for i, r := range rows {
		if r.Status != erpintegration.ReconMatch && r.Status != erpintegration.ReconDiff && r.Status != erpintegration.ReconNotInAdj {
			return 0, fmt.Errorf("erp recon apply: invalid status %q for %s", r.Status, r.Key)
		}
		item[i], grade[i], shade[i], status[i] = r.Key.ItemCode, r.Key.GradeCode, r.Key.ShadeCode, string(r.Status)
		rate[i], qty[i], val[i] = nullDecimalText(r.ErpRate), nullDecimalText(r.QtyKg), nullDecimalText(r.Value)
		flex[i] = nullString(r.Flex13)
		if r.RateVariants != nil {
			v := *r.RateVariants
			if v < 0 || v > 1<<31-1 {
				return 0, fmt.Errorf("erp recon apply: rate variants %d out of range for %s", v, r.Key)
			}
			variants[i] = sql.NullInt32{Int32: int32(v), Valid: true} //nolint:gosec // bounds checked above
		}
	}
	res, err := s.tx.ExecContext(ctx, erpReconApplySQL, s.batchID,
		pq.Array(item), pq.Array(grade), pq.Array(shade), pq.Array(rate), pq.Array(variants),
		pq.Array(qty), pq.Array(val), pq.Array(flex), pq.Array(status))
	if err != nil {
		return 0, fmt.Errorf("erp recon apply: %w", err)
	}
	got, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("erp recon apply rows: %w", err)
	}
	return got, nil
}
