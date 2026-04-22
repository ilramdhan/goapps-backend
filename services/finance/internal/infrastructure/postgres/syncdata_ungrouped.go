package postgres

import (
	"context"
	"fmt"
	"strings"

	appgroup "github.com/mutugading/goapps-backend/services/finance/internal/application/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

// Verify interface compliance at compile time.
var _ appgroup.UngroupedItemsReader = (*SyncDataRepository)(nil)

// ListUngroupedItems returns synced raw-material rows from cst_item_cons_stk_po
// that have no active (non-deleted) entry in cst_rm_group_detail. Used by the
// Ungrouped Items report to seed operators' grouping decisions.
func (r *SyncDataRepository) ListUngroupedItems(
	ctx context.Context,
	filter appgroup.UngroupedItemsFilter,
) ([]*syncdata.ItemConsStockPO, int64, error) {
	filter.Validate()

	var conds []string
	var args []any
	idx := 1

	if filter.Period != "" {
		conds = append(conds, fmt.Sprintf("s.period = $%d", idx))
		args = append(args, filter.Period)
		idx++
	}
	if filter.Search != "" {
		conds = append(conds, fmt.Sprintf(
			"(s.item_code ILIKE $%d OR s.item_name ILIKE $%d OR s.grade_code ILIKE $%d OR s.grade_name ILIKE $%d)",
			idx, idx, idx, idx,
		))
		args = append(args, "%"+filter.Search+"%")
		idx++
	}

	where := "WHERE d.group_detail_id IS NULL"
	if len(conds) > 0 {
		where += " AND " + strings.Join(conds, " AND ")
	}

	// Left-join on (item_code, grade_code) as the natural key — this matches
	// the unique index from migration 000018. Variants of the same item_code
	// with different grade_codes stay independently visible, so once variant
	// A is grouped, variant B still appears here waiting to be assigned.
	// COALESCE pins NULL grade_code to '' symmetrically on both sides.
	const joinClause = `
		FROM cst_item_cons_stk_po s
		LEFT JOIN cst_rm_group_detail d
		  ON d.item_code = s.item_code
		 AND COALESCE(d.grade_code, '') = COALESCE(s.grade_code, '')
		 AND d.is_active = true
		 AND d.deleted_at IS NULL
	`

	var total int64
	countSQL := "SELECT COUNT(*) " + joinClause + " " + where
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count ungrouped items: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize
	listSQL := `
		SELECT s.period, s.item_code, s.grade_code, s.grade_name, s.item_name, s.uom,
		       s.cons_qty, s.cons_val, s.cons_rate,
		       s.stores_qty, s.stores_val, s.stores_rate,
		       s.dept_qty, s.dept_val, s.dept_rate,
		       s.last_po_qty1, s.last_po_val1, s.last_po_rate1, s.last_po_dt1,
		       s.last_po_qty2, s.last_po_val2, s.last_po_rate2, s.last_po_dt2,
		       s.last_po_qty3, s.last_po_val3, s.last_po_rate3, s.last_po_dt3,
		       s.synced_at, s.synced_by_job
	` + joinClause + " " + where +
		fmt.Sprintf(" ORDER BY s.period DESC, s.item_code, s.grade_code LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, filter.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list ungrouped items: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var items []*syncdata.ItemConsStockPO
	for rows.Next() {
		item, scanErr := r.scanSyncedRow(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan ungrouped row: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate ungrouped rows: %w", err)
	}

	return items, total, nil
}
