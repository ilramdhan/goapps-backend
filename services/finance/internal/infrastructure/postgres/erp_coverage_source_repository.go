package postgres

import (
	"context"
	"fmt"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
)

// ErpCoverageSourceRepository implements erpintegration.CoverageSource over
// cost_product_master / cost_product_type / cst_product_cost (plan-04
// P3-T4; D-LINK). Read-only. The D-LINK key is cpm_erp_item_code +
// cpm_shade_code on active AX products; cpm_erp_grade_code_1/2 are never read.
type ErpCoverageSourceRepository struct{ db *DB }

// NewErpCoverageSourceRepository constructs the repository.
func NewErpCoverageSourceRepository(db *DB) *ErpCoverageSourceRepository {
	return &ErpCoverageSourceRepository{db: db}
}

var _ app.CoverageSource = (*ErpCoverageSourceRepository)(nil)

// erpCoverageResolveSQL uses the same predicate as cpmErpKeyPredicate
// (active, erp item set, AX-or-empty grade; trimmed/upper matching).
const erpCoverageResolveSQL = `
	SELECT k.item, k.shade, cpm.cpm_product_sys_id, cpm.cpm_product_code, COALESCE(cpt.cpt_type_code, '')
	FROM UNNEST($1::text[], $2::text[]) AS k(item, shade)
	JOIN cost_product_master cpm
	  ON cpm.cpm_is_active
	 AND cpm.cpm_erp_item_code IS NOT NULL
	 AND UPPER(TRIM(cpm.cpm_erp_item_code)) = k.item
	 AND UPPER(TRIM(COALESCE(cpm.cpm_shade_code, ''))) = k.shade
	 AND UPPER(COALESCE(NULLIF(TRIM(cpm.cpm_grade_code), ''), 'AX')) = 'AX'
	LEFT JOIN cost_product_type cpt ON cpt.cpt_type_id = cpm.cpm_product_type_id
	ORDER BY k.item, k.shade, cpm.cpm_product_sys_id`

// ResolveProducts returns, per normalized key, every active AX product
// holding it (0, 1 or several - the caller classifies).
func (r *ErpCoverageSourceRepository) ResolveProducts(ctx context.Context, keys []app.ErpProductKey) (out map[app.ErpProductKey][]app.ProductCandidate, err error) {
	out = make(map[app.ErpProductKey][]app.ProductCandidate, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	items := make([]string, len(keys))
	shades := make([]string, len(keys))
	for i, k := range keys {
		items[i], shades[i] = k.ItemCode, k.ShadeCode
	}
	rows, err := r.db.QueryContext(ctx, erpCoverageResolveSQL, pq.Array(items), pq.Array(shades))
	if err != nil {
		return nil, fmt.Errorf("resolve ERP products: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close ERP products: %w", cerr)
		}
	}()
	for rows.Next() {
		var k app.ErpProductKey
		var c app.ProductCandidate
		if err := rows.Scan(&k.ItemCode, &k.ShadeCode, &c.SysID, &c.ProductCode, &c.TypeCode); err != nil {
			return nil, fmt.Errorf("scan ERP product: %w", err)
		}
		out[k] = append(out[k], c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ERP products: %w", err)
	}
	return out, nil
}

// erpCoverageCostSQL reads the active (non-SUPERSEDED; uk_cpc_active makes it
// unique) ACTUAL cost of each product for the period.
const erpCoverageCostSQL = `
	SELECT cpc_product_sys_id, cpc_cost_id, cpc_version, cpc_status, cpc_currency_code, cpc_cost_per_unit
	FROM cst_product_cost
	WHERE cpc_period = $1
	  AND cpc_calculation_type = 'ACTUAL'
	  AND cpc_status <> 'SUPERSEDED'
	  AND cpc_product_sys_id = ANY($2::bigint[])`

// ActualCosts returns the active ACTUAL cost per product for the period.
func (r *ErpCoverageSourceRepository) ActualCosts(ctx context.Context, period string, productSysIDs []int64) (out map[int64]app.ActualCost, err error) {
	out = make(map[int64]app.ActualCost, len(productSysIDs))
	if len(productSysIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, erpCoverageCostSQL, period, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("load ACTUAL costs: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close ACTUAL costs: %w", cerr)
		}
	}()
	for rows.Next() {
		var pid int64
		var c app.ActualCost
		var v decimal.Decimal
		if err := rows.Scan(&pid, &c.CostID, &c.Version, &c.Status, &c.Currency, &v); err != nil {
			return nil, fmt.Errorf("scan ACTUAL cost: %w", err)
		}
		c.CostPerUnit = v
		out[pid] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ACTUAL costs: %w", err)
	}
	return out, nil
}
