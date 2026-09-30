// Package yarntxweight provides domain logic for the global TX Weight master.
package yarntxweight

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the persistence contract for TX Weight rules.
type Repository interface {
	// Create persists a new rule.
	Create(ctx context.Context, entity *Entity) error
	// GetByID retrieves a live rule by id (with product type code/name joined).
	GetByID(ctx context.Context, id uuid.UUID) (*Entity, error)
	// List retrieves live rules with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*Entity, int64, error)
	// Update persists mode/value/description changes of a live rule.
	Update(ctx context.Context, entity *Entity) error
	// SoftDelete marks a live rule as deleted.
	SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error
	// ExistsByTypeGrade reports whether a live rule exists for the pair.
	ExistsByTypeGrade(ctx context.Context, productTypeID int32, grade Grade) (bool, error)
	// ProductTypeExists reports whether cost_product_type has the id.
	ProductTypeExists(ctx context.Context, productTypeID int32) (bool, error)
}

// ListFilter contains filtering options for listing rules.
type ListFilter struct {
	Search        string
	ProductTypeID int32 // 0 = all
	Grade         Grade // "" = all
	Page          int
	PageSize      int
	SortBy        string // "product_type", "grade", "created_at", "updated_at"
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
		f.SortBy = "product_type"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the query offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
