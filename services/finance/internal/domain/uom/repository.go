// Package uom provides domain logic for Unit of Measure management.
package uom

import (
	"context"

	"github.com/google/uuid"
)

// CategoryRepository defines read-only access to UOM Category data.
// Used by UOM handlers that need to resolve category codes to IDs.
type CategoryRepository interface {
	// GetCategoryIDByCode resolves a category code to its UUID.
	GetCategoryIDByCode(ctx context.Context, code string) (uuid.UUID, error)
}

// Repository defines the interface for UOM persistence.
// This interface is defined in domain layer, implemented in infrastructure layer.
type Repository interface {
	// Create persists a new UOM.
	Create(ctx context.Context, uom *UOM) error

	// GetByID retrieves a UOM by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*UOM, error)

	// GetByCode retrieves a UOM by its code.
	GetByCode(ctx context.Context, code Code) (*UOM, error)

	// List retrieves UOMs with filtering, searching, and pagination.
	List(ctx context.Context, filter ListFilter) ([]*UOM, int64, error)

	// Update persists changes to an existing UOM.
	Update(ctx context.Context, uom *UOM) error

	// SoftDelete marks a UOM as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByCode checks if a UOM with the given code exists.
	ExistsByCode(ctx context.Context, code Code) (bool, error)

	// ExistsByID checks if a UOM with the given ID exists.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)

	// ListAll retrieves all non-deleted UOMs (for export).
	ListAll(ctx context.Context, filter ExportFilter) ([]*UOM, error)
}

// ListFilter contains filtering options for listing UOMs.
type ListFilter struct {
	// Search query (searches in code, name, description).
	Search string

	// CategoryID filter (FK to mst_uom_category).
	CategoryID *uuid.UUID

	// IsActive filter.
	IsActive *bool

	// Pagination.
	Page     int
	PageSize int

	// Sorting.
	SortBy    string // "code", "name", "category", "created_at"
	SortOrder string // "asc", "desc"
}

// ExportFilter contains filtering options for exporting UOMs.
type ExportFilter struct {
	// CategoryID filter (FK to mst_uom_category).
	CategoryID *uuid.UUID

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
