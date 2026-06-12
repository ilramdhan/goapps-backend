package costproductparameter

import (
	"context"

	"github.com/google/uuid"
)

// Repository is the persistence contract for CPP_.
type Repository interface {
	// ListForProduct returns every active mst_parameter row (with is_period_dependent = FALSE)
	// joined to the existing CPP value when present. If requiredOnly is true, only
	// is_required_for_costing rows are returned.
	ListForProduct(ctx context.Context, productSysID int64, requiredOnly bool) ([]RequiredEntry, error)

	// GetMeta returns the joined mst_parameter snapshot for validation purposes.
	GetMeta(ctx context.Context, paramID uuid.UUID) (*ParamMeta, error)

	// ProductExists checks cost_product_master existence.
	ProductExists(ctx context.Context, productSysID int64) (bool, error)

	// Upsert performs an INSERT … ON CONFLICT (product_sys_id, param_id) DO UPDATE.
	Upsert(ctx context.Context, v *Value) error

	// Delete clears a single value row.
	Delete(ctx context.Context, productSysID int64, paramID uuid.UUID) error

	// MissingRequired returns mst_parameter rows that are required for costing but
	// have no value bound for the given product.
	MissingRequired(ctx context.Context, productSysID int64) ([]ParamMeta, error)

	// ApplicabilityRepository covers CAPP_ (per-product subset selection).
	AddApplicable(ctx context.Context, a *Applicability) error
	RemoveApplicable(ctx context.Context, productSysID int64, paramID uuid.UUID) error
	UpdateApplicable(ctx context.Context, productSysID int64, paramID uuid.UUID, isRequired *bool, displayOrder *int32, updatedBy string) error
	ListAvailableParams(ctx context.Context, productSysID int64) ([]ParamMeta, error)

	// CountApplicableForProducts returns the total number of applicable-param entries
	// for all products in the given slice. Used to pre-populate cft_total_params when
	// creating fill tasks so the fill-progress percentage is computed correctly.
	CountApplicableForProducts(ctx context.Context, productSysIDs []int64) (int32, error)
}
