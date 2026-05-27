package costproductmaster

import "context"

// Filter for List query.
type Filter struct {
	Search        string // matches product_code OR product_name OR erp_item_code
	ProductTypeID int32  // 0 = no filter
	ShadeCode     string
	ActiveFilter  string // "all" | "active" | "inactive" | ""
	Page          int
	PageSize      int
	SortBy        string
	SortOrder     string
}

// Repository persists CostProductMaster aggregates.
type Repository interface {
	// Create inserts a new product. The repo invokes generate_cost_product_code() to obtain
	// the auto-generated product_code and product_sys_id, then assigns them via SetGeneratedCode.
	Create(ctx context.Context, p *CostProductMaster) error
	GetBySysID(ctx context.Context, sysID int64) (*CostProductMaster, error)
	GetByCode(ctx context.Context, code string) (*CostProductMaster, error)
	Update(ctx context.Context, p *CostProductMaster) error
	List(ctx context.Context, f Filter) (items []*CostProductMaster, total int64, err error)
}
