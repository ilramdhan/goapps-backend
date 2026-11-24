package postgres

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

var _ costproductmaster.FixCostMinThresholdRepository = (*CostProductMasterRepository)(nil)

// SaveFixCostMinThreshold implements costproductmaster.FixCostMinThresholdRepository:
// it writes cpm_fixcost_min_threshold (000615) and cpm_updated_at/by only.
func (r *CostProductMasterRepository) SaveFixCostMinThreshold(ctx context.Context, w costproductmaster.FixCostMinThresholdWrite) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE cost_product_master SET
			cpm_fixcost_min_threshold = $2,
			cpm_updated_at = $3, cpm_updated_by = $4
		WHERE cpm_product_sys_id = $1`,
		w.ProductSysID, w.Threshold, w.UpdatedAt, w.UpdatedBy)
	if err != nil {
		return fmt.Errorf("update fixcost min threshold: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("fixcost min threshold rows affected: %w", err)
	}
	if n == 0 {
		return costproductmaster.ErrNotFound
	}
	return nil
}
