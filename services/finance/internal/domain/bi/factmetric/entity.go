package factmetric

import (
	"time"

	"github.com/google/uuid"
)

// FactMetric is the immutable read-side view of a bi_fact_metric row.
//
// Used by the read path (chart data queries) and the ingest path (Excel commit, ETL).
// Construction is unconstrained — validation happens at the ingest boundary,
// not on every hydration from the database.
type FactMetric struct {
	MetricID     int64
	Type         string
	Group1       string
	Group2       string
	Group3       string
	Group1Order  int
	Group2Order  int
	Group3Order  int
	PeriodGrain  string
	PeriodDate   time.Time
	PeriodLabel  string
	Value        float64
	DisplayValue float64
	UOM          string
	Scenario     string
	SourceID     uuid.UUID
	DimensionKey string
	UploadedBy   uuid.UUID
	LoadedAt     time.Time
	IsActive     bool
}

// AggRow is the output shape of QueryAggregate — one row per category
// (Group1, Group2, Group3, or period) with the aggregate value and optional
// previous-period value for compare-mode rendering.
type AggRow struct {
	Category    string
	Period      time.Time
	PeriodLabel string
	Value       float64
	PrevValue   float64 // populated when compare mode is set
	Order       int
}

// KpiRow is the output shape of KPI compute — one row per KPI definition.
type KpiRow struct {
	Label              string
	Value              float64
	CompareValue       float64
	DeltaAbs           float64
	DeltaPct           float64
	ComparePeriodLabel string
	Sparkline          []float64
}
