package costrmtype

import "context"

// Filter for List query.
type Filter struct {
	Search          string
	ReferenceTarget string // "PRODUCT" | "MASTER" | ""
	ActiveFilter    string // "all" | "active" | "inactive" | ""
	Page            int
	PageSize        int
}

// Repository persists CostRmType aggregates.
type Repository interface {
	Create(ctx context.Context, t *CostRmType) error
	GetByID(ctx context.Context, id int32) (*CostRmType, error)
	GetByCode(ctx context.Context, code string) (*CostRmType, error)
	Update(ctx context.Context, t *CostRmType) error
	List(ctx context.Context, f Filter) (items []*CostRmType, total int64, err error)
}
