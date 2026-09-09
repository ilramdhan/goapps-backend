package costproductmaster

import (
	"context"
	"time"
)

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
	Search         string  // matches product_code OR product_name OR erp_item_code OR oracle sys id (flex_02)
	ProductTypeID  int32   // 0 = no filter (legacy single-type filter)
	ProductTypeIDs []int32 // additional type filter, unioned with ProductTypeID; empty = no filter
	ShadeCode      string
	ActiveFilter   string // "all" | "active" | "inactive" | ""
	Page           int
	PageSize       int
	SortBy         string
	SortOrder      string
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
	// ListAllLegacyIDs returns a map of flex02OrCode → cpm_product_sys_id for all
	// active products. flex02OrCode = cpm_flex_02 if set, else cpm_product_code.
	// Used by the params-only import to resolve legacy_oracle_sys_id without
	// requiring a product_master sheet in the same file.
	ListAllLegacyIDs(ctx context.Context) (map[string]int64, error)
	// RollbackImport deletes all data written by a failed bulk import for the given
	// newly-inserted product IDs. Removes route_rm, route_seq, route_head rows (in FK
	// order) then deletes the products themselves; cost_product_parameter and
	// cost_product_applicable_param are cleaned up via ON DELETE CASCADE.
	RollbackImport(ctx context.Context, insertedProductSysIDs []int64) error
	// UnlockWithLog clears cpm_is_locked and inserts the mst_mb_lock_log audit row in a single
	// transaction, so a failure writing the audit trail rolls back the lock change too.
	UnlockWithLog(ctx context.Context, in LockLogInput) error
	// DuplicateProduct clones one product master row and, when CopyParams is true, its
	// cost_product_applicable_param (CAPP) and cost_product_parameter (CPP) rows onto the
	// new product (F2). Does not touch any route -- the duplicate starts with no route,
	// exactly like a freshly-created product. Mirrors the product-cloning step Fork
	// (CostRouteService.DuplicateRoute) already performs.
	DuplicateProduct(ctx context.Context, in DuplicateInput) (DuplicateOutput, error)
}

// DuplicateInput is F2's request: clone one product master, optionally with its CAPP/CPP rows.
type DuplicateInput struct {
	ProductSysID  int64
	NewCodePrefix string
	CopyParams    bool
	ActorUserID   string
}

// DuplicateOutput reports the newly created product.
type DuplicateOutput struct {
	NewProductSysID int64
	NewProductCode  string
}

// LockLogInput is a single mst_mb_lock_log row for an escape-hatch unlock.
type LockLogInput struct {
	ProductSysID int64
	UnlockedBy   string
	Reason       string
	AutoRelockAt time.Time
}
