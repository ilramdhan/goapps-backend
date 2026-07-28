// Package downtimereason provides domain logic for PPC downtime-reason master data.
package downtimereason

import "context"

// Repository defines persistence operations for downtime reasons.
type Repository interface {
	// Create persists a new downtime reason and assigns its ID.
	Create(ctx context.Context, entity *Reason) error

	// GetByID retrieves a downtime reason by its ID.
	GetByID(ctx context.Context, id int64) (*Reason, error)

	// List retrieves downtime reasons with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Reason, int64, error)

	// Update persists changes to an existing downtime reason.
	Update(ctx context.Context, entity *Reason) error

	// Delete removes a downtime reason by its ID.
	Delete(ctx context.Context, id int64) error
}

// ListFilter contains filtering and pagination options for listing downtime reasons.
type ListFilter struct {
	Search    string
	Area      string
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
