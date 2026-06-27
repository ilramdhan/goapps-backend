// Package mbspin provides domain logic for Melange Batch Spin (child of MB Head) management.
package mbspin

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the persistence interface for MB Spin.
type Repository interface {
	// Create persists a new MB Spin.
	Create(ctx context.Context, entity *Entity) error

	// GetByID retrieves an MB Spin by its UUID primary key.
	GetByID(ctx context.Context, id uuid.UUID) (*Entity, error)

	// List retrieves MB Spins with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Entity, int64, error)

	// Update persists changes to an existing MB Spin.
	Update(ctx context.Context, entity *Entity) error

	// SoftDelete marks an MB Spin as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// ExistsByID checks if an MB Spin with the given UUID exists.
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)

	// GetByMBCosting retrieves an MB Spin by its MB costing code.
	GetByMBCosting(ctx context.Context, code string) (*Entity, error)

	// GetByOrionItemCode retrieves the first active MB Spin with the given ORION item code.
	// Returns ErrNotFound if no match exists.
	GetByOrionItemCode(ctx context.Context, code string) (*Entity, error)
}

// ListFilter contains filtering options for listing MB Spins.
type ListFilter struct {
	HeadID    uuid.UUID
	Search    string
	IsActive  *bool
	Page      int
	PageSize  int
	SortBy    string // "mbs_mgt_name", "mbs_denier", "created_at"
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
		f.SortBy = "mbs_mgt_name"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the pagination offset.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
