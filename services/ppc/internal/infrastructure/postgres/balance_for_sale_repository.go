package postgres

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/balanceforsale"
)

// BalanceForSaleRepository gathers the balance-for-sale components from PPC
// planning data: running-WO production estimate, confirmed MTS plan quantity,
// and committed contract demand. current_stock_AX is NOT sourced here (no Orion
// inventory ETL in scope) and stays 0 in the returned components.
type BalanceForSaleRepository struct {
	db *DB
}

// NewBalanceForSaleRepository creates a new BalanceForSaleRepository.
func NewBalanceForSaleRepository(db *DB) *BalanceForSaleRepository {
	return &BalanceForSaleRepository{db: db}
}

// CommodityProduct identifies a commodity-watch product for the balance view.
type CommodityProduct struct {
	CpmProductSysID  int64
	IsCommodityWatch bool
}

// ListCommodityProducts returns the cpm_product_sys_id set on the PPC watchlist
// (product_ppc_config.ppc_is_commodity_watch = true). Used when the caller
// requests the commodity-only view.
func (r *BalanceForSaleRepository) ListCommodityProducts(ctx context.Context) ([]int64, error) {
	const query = `
		SELECT ppc_cpm_product_sys_id
		FROM product_ppc_config
		WHERE ppc_is_commodity_watch = TRUE
		ORDER BY ppc_cpm_product_sys_id`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list commodity products: %w", err)
	}
	defer closeRows(rows)

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan commodity product: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commodity products: %w", err)
	}
	return ids, nil
}

// GatherComponents aggregates the three PPC-sourced balance components per
// product. When productIDs is non-empty the aggregation is scoped to those
// products; otherwise it spans every product appearing in the planning data.
// current_stock_AX is left 0 (out of scope — Orion stock ETL). The returned map
// is keyed by cpm_product_sys_id.
func (r *BalanceForSaleRepository) GatherComponents(ctx context.Context, productIDs []int64) (map[int64]*balanceforsale.Components, error) {
	acc := make(map[int64]*balanceforsale.Components)
	filter := productIDs
	if err := r.accumulateRunningWO(ctx, filter, acc); err != nil {
		return nil, err
	}
	if err := r.accumulateMtsPlan(ctx, filter, acc); err != nil {
		return nil, err
	}
	if err := r.accumulateCommittedContract(ctx, filter, acc); err != nil {
		return nil, err
	}
	return acc, nil
}

// component returns (creating if needed) the accumulator for a product id.
func component(acc map[int64]*balanceforsale.Components, id int64) *balanceforsale.Components {
	c, ok := acc[id]
	if !ok {
		c = &balanceforsale.Components{CpmProductSysID: id}
		acc[id] = c
	}
	return c
}

// accumulateRunningWO sums the estimated remaining output of RUNNING work orders
// (wo_qty_target − confirmed produced qty_actual) per product.
func (r *BalanceForSaleRepository) accumulateRunningWO(ctx context.Context, ids []int64, acc map[int64]*balanceforsale.Components) error {
	query := `
		SELECT ppi.ppi_cpm_product_sys_id,
			COALESCE(SUM(GREATEST(wo.wo_qty_target - COALESCE(prod.qty_actual, 0), 0)), 0)
		FROM work_order wo
		JOIN production_plan_item ppi ON ppi.ppi_id = wo.wo_plan_item_id
		LEFT JOIN (
			SELECT wpa_wo_id, SUM(COALESCE(wpa_qty_actual, 0)) AS qty_actual
			FROM wo_production_actual GROUP BY wpa_wo_id
		) prod ON prod.wpa_wo_id = wo.wo_id
		WHERE wo.wo_status = 'RUNNING'`
	args := appendProductFilter(&query, "ppi.ppi_cpm_product_sys_id", ids)
	query += ` GROUP BY ppi.ppi_cpm_product_sys_id`
	return r.scanInto(ctx, query, args, acc, func(c *balanceforsale.Components, v float64) {
		c.WORunningOutputEst += v
	})
}

// accumulateMtsPlan sums confirmed MTS plan-item target quantities per product.
func (r *BalanceForSaleRepository) accumulateMtsPlan(ctx context.Context, ids []int64, acc map[int64]*balanceforsale.Components) error {
	query := `
		SELECT ppi_cpm_product_sys_id, COALESCE(SUM(ppi_qty_target), 0)
		FROM production_plan_item
		WHERE ppi_type = 'MTS' AND ppi_status = 'CONFIRMED'`
	args := appendProductFilter(&query, "ppi_cpm_product_sys_id", ids)
	query += ` GROUP BY ppi_cpm_product_sys_id`
	return r.scanInto(ctx, query, args, acc, func(c *balanceforsale.Components, v float64) {
		c.MtsPlanQty += v
	})
}

// accumulateCommittedContract sums committed contract demand (IN_PRODUCTION +
// CONFIRMED) remaining quantity per product.
func (r *BalanceForSaleRepository) accumulateCommittedContract(ctx context.Context, ids []int64, acc map[int64]*balanceforsale.Components) error {
	query := `
		SELECT pd_cpm_product_sys_id, COALESCE(SUM(pd_qty_remaining), 0)
		FROM production_demand
		WHERE pd_type = 'CONTRACT' AND pd_status IN ('IN_PRODUCTION', 'CONFIRMED')`
	args := appendProductFilter(&query, "pd_cpm_product_sys_id", ids)
	query += ` GROUP BY pd_cpm_product_sys_id`
	return r.scanInto(ctx, query, args, acc, func(c *balanceforsale.Components, v float64) {
		c.CommittedContractQty += v
	})
}

// scanInto runs an aggregation query yielding (product_id, value) rows and folds
// each value into the accumulator via apply.
func (r *BalanceForSaleRepository) scanInto(
	ctx context.Context, query string, args []any,
	acc map[int64]*balanceforsale.Components, apply func(*balanceforsale.Components, float64),
) error {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("balance-for-sale aggregate: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var (
			productID int64
			value     float64
		)
		if err := rows.Scan(&productID, &value); err != nil {
			return fmt.Errorf("scan balance-for-sale aggregate: %w", err)
		}
		apply(component(acc, productID), value)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate balance-for-sale aggregate: %w", err)
	}
	return nil
}

// appendProductFilter appends an IN (...) product-id filter to a query that
// already has a WHERE clause and returns the positional args. An empty id slice
// adds no filter.
func appendProductFilter(query *string, column string, ids []int64) []any {
	if len(ids) == 0 {
		return nil
	}
	*query += " AND " + column + " IN ("
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		if i > 0 {
			*query += ","
		}
		*query += fmt.Sprintf("$%d", i+1)
		args = append(args, id)
	}
	*query += ")"
	return args
}
