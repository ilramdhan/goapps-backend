// Package mbhead provides domain logic for Melange Batch Head (MEL product type) management.
package mbhead

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the persistence interface for MB Head.
type Repository interface {
	// Create persists a new MB Head.
	Create(ctx context.Context, entity *Entity) error

	// GetByID retrieves an MB Head by its UUID primary key.
	GetByID(ctx context.Context, id uuid.UUID) (*Entity, error)

	// GetByMBCosting retrieves an MB Head by its unique mb_costing value.
	GetByMBCosting(ctx context.Context, mbCosting string) (*Entity, error)

	// List retrieves MB Heads with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Entity, int64, error)

	// Update persists changes to an existing MB Head.
	Update(ctx context.Context, entity *Entity) error

	// SoftDelete marks an MB Head as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByMBCosting checks if an MB Head with the given mb_costing exists.
	ExistsByMBCosting(ctx context.Context, mbCosting string) (bool, error)

	// ExistsByID checks if an MB Head with the given UUID exists.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)
}

// ListFilter contains filtering options for listing MB Heads.
type ListFilter struct {
	Search    string
	IsActive  *bool
	Page      int
	PageSize  int
	SortBy    string // "mbh_mb_costing", "mbh_mgt_name", "mbh_denier", "created_at"
	SortOrder string // "asc", "desc"
}

// Validate normalizes filter values to safe defaults.
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
		f.SortBy = "mbh_mb_costing"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
