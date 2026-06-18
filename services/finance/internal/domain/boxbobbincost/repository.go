// Package boxbobbincost provides domain logic for Box Bobbin Cost management.
package boxbobbincost

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the contract for Box Bobbin Cost persistence.
type Repository interface {
	// Create persists a new Entity.
	Create(ctx context.Context, entity *Entity) error

	// GetByID retrieves an Entity by UUID primary key (includes latest rates).
	GetByID(ctx context.Context, id uuid.UUID) (*Entity, error)

	// GetByCode retrieves an Entity by its unique code.
	GetByCode(ctx context.Context, code string) (*Entity, error)

	// List retrieves entities with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Entity, int64, error)

	// Update persists changes to an existing Entity.
	Update(ctx context.Context, entity *Entity) error

	// Delete soft-deletes an Entity by UUID.
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ListRates retrieves all active rate entries for a parent entity.
	ListRates(ctx context.Context, parentID uuid.UUID) ([]*RateEntry, error)

	// CreateRate persists a new rate entry.
	CreateRate(ctx context.Context, rate *RateEntry) error

	// DeleteRate soft-deletes a single rate entry by UUID.
	DeleteRate(ctx context.Context, rateID uuid.UUID, deletedBy string) error

	// ExistsByCode checks if a non-deleted entity with the given code exists.
	ExistsByCode(ctx context.Context, code string) (bool, error)
}

// ListFilter holds filtering and pagination options.
type ListFilter struct {
	Search    string
	BBCType   string
	IsActive  *bool
	Page      int
	PageSize  int
	SortBy    string // "bbc_code", "bbc_name", "bbc_type", "created_at"
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
		f.SortBy = "bbc_code"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the SQL OFFSET value.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
