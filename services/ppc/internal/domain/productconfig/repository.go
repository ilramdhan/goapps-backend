// Package productconfig provides domain logic for PPC product-config master data.
package productconfig

import "context"

// Repository defines persistence operations for product PPC configs.
type Repository interface {
	// Create persists a new product config and assigns its ID.
	Create(ctx context.Context, entity *ProductConfig) error

	// GetByID retrieves a product config by its ID.
	GetByID(ctx context.Context, id int64) (*ProductConfig, error)

	// List retrieves product configs with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*ProductConfig, int64, error)

	// Update persists changes to an existing product config.
	Update(ctx context.Context, entity *ProductConfig) error

	// Delete removes a product config by its ID.
	Delete(ctx context.Context, id int64) error
}

// ListFilter contains filtering and pagination options for listing configs.
type ListFilter struct {
	Search             string
	CommodityWatchOnly *bool
	Page               int
	PageSize           int
	SortBy             string
	SortOrder          string
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
