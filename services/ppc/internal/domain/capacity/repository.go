// Package capacity provides domain logic for PPC product-machine-capacity master data.
package capacity

import "context"

// Repository defines persistence operations for product-machine capacities.
type Repository interface {
	// Create persists a new capacity and assigns its ID.
	Create(ctx context.Context, entity *Capacity) error

	// GetByID retrieves a capacity by its ID.
	GetByID(ctx context.Context, id int64) (*Capacity, error)

	// List retrieves capacities with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Capacity, int64, error)

	// Update persists changes to an existing capacity.
	Update(ctx context.Context, entity *Capacity) error

	// Delete removes a capacity by its ID.
	Delete(ctx context.Context, id int64) error
}

// ListFilter contains filtering and pagination options for listing capacities.
type ListFilter struct {
	CpmProductSysID *int64
	MachineID       *int64
	Page            int
	PageSize        int
	SortBy          string
	SortOrder       string
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
		f.SortBy = "cpm_product_sys_id"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the SQL offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
