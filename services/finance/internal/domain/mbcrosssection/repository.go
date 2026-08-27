package mbcrosssection

import "context"

// Repository defines the persistence contract for MB cross-section master data.
type Repository interface {
	Create(ctx context.Context, e *Entity) error
	Update(ctx context.Context, e *Entity) error
	Delete(ctx context.Context, id, deletedBy string) error
	GetByID(ctx context.Context, id string) (*Entity, error)
	List(ctx context.Context, filter ListFilter) ([]*Entity, int64, error)

	// GetByCode retrieves a cross-section by its unique code.
	GetByCode(ctx context.Context, code string) (*Entity, error)
}

// ListFilter contains filtering, sorting and pagination options for listing cross sections.
type ListFilter struct {
	Search    string
	IsActive  *bool
	Page      int32
	PageSize  int32
	SortBy    string // "code", "display_name", "display_order", "created_at"
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
		f.SortBy = "display_order"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the offset for pagination.
func (f *ListFilter) Offset() int32 {
	return (f.Page - 1) * f.PageSize
}
