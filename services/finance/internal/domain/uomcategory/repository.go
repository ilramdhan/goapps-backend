// Package uomcategory provides domain logic for UOM Category management.
package uomcategory

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the interface for UOM Category persistence.
// This interface is defined in domain layer, implemented in infrastructure layer.
type Repository interface {
	// Create persists a new UOM Category.
	Create(ctx context.Context, entity *Category) error

	// GetByID retrieves a UOM Category by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*Category, error)

	// GetByCode retrieves a UOM Category by its code.
	GetByCode(ctx context.Context, code Code) (*Category, error)

	// List retrieves UOM Categories with filtering, searching, and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Category, int64, error)

	// Update persists changes to an existing UOM Category.
	Update(ctx context.Context, entity *Category) error

	// SoftDelete marks a UOM Category as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByCode checks if a UOM Category with the given code exists.
	ExistsByCode(ctx context.Context, code Code) (bool, error)

	// ExistsByID checks if a UOM Category with the given ID exists.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)

	// ListAll retrieves all non-deleted UOM Categories (for export).
	ListAll(ctx context.Context, filter ExportFilter) ([]*Category, error)

	// IsInUse checks if a UOM Category is referenced by any UOM.
	IsInUse(ctx context.Context, id uuid.UUID) (bool, error)
}

// ListFilter contains filtering options for listing UOM Categories.
type ListFilter struct {
	// Search query (searches in code, name, description).
	Search string

	// IsActive filter.
	IsActive *bool

	// Pagination.
	Page     int
	PageSize int

	// Sorting.
	SortBy    string // "code", "name", "created_at"
	SortOrder string // "asc", "desc"
}

// ExportFilter contains filtering options for exporting UOM Categories.
type ExportFilter struct {
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
