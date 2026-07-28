// Package lookup provides domain logic for the PPC lookup master.
package lookup

import "context"

// Repository defines persistence operations for lookups.
type Repository interface {
	// Create persists a new lookup and assigns its ID.
	Create(ctx context.Context, entity *Lookup) error

	// GetByID retrieves a lookup by its ID.
	GetByID(ctx context.Context, id int64) (*Lookup, error)

	// List retrieves lookups with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Lookup, int64, error)

	// Update persists changes to an existing lookup.
	Update(ctx context.Context, entity *Lookup) error

	// Delete removes a lookup by its ID.
	Delete(ctx context.Context, id int64) error
}

// ListFilter contains filtering and pagination options for listing lookups.
type ListFilter struct {
	Category string
	Search   string
	IsActive *bool
	Page     int
	PageSize int
}

// Validate normalizes pagination defaults.
func (f *ListFilter) Validate() {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 50
	}
	if f.PageSize > 200 {
		f.PageSize = 200
	}
}

// Offset returns the SQL offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
