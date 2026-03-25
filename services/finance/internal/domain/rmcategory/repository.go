// Package rmcategory provides domain logic for Raw Material Category management.
package rmcategory

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the interface for RMCategory persistence.
// This interface is defined in domain layer, implemented in infrastructure layer.
type Repository interface {
	// Create persists a new RMCategory.
	Create(ctx context.Context, entity *RMCategory) error

	// GetByID retrieves an RMCategory by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*RMCategory, error)

	// GetByCode retrieves an RMCategory by its code.
	GetByCode(ctx context.Context, code Code) (*RMCategory, error)

	// List retrieves RMCategories with filtering, searching, and pagination.
	List(ctx context.Context, filter ListFilter) ([]*RMCategory, int64, error)

	// Update persists changes to an existing RMCategory.
	Update(ctx context.Context, entity *RMCategory) error

	// SoftDelete marks an RMCategory as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByCode checks if an RMCategory with the given code exists.
	ExistsByCode(ctx context.Context, code Code) (bool, error)

	// ExistsByID checks if an RMCategory with the given ID exists.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)

	// ListAll retrieves all non-deleted RMCategories (for export).
	ListAll(ctx context.Context, filter ExportFilter) ([]*RMCategory, error)
}

// ListFilter contains filtering options for listing RMCategories.
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

// ExportFilter contains filtering options for exporting RMCategories.
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
