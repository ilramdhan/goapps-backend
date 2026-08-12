package postgres

import (
	"context"
	"fmt"
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

// IsMBProductType reports whether the product type id is the MB type.
func (c *MBTypeChecker) IsMBProductType(ctx context.Context, productTypeID int32) (bool, error) {
	const q = `SELECT EXISTS (SELECT 1 FROM cost_product_type WHERE cpt_type_id = $1 AND cpt_type_code = $2)`
	var isMB bool
	if err := c.db.QueryRowContext(ctx, q, productTypeID, mbCostProductTypeCode).Scan(&isMB); err != nil {
		return false, fmt.Errorf("check MB product type %d: %w", productTypeID, err)
	}
	return isMB, nil
}
