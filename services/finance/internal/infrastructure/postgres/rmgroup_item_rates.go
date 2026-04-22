package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	appgroup "github.com/mutugading/goapps-backend/services/finance/internal/application/rmgroup"
)

// Verify interface compliance at compile time.
var _ appgroup.GroupItemRatesReader = (*SyncDataRepository)(nil)

// ListGroupItemRates joins active (non-deleted) details of a group with the
// period's Oracle sync rates. Details without a sync row produce zero rates.
func (r *SyncDataRepository) ListGroupItemRates(
	ctx context.Context,
	headID uuid.UUID,
	period string,
) ([]*appgroup.GroupItemRates, error) {
	const q = `
		SELECT
		  d.item_code,
		  COALESCE(d.item_name, ''),
		  COALESCE(d.grade_code, ''),
		  COALESCE(d.item_grade, ''),
		  COALESCE(d.uom_code, ''),
		  d.is_active,
		  d.is_dummy,
		  COALESCE(s.period, ''),
		  COALESCE(s.cons_qty, 0),    COALESCE(s.cons_val, 0),    COALESCE(s.cons_rate, 0),
		  COALESCE(s.stores_qty, 0),  COALESCE(s.stores_val, 0),  COALESCE(s.stores_rate, 0),
		  COALESCE(s.dept_qty, 0),    COALESCE(s.dept_val, 0),    COALESCE(s.dept_rate, 0),
		  COALESCE(s.last_po_qty1, 0), COALESCE(s.last_po_val1, 0), COALESCE(s.last_po_rate1, 0),
		  COALESCE(s.last_po_qty2, 0), COALESCE(s.last_po_val2, 0), COALESCE(s.last_po_rate2, 0),
		  COALESCE(s.last_po_qty3, 0), COALESCE(s.last_po_val3, 0), COALESCE(s.last_po_rate3, 0)
		FROM cst_rm_group_detail d
		LEFT JOIN cst_item_cons_stk_po s
		  ON s.item_code = d.item_code
		 AND COALESCE(s.grade_code, '') = COALESCE(d.grade_code, '')
		 AND s.period = $2
		WHERE d.group_head_id = $1
		  AND d.deleted_at IS NULL
		ORDER BY d.sort_order, d.item_code, d.grade_code
	`
	rows, err := r.db.QueryContext(ctx, q, headID, period)
	if err != nil {
		return nil, fmt.Errorf("query group item rates: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var out []*appgroup.GroupItemRates
	for rows.Next() {
		row, scanErr := scanGroupItemRates(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan group item rates: %w", scanErr)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group item rates: %w", err)
	}
	return out, nil
}

func scanGroupItemRates(rows *sql.Rows) (*appgroup.GroupItemRates, error) {
	var r appgroup.GroupItemRates
	if err := rows.Scan(
		&r.ItemCode, &r.ItemName, &r.GradeCode, &r.ItemGrade, &r.UOMCode,
		&r.IsActive, &r.IsDummy, &r.Period,
		&r.ConsQty, &r.ConsVal, &r.ConsRate,
		&r.StoresQty, &r.StoresVal, &r.StoresRate,
		&r.DeptQty, &r.DeptVal, &r.DeptRate,
		&r.LastPOQty1, &r.LastPOVal1, &r.LastPORate1,
		&r.LastPOQty2, &r.LastPOVal2, &r.LastPORate2,
		&r.LastPOQty3, &r.LastPOVal3, &r.LastPORate3,
	); err != nil {
		return nil, err
	}
	return &r, nil
}
