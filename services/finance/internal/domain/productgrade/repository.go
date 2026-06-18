// Package productgrade provides domain logic for Product Grade quality-loss configuration.
package productgrade

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the interface for Product Grade persistence.
type Repository interface {
	// Create persists a new Product Grade.
	Create(ctx context.Context, entity *Entity) error

	// GetByID retrieves a Product Grade by its UUID primary key.
	GetByID(ctx context.Context, id uuid.UUID) (*Entity, error)

	// GetByCode retrieves a Product Grade by its code.
	GetByCode(ctx context.Context, code string) (*Entity, error)

	// List retrieves Product Grades with filtering, searching, and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Entity, int64, error)

	// Update persists changes to an existing Product Grade.
	Update(ctx context.Context, entity *Entity) error

	// SoftDelete marks a Product Grade as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByCode checks if a Product Grade with the given code exists.
	ExistsByCode(ctx context.Context, code string) (bool, error)

	// ExistsByID checks if a Product Grade with the given UUID exists.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)
}

// ListFilter contains filtering options for listing Product Grades.
type ListFilter struct {
	Search    string
	IsActive  *bool
	Page      int
	PageSize  int
	SortBy    string // "pg_code", "pg_name", "bc_perc", "created_at"
	SortOrder string // "asc", "desc"
}

// Validate validates and normalizes the filter values.
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
		f.SortBy = "pg_code"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
