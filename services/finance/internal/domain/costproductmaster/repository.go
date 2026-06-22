package costproductmaster

import "context"

// ProductUpsertInput is a single row for BulkUpsertByLegacyID.
type ProductUpsertInput struct {
	LegacySysID   string // maps to cpm_flex_02
	ProductTypeID int32
	ProductName   string
	ShadeCode     string
	ShadeName     string
	GradeCode     string
	Description   string
	ErpItemCode   string
	Flex01        string // legacy_erp_compound_key
	Flex03        string // legacy_type_label
	IsActive      bool
}

// ProductUpsertResult reports the outcome for one upserted product.
type ProductUpsertResult struct {
	LegacySysID  string
	ProductSysID int64
	WasInserted  bool
}

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
	// BulkCreate upserts a batch of products and returns product_code → assigned sysID mapping.
	BulkCreate(ctx context.Context, items []*CostProductMaster, updatedBy string) (map[string]int64, error)
	// ListAll returns all products matching the filter with no pagination cap, for export/sync use.
	ListAll(ctx context.Context, f Filter) ([]*CostProductMaster, error)
	// BulkUpsertByLegacyID upserts products using cpm_flex_02 (legacy Oracle sys_id) as the
	// conflict key. Returns a slice of results mapping each legacySysId to its assigned cpm_product_sys_id.
	BulkUpsertByLegacyID(ctx context.Context, items []ProductUpsertInput, actor string) ([]ProductUpsertResult, error)
}
