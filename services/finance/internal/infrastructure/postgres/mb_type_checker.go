package postgres

import (
	"context"
	"fmt"

	"github.com/lib/pq"
)

// MBTypeChecker answers MB-typed questions for the calc-job trigger guard. It
// implements costcalc.MBTypeChecker.
//
// MB costs are produced exclusively by the MB_BATCH path ("MB Push to Head"), which
// computes ACTUAL/SELLING/FORECAST in one dependency-ordered run and pushes the results
// into cst_mb_cost. A generic calc job over the same product would be a second writer
// for the same (product, period, calc_type) and would supersede the APPROVED
// cst_product_cost row a push already consumed.
type MBTypeChecker struct {
	db *DB
}

// NewMBTypeChecker constructs the checker.
func NewMBTypeChecker(db *DB) *MBTypeChecker {
	return &MBTypeChecker{db: db}
}

// IsMBProduct reports whether the product is of the MB product type. A product with no
// master row is reported as not MB, matching the orchestrator's NOT EXISTS semantics.
func (c *MBTypeChecker) IsMBProduct(ctx context.Context, productSysID int64) (bool, error) {
	const q = `
		SELECT EXISTS (
		  SELECT 1
		  FROM cost_product_master pm
		  JOIN cost_product_type   pt ON pt.cpt_type_id = pm.cpm_product_type_id
		  WHERE pm.cpm_product_sys_id = $1 AND pt.cpt_type_code = $2
		)`
	var isMB bool
	if err := c.db.QueryRowContext(ctx, q, productSysID, mbCostProductTypeCode).Scan(&isMB); err != nil {
		return false, fmt.Errorf("check MB product %d: %w", productSysID, err)
	}
	return isMB, nil
}

// IsMBCostRow reports whether the cst_product_cost row identified by costID belongs to an
// MB-typed product. It implements costcalc.MBCostRowChecker, guarding the manual
// verify/approve RPCs.
//
// A missing cost row reports false so the repository's own ErrCostInvalidStatus /
// not-found handling stays the single source of truth for "no such cost".
func (c *MBTypeChecker) IsMBCostRow(ctx context.Context, costID int64) (bool, error) {
	const q = `
		SELECT EXISTS (
		  SELECT 1
		  FROM cst_product_cost  pc
		  JOIN cost_product_master pm ON pm.cpm_product_sys_id = pc.cpc_product_sys_id
		  JOIN cost_product_type   pt ON pt.cpt_type_id        = pm.cpm_product_type_id
		  WHERE pc.cpc_cost_id = $1 AND pt.cpt_type_code = $2
		)`
	var isMB bool
	if err := c.db.QueryRowContext(ctx, q, costID, mbCostProductTypeCode).Scan(&isMB); err != nil {
		return false, fmt.Errorf("check MB cost row %d: %w", costID, err)
	}
	return isMB, nil
}

// MBProductIDs returns the subset of productSysIDs that are MB-typed, as a set. It
// implements costcalc.MBProductSetChecker, which the chunk processor uses to refuse to
// persist a cst_product_cost row for an MB product pulled into a DAG as a dependency.
//
// One query per chunk, not per product. An empty input returns an empty set without
// touching the database.
func (c *MBTypeChecker) MBProductIDs(ctx context.Context, productSysIDs []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	const q = `
		SELECT pm.cpm_product_sys_id
		FROM cost_product_master pm
		JOIN cost_product_type   pt ON pt.cpt_type_id = pm.cpm_product_type_id
		WHERE pm.cpm_product_sys_id = ANY($1) AND pt.cpt_type_code = $2`
	rows, err := c.db.QueryContext(ctx, q, pq.Array(productSysIDs), mbCostProductTypeCode)
	if err != nil {
		return nil, fmt.Errorf("load MB product ids: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan MB product id: %w", err)
		}
		out[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate MB product ids: %w", err)
	}
	return out, nil
}

// IsMBProductType reports whether the product type id is the MB type.
func (c *MBTypeChecker) IsMBProductType(ctx context.Context, productTypeID int32) (bool, error) {
	const q = `SELECT EXISTS (SELECT 1 FROM cost_product_type WHERE cpt_type_id = $1 AND cpt_type_code = $2)`
	var isMB bool
	if err := c.db.QueryRowContext(ctx, q, productTypeID, mbCostProductTypeCode).Scan(&isMB); err != nil {
		return false, fmt.Errorf("check MB product type %d: %w", productTypeID, err)
	}
	return isMB, nil
}
