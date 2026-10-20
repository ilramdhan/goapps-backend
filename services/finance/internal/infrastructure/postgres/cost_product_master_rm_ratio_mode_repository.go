package postgres

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

var _ costproductmaster.RMRatioModeRepository = (*CostProductMasterRepository)(nil)

// SaveRMRatioMode implements costproductmaster.RMRatioModeRepository: it
// writes cpm_rm_ratio_mode (000579; "" = NULL, follow product type) and
// cpm_updated_at/by only.
func (r *CostProductMasterRepository) SaveRMRatioMode(ctx context.Context, w costproductmaster.RMRatioModeWrite) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE cost_product_master SET
			cpm_rm_ratio_mode = NULLIF($2, ''),
			cpm_updated_at = $3, cpm_updated_by = $4
		WHERE cpm_product_sys_id = $1`,
		w.ProductSysID, w.Override, w.UpdatedAt, w.UpdatedBy)
	if err != nil {
		return fmt.Errorf("update RM ratio mode: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("RM ratio mode rows affected: %w", err)
	}
	if n == 0 {
		return costproductmaster.ErrNotFound
	}
	return nil
}
