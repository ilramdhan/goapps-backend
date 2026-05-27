package syncdata

import (
	"context"

	"github.com/google/uuid"
)

// ListFilter holds criteria for listing synced data.
type ListFilter struct {
	Period   string
	ItemCode string
	Search   string
	Page     int
	PageSize int
}

// OracleSourceRepository defines the contract for reading data from Oracle.
type OracleSourceRepository interface {
	// ExecuteProcedure runs the Oracle stored procedure to refresh data.
	ExecuteProcedure(ctx context.Context, schema, procedure string) error

	// ExecuteProcedureWithParam runs the Oracle stored procedure with a parameter.
	ExecuteProcedureWithParam(ctx context.Context, schema, procedure, param string) error

	// FetchItemConsStockPO fetches records for a specific period from Oracle.
	FetchItemConsStockPO(ctx context.Context, period string) ([]*ItemConsStockPO, error)

	// FetchAllItemConsStockPO fetches all records from Oracle.
	FetchAllItemConsStockPO(ctx context.Context) ([]*ItemConsStockPO, error)
}

// PostgresTargetRepository defines the contract for writing synced data to PostgreSQL.
type PostgresTargetRepository interface {
	// UpsertItemConsStockPO batch upserts records into PostgreSQL.
	// Uses ON CONFLICT to prevent duplicates.
	UpsertItemConsStockPO(ctx context.Context, items []*ItemConsStockPO, syncedByJob uuid.UUID) (*UpsertResult, error)

	// ListItemConsStockPO retrieves a paginated list of synced records.
	ListItemConsStockPO(ctx context.Context, filter ListFilter) ([]*ItemConsStockPO, int64, error)

	// GetDistinctPeriods returns all distinct periods that have been synced.
	GetDistinctPeriods(ctx context.Context) ([]string, error)
}
