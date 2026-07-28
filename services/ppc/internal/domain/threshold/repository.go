// Package threshold provides domain logic for PPC overrun-threshold-config master data.
package threshold

import "context"

// Repository defines persistence operations for overrun threshold configs.
type Repository interface {
	// Create persists a new config and assigns its ID.
	Create(ctx context.Context, entity *Config) error

	// GetByID retrieves a config by its ID.
	GetByID(ctx context.Context, id int64) (*Config, error)

	// List retrieves configs with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Config, int64, error)

	// Update persists changes to an existing config.
	Update(ctx context.Context, entity *Config) error

	// Delete removes a config by its ID.
	Delete(ctx context.Context, id int64) error
}

// ListFilter contains filtering and pagination options for listing configs.
type ListFilter struct {
	Level     string
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
		f.SortBy = "level"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the SQL offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
