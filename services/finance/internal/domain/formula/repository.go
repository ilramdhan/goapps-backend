// Package formula provides domain logic for Formula management.
package formula

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the interface for Formula persistence.
type Repository interface {
	// Create persists a new Formula with its input parameters.
	Create(ctx context.Context, formula *Formula) error

	// GetByID retrieves a Formula by its ID (with joins).
	GetByID(ctx context.Context, id uuid.UUID) (*Formula, error)

	// GetByCode retrieves a Formula by its code (with joins).
	GetByCode(ctx context.Context, code Code) (*Formula, error)

	// List retrieves Formulas with filtering, searching, and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Formula, int64, error)

	// Update persists changes to an existing Formula and replaces input params.
	Update(ctx context.Context, formula *Formula) error

	// SoftDelete marks a Formula as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByCode checks if a Formula with the given code exists.
	ExistsByCode(ctx context.Context, code Code) (bool, error)

	// ExistsByID checks if a Formula with the given ID exists.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)

	// ListAll retrieves all non-deleted Formulas (for export).
	ListAll(ctx context.Context, filter ExportFilter) ([]*Formula, error)

	// ResultParamUsedByOther checks if a result_param_id is used by another formula (excluding excludeID).
	ResultParamUsedByOther(ctx context.Context, resultParamID uuid.UUID, excludeID uuid.UUID) (bool, error)

	// ResolveParamCode resolves a parameter code to its UUID. Returns ErrInputParamNotFound if not found.
	ResolveParamCode(ctx context.Context, paramCode string) (*uuid.UUID, error)

	// ParamExistsByID checks if a parameter with the given ID exists.
	ParamExistsByID(ctx context.Context, id uuid.UUID) (bool, error)
}

// ListFilter contains filtering options for listing Formulas.
type ListFilter struct {
	Search      string
	FormulaType *Type
	IsActive    *bool
	Page        int
	PageSize    int
	SortBy      string
	SortOrder   string
}

// ExportFilter contains filtering options for exporting Formulas.
type ExportFilter struct {
	FormulaType *Type
	IsActive    *bool
}

// NewListFilter creates a ListFilter with default values.
func NewListFilter() ListFilter {
	return ListFilter{
		Page:      1,
		PageSize:  10,
		SortBy:    "code",
		SortOrder: "asc",
	}
}

// Validate validates and normalizes the filter values.
func (f *ListFilter) Validate() {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 10
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	if f.SortBy == "" {
		f.SortBy = "code"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
