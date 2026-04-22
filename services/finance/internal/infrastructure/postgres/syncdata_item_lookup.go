package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

// GetItemByCode returns the most recent sync record for the given item_code,
// prefers a row whose non-null qty columns are populated (so we don't latch
// onto a low-value variant when an enriched variant exists). Retained as a
// compatibility shim that delegates to GetItemByCodeGrade with "".
func (r *SyncDataRepository) GetItemByCode(ctx context.Context, itemCode string) (*syncdata.ItemConsStockPO, error) {
	return r.GetItemByCodeGrade(ctx, itemCode, "")
}

// GetItemByCodeGrade returns the sync record matching (item_code, grade_code).
// When gradeCode is empty, falls back to the most recent variant with the
// largest cons_qty + stores_qty (so metadata backfill picks the enriched row
// instead of an arbitrary one).
func (r *SyncDataRepository) GetItemByCodeGrade(ctx context.Context, itemCode, gradeCode string) (*syncdata.ItemConsStockPO, error) {
	var row *sql.Row
	if gradeCode != "" {
		row = r.db.QueryRowContext(ctx, `
			SELECT period, item_code, grade_code, grade_name, item_name, uom
			FROM cst_item_cons_stk_po
			WHERE item_code = $1 AND COALESCE(grade_code,'') = $2
			ORDER BY period DESC
			LIMIT 1
		`, itemCode, gradeCode)
	} else {
		row = r.db.QueryRowContext(ctx, `
			SELECT period, item_code, grade_code, grade_name, item_name, uom
			FROM cst_item_cons_stk_po
			WHERE item_code = $1
			ORDER BY period DESC,
			         (COALESCE(cons_qty,0) + COALESCE(stores_qty,0)) DESC
			LIMIT 1
		`, itemCode)
	}

	var (
		period    string
		code      string
		grade     sql.NullString
		gradeName sql.NullString
		itemName  sql.NullString
		uom       sql.NullString
	)
	if err := row.Scan(&period, &code, &grade, &gradeName, &itemName, &uom); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // not found — return nil, no error
		}
		return nil, fmt.Errorf("get item by code: %w", err)
	}

	return &syncdata.ItemConsStockPO{
		Period:    period,
		ItemCode:  code,
		GradeCode: grade.String,
		GradeName: gradeName.String,
		ItemName:  itemName.String,
		UOM:       uom.String,
	}, nil
}
