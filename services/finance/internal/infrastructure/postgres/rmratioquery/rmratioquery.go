// Package rmratioquery holds the SQL that feeds the DENIER RM ratio mode
// (costroute.ComputeSeqRMRatios). Both the calc engine's bulk loader
// (application/costcalc) and the route graph ratio loader (postgres) call
// these functions, so the two read paths can never diverge.
//
// It is a leaf package (database/sql + lib/pq only) so costcalc can import it
// without pulling in the full postgres package.
package rmratioquery

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

// Querier is the read subset of *sql.DB / *sql.Tx used here.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// ProductRatioModes returns cost_product_type.cpt_rm_ratio_mode per product
// (upper-cased, trimmed) in one query. Unknown products are absent, which
// callers treat as INPUT.
func ProductRatioModes(ctx context.Context, q Querier, productSysIDs []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	const sqlText = `
		SELECT pm.cpm_product_sys_id, pt.cpt_rm_ratio_mode
		FROM cost_product_master pm
		JOIN cost_product_type   pt ON pt.cpt_type_id = pm.cpm_product_type_id
		WHERE pm.cpm_product_sys_id = ANY($1)`
	rows, err := q.QueryContext(ctx, sqlText, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("load product RM ratio modes: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var (
			productSysID int64
			mode         string
		)
		if err := rows.Scan(&productSysID, &mode); err != nil {
			return nil, fmt.Errorf("scan product RM ratio mode: %w", err)
		}
		out[productSysID] = strings.ToUpper(strings.TrimSpace(mode))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product RM ratio modes: %w", err)
	}
	return out, nil
}

// GroupDeniers returns cst_rm_group_head.denier per group_code in one query.
// Groups whose denier is NULL, soft-deleted groups and unknown codes are
// absent, so presence in the map is what makes a GROUP line a denier line.
func GroupDeniers(ctx context.Context, q Querier, groupCodes []string) (map[string]float64, error) {
	out := map[string]float64{}
	if len(groupCodes) == 0 {
		return out, nil
	}
	const sqlText = `
		SELECT group_code, denier::float8
		FROM cst_rm_group_head
		WHERE group_code = ANY($1)
		  AND deleted_at IS NULL
		  AND denier IS NOT NULL`
	rows, err := q.QueryContext(ctx, sqlText, pq.Array(groupCodes))
	if err != nil {
		return nil, fmt.Errorf("load RM group deniers: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var (
			code   string
			denier float64
		)
		if err := rows.Scan(&code, &denier); err != nil {
			return nil, fmt.Errorf("scan RM group denier: %w", err)
		}
		out[code] = denier
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate RM group deniers: %w", err)
	}
	return out, nil
}

// ProductParamValues returns one numeric product-master parameter per product
// in one query: the cost_product_parameter value of paramCode for every
// product that has it applicable (cost_product_applicable_param). The value
// expression and filters mirror the calc engine's LoadCAPP, so a param read
// here is exactly what CAPP would carry. Products without a value are absent.
func ProductParamValues(ctx context.Context, q Querier, productSysIDs []int64, paramCode string) (map[int64]float64, error) {
	out := map[int64]float64{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	const sqlText = `
		SELECT capp.capp_product_sys_id,
		       COALESCE(cpp.cpp_value_numeric,
		                 CASE WHEN cpp.cpp_value_flag THEN 1.0 ELSE 0.0 END)
		FROM cost_product_applicable_param capp
		JOIN mst_parameter mp ON mp.id = capp.capp_param_id
		JOIN cost_product_parameter cpp
		     ON cpp.cpp_product_sys_id = capp.capp_product_sys_id
		    AND cpp.cpp_param_id = capp.capp_param_id
		WHERE capp.capp_product_sys_id = ANY($1)
		  AND mp.param_code = $2
		  AND (cpp.cpp_value_numeric IS NOT NULL OR cpp.cpp_value_flag IS NOT NULL)
		  AND mp.deleted_at IS NULL`
	rows, err := q.QueryContext(ctx, sqlText, pq.Array(productSysIDs), paramCode)
	if err != nil {
		return nil, fmt.Errorf("load product param %s: %w", paramCode, err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var (
			productSysID int64
			val          float64
		)
		if err := rows.Scan(&productSysID, &val); err != nil {
			return nil, fmt.Errorf("scan product param %s: %w", paramCode, err)
		}
		out[productSysID] = val
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product param %s: %w", paramCode, err)
	}
	return out, nil
}

func closeRows(rows *sql.Rows) {
	if cerr := rows.Close(); cerr != nil {
		_ = cerr
	}
}
