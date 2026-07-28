// Package shift provides domain logic for the PPC shift master.
package shift

import "context"

// Repository defines persistence operations for shifts.
type Repository interface {
	// Create persists a new shift and assigns its ID.
	Create(ctx context.Context, entity *Shift) error

	// GetByID retrieves a shift by its ID.
	GetByID(ctx context.Context, id int64) (*Shift, error)

	// List retrieves shifts with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Shift, int64, error)

	// Update persists changes to an existing shift.
	Update(ctx context.Context, entity *Shift) error

	// Delete removes a shift by its ID.
	Delete(ctx context.Context, id int64) error
}

// ListFilter contains filtering and pagination options for listing shifts.
type ListFilter struct {
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
		f.PageSize = 10
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
}

// Offset returns the SQL offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
