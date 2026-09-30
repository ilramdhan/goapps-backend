// Package yarntxweightgroup provides domain logic for TX Weight groups.
package yarntxweightgroup

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the persistence contract for TX Weight groups.
type Repository interface {
	// Create persists the group, its type mappings and its rules in one transaction.
	Create(ctx context.Context, entity *Entity) error
	// GetByID retrieves a live group with its product types and rules.
	GetByID(ctx context.Context, id uuid.UUID) (*Entity, error)
	// List retrieves live groups with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Entity, int64, error)
	// Update replaces the header, the type set and the rules in one transaction.
	Update(ctx context.Context, entity *Entity) error
	// SoftDelete marks the group and its rules deleted and removes its type
	// mappings (the types become free again), in one transaction.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error
	// ExistsByCode reports whether another live group (not excludeID) uses code.
	ExistsByCode(ctx context.Context, code string, excludeID uuid.UUID) (bool, error)
	// MissingProductTypes returns the ids absent from cost_product_type.
	MissingProductTypes(ctx context.Context, productTypeIDs []int32) ([]int32, error)
	// FindProductTypeConflicts returns the ids already mapped to a live group
	// other than excludeID (uuid.Nil on create), with type and group codes.
	FindProductTypeConflicts(ctx context.Context, productTypeIDs []int32, excludeID uuid.UUID) ([]ProductTypeConflict, error)
}

// Sort keys accepted by ListFilter.SortBy.
const (
	SortByCode      = "code"
	SortByName      = "name"
	SortByCreatedAt = "created_at"
	SortByUpdatedAt = "updated_at"
)

// ListFilter contains filtering options for listing groups.
type ListFilter struct {
	Search        string
	ProductTypeID int32 // 0 = all
	Page          int
	PageSize      int
	SortBy        string
	SortOrder     string // "asc", "desc"
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
		f.SortBy = SortByCode
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the query offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
