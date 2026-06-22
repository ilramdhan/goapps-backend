package costproducttype

import "context"

// Filter for List query.
type Filter struct {
	Search       string
	ActiveFilter string // "all" | "active" | "inactive" | ""
	Page         int
	PageSize     int
	SortBy       string
	SortOrder    string
}

// Repository persists CostProductType aggregates.
type Repository interface {
	Create(ctx context.Context, t *CostProductType) error
	GetByID(ctx context.Context, id int32) (*CostProductType, error)
	GetByCode(ctx context.Context, code string) (*CostProductType, error)
	Update(ctx context.Context, t *CostProductType) error
	List(ctx context.Context, f Filter) (items []*CostProductType, total int64, err error)
	// ListAllActive returns all active cost_product_type rows for bulk import map preloading.
	ListAllActive(ctx context.Context) ([]*CostProductType, error)
}
