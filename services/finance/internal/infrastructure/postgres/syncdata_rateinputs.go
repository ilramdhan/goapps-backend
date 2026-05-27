package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
)

// FetchRateInputs returns per-stage numerator/denominator pointers from
// cst_item_cons_stk_po scoped to (period, item_codes). The returned slice is the
// input to rmcost.AggregateRates; int is the number of source rows found.
//
// Implements appcost.SourceDataReader. Lives on SyncDataRepository because the
// data source is the Oracle-synced table owned by the syncdata bounded context.
func (r *SyncDataRepository) FetchRateInputs(
	ctx context.Context,
	period string,
	itemCodes []string,
) ([]rmcost.RateInputs, int, error) {
	if len(itemCodes) == 0 {
		return nil, 0, nil
	}

	placeholders := make([]string, len(itemCodes))
	args := make([]any, 0, len(itemCodes)+1)
	args = append(args, period)
	for i, code := range itemCodes {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, code)
	}

	query := fmt.Sprintf(`
		SELECT cons_qty, cons_val,
		       stores_qty, stores_val,
		       dept_qty, dept_val,
		       last_po_qty1, last_po_val1,
		       last_po_qty2, last_po_val2,
		       last_po_qty3, last_po_val3
		FROM cst_item_cons_stk_po
		WHERE period = $1 AND item_code IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch rate inputs: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var out []rmcost.RateInputs
	for rows.Next() {
		var in rmcost.RateInputs
		if err := rows.Scan(
			&in.ConsQty, &in.ConsVal,
			&in.StoresQty, &in.StoresVal,
			&in.DeptQty, &in.DeptVal,
			&in.PO1Qty, &in.PO1Val,
			&in.PO2Qty, &in.PO2Val,
			&in.PO3Qty, &in.PO3Val,
		); err != nil {
			return nil, 0, fmt.Errorf("scan rate inputs: %w", err)
		}
		out = append(out, in)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate rate inputs: %w", err)
	}

	return out, len(out), nil
}

// FetchItemUOMs returns a map of item_code -> uom pulled from
// cst_item_cons_stk_po for the given period. Items with a NULL or empty uom
// are omitted from the map so the caller can treat "missing" uniformly.
//
// Implements appcost.SourceDataReader.FetchItemUOMs.
func (r *SyncDataRepository) FetchItemUOMs(
	ctx context.Context,
	period string,
	itemCodes []string,
) (map[string]string, error) {
	if len(itemCodes) == 0 {
		return map[string]string{}, nil
	}

	placeholders := make([]string, len(itemCodes))
	args := make([]any, 0, len(itemCodes)+1)
	args = append(args, period)
	for i, code := range itemCodes {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, code)
	}

	query := fmt.Sprintf(`
		SELECT item_code, uom
		FROM cst_item_cons_stk_po
		WHERE period = $1 AND item_code IN (%s) AND uom IS NOT NULL AND uom <> ''
	`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch item uoms: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	out := make(map[string]string, len(itemCodes))
	for rows.Next() {
		var code, uom string
		if err := rows.Scan(&code, &uom); err != nil {
			return nil, fmt.Errorf("scan item uom: %w", err)
		}
		// First non-empty wins per item_code.
		if _, seen := out[code]; !seen {
			out[code] = uom
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate item uoms: %w", err)
	}
	return out, nil
}
