package lookupmaster

import "context"

// Repository is the persistence contract for mst_lookup_master + mst_lookup_master_column.
type Repository interface {
	// ListMasters returns all (or only active) registered master lookup codes.
	ListMasters(ctx context.Context, activeOnly bool) ([]*LookupMaster, error)
	// ListColumns returns fillable columns for a given master code, ordered by sort_order.
	ListColumns(ctx context.Context, masterCode string) ([]*Column, error)
	// CreateMaster inserts a new master into the registry.
	CreateMaster(ctx context.Context, m *LookupMaster, createdBy string) error
	// DeleteMaster removes a master from the registry by code.
	DeleteMaster(ctx context.Context, code string) error
	// CreateColumn adds a fillable column to a master and returns the new UUID.
	CreateColumn(ctx context.Context, c *Column, createdBy string) (string, error)
	// DeleteColumn removes a column by its UUID.
	DeleteColumn(ctx context.Context, id string) error
	// UpdateMaster applies partial updates to an existing master.
	UpdateMaster(ctx context.Context, code string, updates UpdateMaster) error
	// ListTableColumns introspects information_schema.columns for a registered table.
	// tableName must exist in mst_lookup_master.lm_table_name (validated by caller).
	ListTableColumns(ctx context.Context, tableName string) ([]*TableColumn, error)
	// ListMasterOptions queries the master's registered table and returns code+label rows.
	ListMasterOptions(ctx context.Context, masterCode string) ([]MasterOption, error)
	// ExportMasters exports all masters+columns to an Excel workbook.
	ExportMasters(ctx context.Context) ([]byte, string, error)
	// ImportMasters imports masters+columns from an Excel workbook.
	ImportMasters(ctx context.Context, content []byte) (success, skipped, failed int, errs []string, err error)
}
