package factmetric

import (
	"context"
	"time"
)

// Repository is the read + ingest contract for bi_fact_metric.
type Repository interface {
	// GetDistincts returns the distinct type/group_1/group_2/group_3 values to
	// populate admin form dropdowns. Cached upstream in Redis (5-min TTL).
	GetDistincts(ctx context.Context, scope DistinctScope) (DistinctValues, error)

	// QueryAggregate executes a PlannedQuery and returns AggRows in plan order.
	QueryAggregate(ctx context.Context, plan PlannedQuery) ([]AggRow, error)

	// LatestPeriod returns the most recent periode_date for the given scope — the
	// "data as of" anchor used so a lagging warehouse dashboard reports relative to the
	// latest loaded period rather than wall-clock now. group1 may be empty to span all
	// groups of the type. Returns a zero time when no matching rows exist.
	LatestPeriod(ctx context.Context, factType, group1, grain string) (time.Time, error)

	// Upsert ingests a batch of fact rows with ON CONFLICT business-key DO UPDATE.
	// Consumed by spec 1C (Excel commit) and spec 1D (ETL). Chunks of 1000 are
	// processed in a single transaction; larger batches use pgx CopyFrom.
	Upsert(ctx context.Context, rows []FactMetric) error
}
