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
}
