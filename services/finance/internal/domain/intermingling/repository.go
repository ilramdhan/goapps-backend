// Package intermingling provides domain logic for Intermingling cost lookup management.
package intermingling

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the persistence contract for the Intermingling domain.
type Repository interface {
	// Create persists a new Intermingling record.
	Create(ctx context.Context, entity *Entity) error

	// GetByID retrieves a record by its UUID primary key.
	GetByID(ctx context.Context, id uuid.UUID) (*Entity, error)

	// GetByCode retrieves a record by its unique code.
	GetByCode(ctx context.Context, code string) (*Entity, error)

	// List retrieves records with filtering, searching, and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Entity, int64, error)

	// Update persists changes to an existing record.
	Update(ctx context.Context, entity *Entity) error

	// SoftDelete marks a record as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByCode checks if a record with the given code exists.
	ExistsByCode(ctx context.Context, code string) (bool, error)

	// ExistsByID checks if a record with the given UUID exists.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)
}

// ListFilter contains filtering options for listing Intermingling records.
type ListFilter struct {
	Search    string
	IsActive  *bool
	Page      int
	PageSize  int
	SortBy    string // "intm_code", "intm_name", "intm_cost_per_kg", "created_at"
	SortOrder string // "asc", "desc"
}

// Validate normalizes the filter to safe defaults.
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
		f.SortBy = "intm_code"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the query offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
