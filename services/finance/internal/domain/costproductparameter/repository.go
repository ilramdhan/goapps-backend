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

	// GetParamIDByCode resolves mst_parameter.param_code → UUID. Returns ErrParamNotFound
	// when no active parameter exists with that code.
	GetParamIDByCode(ctx context.Context, paramCode string) (uuid.UUID, error)

	// GetProductSysIDByCode resolves cost_product_master.cpm_product_code → sys_id.
	// Returns ErrProductNotFound when the product does not exist.
	GetProductSysIDByCode(ctx context.Context, productCode string) (int64, error)

	// ListApplicable returns all CAPP rows for a product as CAPPRow slices.
	// Used by the CAPP export handler to build the report.
	ListApplicable(ctx context.Context, productSysID int64) ([]CAPPRow, error)

	// ListAllApplicable returns all CAPP rows across all products.
	// Used for full dataset export.
	ListAllApplicable(ctx context.Context) ([]CAPPRow, error)

	// ListAllValues returns all CPP rows across all products.
	// Used for full dataset export.
	ListAllValues(ctx context.Context) ([]CPPRow, error)

	// GetParamCodeByID resolves a param UUID to its mst_parameter.param_code string.
	// Returns ErrParamNotFound when no active parameter exists with that ID.
	GetParamCodeByID(ctx context.Context, paramID uuid.UUID) (string, error)

	// GetCurrentValueAsText returns the current stored value for (productSysID, paramID)
	// formatted as a human-readable string. Returns empty string when no value exists.
	GetCurrentValueAsText(ctx context.Context, productSysID int64, paramID uuid.UUID) (string, error)

	// AddApplicableWithChildren adds a MASTER_LOOKUP param and all its fill-group children atomically.
	// fillGroupChildren contains the IDs of child params (may be empty for non-MASTER_LOOKUP params).
	AddApplicableWithChildren(ctx context.Context, productSysID int64, triggerParamID uuid.UUID, createdBy string, fillGroupChildren []uuid.UUID) error

	// GetRemovePreview returns trigger + child param info for the confirm dialog.
	GetRemovePreview(ctx context.Context, productSysID int64, paramID uuid.UUID) (RemovePreview, error)

	// RemoveApplicableWithChildren removes a MASTER_LOOKUP param + all children + their CPP values atomically.
	RemoveApplicableWithChildren(ctx context.Context, productSysID int64, triggerParamID uuid.UUID, deletedBy string) error

	// BulkUpsertValues upserts multiple CPP value rows by (cpp_product_sys_id, cpp_param_id).
	// Returns counts of rows inserted and updated.
	BulkUpsertValues(ctx context.Context, items []CPPUpsertInput, actor string) (inserted, updated int, err error)

	// BulkUpsertApplicable upserts CAPP rows by (capp_product_sys_id, capp_param_id).
	// Returns counts of rows inserted and updated.
	BulkUpsertApplicable(ctx context.Context, items []CAPPUpsertInput, actor string) (inserted, updated int, err error)
}
