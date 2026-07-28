// Package wastecategory provides domain logic for PPC waste-category master data.
package wastecategory

import "context"

// Repository defines persistence operations for waste categories.
type Repository interface {
	// Create persists a new waste category and assigns its ID.
	Create(ctx context.Context, entity *Category) error

	// GetByID retrieves a waste category by its ID.
	GetByID(ctx context.Context, id int64) (*Category, error)

	// List retrieves waste categories with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Category, int64, error)

	// Update persists changes to an existing waste category.
	Update(ctx context.Context, entity *Category) error

	// Delete removes a waste category by its ID.
	Delete(ctx context.Context, id int64) error
}

// ListFilter contains filtering and pagination options for listing categories.
type ListFilter struct {
	Search    string
	Area      string
	Type      string
	IsActive  *bool
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// Validate normalizes pagination and sort defaults.
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
		f.SortBy = "sort_order"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the SQL offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
