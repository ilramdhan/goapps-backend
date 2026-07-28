// Package productmachineparameter provides domain logic for the PPC
// product-machine-parameter master (per product+machine typed parameter values).
package productmachineparameter

import "context"

// Repository defines persistence operations for product-machine parameters.
type Repository interface {
	// Create persists a new parameter value and assigns its ID.
	Create(ctx context.Context, entity *Parameter) error

	// GetByID retrieves a parameter value by its ID.
	GetByID(ctx context.Context, id int64) (*Parameter, error)

	// List retrieves parameter values with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Parameter, int64, error)

	// Update persists changes to an existing parameter value.
	Update(ctx context.Context, entity *Parameter) error

	// Delete removes a parameter value by its ID.
	Delete(ctx context.Context, id int64) error
}

// ListFilter contains filtering and pagination options for listing parameters.
type ListFilter struct {
	CpmProductSysID *int64
	MachineID       *int64
	ParamID         string
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
