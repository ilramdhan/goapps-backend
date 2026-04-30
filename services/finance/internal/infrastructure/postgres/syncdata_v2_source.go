// Package postgres — V2 per-(item, grade) source fetcher for the V2 RM cost engine.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ItemGradeKey is the natural key for an RM variant in the sync feed.
type ItemGradeKey struct {
	ItemCode  string
	GradeCode string // "" for variants with NULL/empty grade_code
}

// V2SourceQty is the per-detail source-quantity bag the V2 engine needs.
type V2SourceQty struct {
	ConsVal  float64
	ConsQty  float64
	StockVal float64
	StockQty float64
	POVal    float64
	POQty    float64
}

// FetchSourceQtyByItemGrade returns one record per (item_code, grade_code) in
// the sync feed for the given period, keyed by (item_code, COALESCE(grade_code,'')).
// Stock = STORES stage in cst_item_cons_stk_po. PO = PO_1 (first PO slot).
//
// Used by the V2 RM cost engine — replaces the aggregate FetchRateInputs.
func (r *SyncDataRepository) FetchSourceQtyByItemGrade(
	ctx context.Context,
	period string,
	keys []ItemGradeKey,
) (map[ItemGradeKey]V2SourceQty, error) {
	if len(keys) == 0 {
		return map[ItemGradeKey]V2SourceQty{}, nil
	}

	// Build IN clause on item_code only — we then pick the right grade_code in Go.
	codes := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		codes[k.ItemCode] = struct{}{}
	}
	placeholders := make([]string, 0, len(codes))
	args := make([]any, 0, len(codes)+1)
	args = append(args, period)
	idx := 2
	for code := range codes {
		placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
		args = append(args, code)
		idx++
	}

	query := fmt.Sprintf(`
		SELECT item_code, COALESCE(grade_code, '') AS grade_code,
		       cons_val, cons_qty,
		       stores_val, stores_qty,
		       last_po_val1, last_po_qty1
		FROM cst_item_cons_stk_po
		WHERE period = $1 AND item_code IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch v2 source: %w", err)
	}
	defer closeRows(rows)

	out := make(map[ItemGradeKey]V2SourceQty, len(keys))
	for rows.Next() {
		var (
			code, grade                                       string
			consVal, consQty, stockVal, stockQty, poVal, poQty sql.NullFloat64
		)
		if err := rows.Scan(&code, &grade, &consVal, &consQty, &stockVal, &stockQty, &poVal, &poQty); err != nil {
			return nil, fmt.Errorf("scan v2 source: %w", err)
		}
		out[ItemGradeKey{ItemCode: code, GradeCode: grade}] = V2SourceQty{
			ConsVal:  nullFloatOrZero(consVal),
			ConsQty:  nullFloatOrZero(consQty),
			StockVal: nullFloatOrZero(stockVal),
			StockQty: nullFloatOrZero(stockQty),
			POVal:    nullFloatOrZero(poVal),
			POQty:    nullFloatOrZero(poQty),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate v2 source: %w", err)
	}
	return out, nil
}
