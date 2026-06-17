// Package machine provides domain logic for Machine master data management.
package machine

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the persistence contract for the Machine domain.
type Repository interface {
	// Create persists a new machine.
	Create(ctx context.Context, entity *Entity) error

	// GetByID retrieves a machine by its UUID primary key.
	GetByID(ctx context.Context, id uuid.UUID) (*Entity, error)

	// GetByCode retrieves a machine by its unique code.
	GetByCode(ctx context.Context, code string) (*Entity, error)

	// List retrieves machines with filtering, searching, and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Entity, int64, error)

	// Update persists changes to an existing machine.
	Update(ctx context.Context, entity *Entity) error

	// SoftDelete marks a machine as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByCode reports whether a non-deleted machine with the given code exists.
	ExistsByCode(ctx context.Context, code string) (bool, error)

	// ExistsByID reports whether a non-deleted machine with the given UUID exists.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)
}

// ListFilter holds filtering, searching, sorting, and pagination options.
type ListFilter struct {
	Search    string
	MCType    string
	IsActive  *bool
	Page      int
	PageSize  int
	SortBy    string // "mc_code", "mc_name", "mc_type", "created_at"
	SortOrder string // "asc", "desc"
}

// Validate normalizes out-of-range values to safe defaults.
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
		f.SortBy = "mc_code"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the SQL OFFSET value for the current page.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
