// Package parameter provides domain logic for Parameter management.
package parameter

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the interface for Parameter persistence.
type Repository interface {
	// Create persists a new Parameter.
	Create(ctx context.Context, param *Parameter) error

	// GetByID retrieves a Parameter by its ID (with UOM join).
	GetByID(ctx context.Context, id uuid.UUID) (*Parameter, error)

	// GetByCode retrieves a Parameter by its code (with UOM join).
	GetByCode(ctx context.Context, code Code) (*Parameter, error)

	// List retrieves Parameters with filtering, searching, and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Parameter, int64, error)

	// Update persists changes to an existing Parameter.
	Update(ctx context.Context, param *Parameter) error

	// SoftDelete marks a Parameter as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByCode checks if a Parameter with the given code exists.
	ExistsByCode(ctx context.Context, code Code) (bool, error)

	// ExistsByID checks if a Parameter with the given ID exists.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)

	// ListAll retrieves all non-deleted Parameters (for export).
	ListAll(ctx context.Context, filter ExportFilter) ([]*Parameter, error)

	// ResolveUOMCode resolves a UOM code to its UUID. Returns nil if not found.
	ResolveUOMCode(ctx context.Context, uomCode string) (*uuid.UUID, error)
}

// ListFilter contains filtering options for listing Parameters.
type ListFilter struct {
	// Search query (searches in code, name, short_name).
	Search string

	// DataType filter.
	DataType *DataType

	// ParamCategory filter.
	ParamCategory *ParamCategory

	// IsActive filter.
	IsActive *bool

	// Pagination.
	Page     int
	PageSize int

	// Sorting.
	SortBy    string // "code", "name", "category", "data_type", "created_at"
	SortOrder string // "asc", "desc"
}

// ExportFilter contains filtering options for exporting Parameters.
type ExportFilter struct {
	// DataType filter.
	DataType *DataType

	// ParamCategory filter.
	ParamCategory *ParamCategory

	// IsActive filter.
	IsActive *bool
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
