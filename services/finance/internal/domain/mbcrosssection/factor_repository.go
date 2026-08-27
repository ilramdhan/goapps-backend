package mbcrosssection

import "context"

// FactorRepository defines the persistence contract for MB cross-section conversion factors.
type FactorRepository interface {
	Create(ctx context.Context, e *FactorEntity) error
	Update(ctx context.Context, e *FactorEntity) error
	Delete(ctx context.Context, id, deletedBy string) error
	GetByID(ctx context.Context, id string) (*FactorEntity, error)
	List(ctx context.Context, filter FactorListFilter) ([]*FactorEntity, int64, error)

	// GetByPair retrieves the live factor for an ordered (from_code, to_code) pair.
	GetByPair(ctx context.Context, fromCode, toCode string) (*FactorEntity, error)
}

// FactorListFilter contains filtering, sorting and pagination options for listing factors.
type FactorListFilter struct {
	Search    string
	FromCode  string
	ToCode    string
	IsActive  *bool
	Page      int32
	PageSize  int32
	SortBy    string // "from_code", "to_code", "factor", "created_at"
	SortOrder string // "asc", "desc"
}

// Validate normalizes filter values to safe defaults.
func (f *FactorListFilter) Validate() {
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
		f.SortBy = "from_code"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the offset for pagination.
func (f *FactorListFilter) Offset() int32 {
	return (f.Page - 1) * f.PageSize
}
