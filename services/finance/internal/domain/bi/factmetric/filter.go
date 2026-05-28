package factmetric

import "time"

// Filter is the input to QueryAggregate. Built by the query planner from a Dashboard
// + ViewerFilters; never constructed directly by gRPC handlers.
type Filter struct {
	Type         string    // bi_fact_metric.type (e.g. "MIS")
	Group1       string    // optional pre-filter
	Group2       string    // populated when drill depth >= 1
	Group3       string    // populated when drill depth >= 2
	Grain        string    // DAILY|MONTHLY|QUARTERLY|YEARLY
	DateFrom     time.Time // inclusive
	DateTo       time.Time // inclusive
	Scenario     string    // default 'ACTUAL'
	DrillPath    []string  // semantic copy of {Group2, Group3} for diagnostics
	IncludeOrder bool      // include group_*_order fields in SELECT (for ORDER BY)
}

// PlannedQuery is the SQL+args bundle produced by the query planner.
type PlannedQuery struct {
	// SQL is a parameterized SELECT statement.
	SQL string
	// Args are positional arguments matching $1, $2, ... in SQL.
	Args []any
	// TargetTable is informational (mv_bi_metric_g1 / mv_bi_metric_g2 / bi_fact_metric).
	TargetTable string
}

// DistinctScope narrows GetDistincts queries.
//
// Empty Type means "return every distinct type across all rows". Non-empty Type
// returns group_1/group_2/group_3 distincts scoped to that type.
type DistinctScope struct {
	Type string
}

// DistinctValues is the result of GetDistincts.
type DistinctValues struct {
	Types         []string
	Group1s       []string
	Group2s       []string
	Group3s       []string
	DimensionKeys []string
}
