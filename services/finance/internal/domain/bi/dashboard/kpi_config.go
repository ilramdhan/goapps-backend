package dashboard

import (
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
)

// KpiEntry is one KPI card definition stored inside bi_dashboard.kpi_config.
type KpiEntry struct {
	Label            string
	ValueField       string
	MetricName       string // optional; when set, queries bi_fact_metric directly with metric_name filter (multi-metric SALES dashboards)
	Agg              string // sum|avg|min|max|last|cross_ratio
	Compare          string // MoM|QoQ|YoY|YTD_vs_LY|none
	Period           string // selected|current_month|ytd|l12m — scopes the KPI window independently of the viewer's selected period
	Format           chart.NumberFormat
	Decimals         int
	ShowSparkline    bool
	SparklinePeriods int

	// CrossRatio fields — used when Agg == kpiAggCrossRatio.
	// Computes SUM(numerator_group_1) / NULLIF(SUM(denominator_group_1), 0) * CrossRatioScale.
	// Both group_1 values are queried from bi_fact_metric with the same type + grain + period.
	CrossRatioNumeratorGroup1   string  // e.g. "NET PROFIT"
	CrossRatioDenominatorGroup1 string  // e.g. "EBITDA"
	CrossRatioScale             float64 // e.g. 100 for percentage; default 1
}

// KpiConfig is the ordered list of KPI cards a dashboard renders above its main chart.
type KpiConfig []KpiEntry

// allowedAggs is the closed set of aggregation keys.
var allowedAggs = map[string]struct{}{
	"sum": {}, "avg": {}, "min": {}, "max": {}, "last": {}, kpiAggCrossRatio: {},
}

// allowedKpiCompares is the closed set of KPI compare keys (separate from chart compare set:
// KPI supports YTD_vs_LY which the chart overlay calls YTD).
var allowedKpiCompares = map[string]struct{}{
	"MoM": {}, "QoQ": {}, "YoY": {}, "YTD_vs_LY": {}, "none": {},
}

// kpiPeriodSelected is the default per-KPI period scope: inherit the viewer's selected period.
const kpiPeriodSelected = "selected"

// kpiAggCrossRatio is the aggregation key for ratio-between-groups computation.
const kpiAggCrossRatio = "cross_ratio"

// allowedKpiPeriods is the closed set of per-KPI period-scope keys. "selected" (the default)
// means the KPI inherits the viewer's currently selected period; the others scope the KPI to a
// fixed window relative to "now" so labels like "Current Month" / "YTD" / "L12M" stay meaningful
// regardless of what the viewer selected.
var allowedKpiPeriods = map[string]struct{}{
	kpiPeriodSelected: {}, "current_month": {}, "ytd": {}, "l12m": {},
}

// ParseKpiConfig validates every entry in the raw list and returns a typed KpiConfig.
//
// An empty input yields an empty (but non-nil) KpiConfig with no error — a dashboard
// is permitted to render no KPI cards above its main chart.
func ParseKpiConfig(raw []map[string]any) (KpiConfig, error) {
	if len(raw) == 0 {
		return KpiConfig{}, nil
	}
	if len(raw) > 6 {
		return nil, fmt.Errorf("%w: maximum 6 KPI cards, got %d", ErrInvalidKpiConfig, len(raw))
	}
	out := make(KpiConfig, 0, len(raw))
	for i, m := range raw {
		entry, err := parseKpiEntry(m, i)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

// parseKpiEntry validates one KPI entry map.
func parseKpiEntry(m map[string]any, idx int) (KpiEntry, error) {
	label := mapStringVal(m, "label")
	if label == "" {
		return KpiEntry{}, fmt.Errorf("%w: entry %d missing 'label'", ErrInvalidKpiConfig, idx)
	}

	agg, err := parseKpiAgg(m, idx)
	if err != nil {
		return KpiEntry{}, err
	}

	// value_field is required for standard agg modes but not for cross_ratio
	// (which derives its value from two group_1 queries instead of a single field).
	valueField := mapStringVal(m, "value_field")
	if valueField == "" && agg != kpiAggCrossRatio {
		return KpiEntry{}, fmt.Errorf("%w: entry %d missing 'value_field'", ErrInvalidKpiConfig, idx)
	}

	metricName := mapStringVal(m, "metric_name") // optional; when set, KPI queries bi_fact_metric with metric_name filter

	compare, err := parseKpiCompare(m, idx)
	if err != nil {
		return KpiEntry{}, err
	}

	period, err := parseKpiPeriod(m, idx)
	if err != nil {
		return KpiEntry{}, err
	}

	format := mapStringVal(m, "format")
	if format == "" {
		format = string(chart.NumberFormatCurrencyThousands)
	}
	if !chart.IsValidNumberFormat(format) {
		return KpiEntry{}, fmt.Errorf("%w: entry %d format %q is not a recognized number-format", ErrInvalidKpiConfig, idx, format)
	}

	decimals := 0
	if v, ok := mergedAsInt(m, "decimals"); ok {
		decimals = v
	}
	if decimals < 0 || decimals > 6 {
		return KpiEntry{}, fmt.Errorf("%w: entry %d decimals %d out of [0,6]", ErrInvalidKpiConfig, idx, decimals)
	}

	showSparkline := mapBoolVal(m, "show_sparkline")
	sparkPeriods := 0
	if v, ok := mergedAsInt(m, "sparkline_periods"); ok {
		sparkPeriods = v
	}
	if showSparkline && sparkPeriods == 0 {
		sparkPeriods = 12
	}

	crossNumer, crossDenom, crossScale := parseCrossRatioFields(agg, m)

	return KpiEntry{
		Label:                       label,
		ValueField:                  valueField,
		MetricName:                  metricName,
		Agg:                         agg,
		Compare:                     compare,
		Period:                      period,
		Format:                      chart.NumberFormat(format),
		Decimals:                    decimals,
		ShowSparkline:               showSparkline,
		SparklinePeriods:            sparkPeriods,
		CrossRatioNumeratorGroup1:   crossNumer,
		CrossRatioDenominatorGroup1: crossDenom,
		CrossRatioScale:             crossScale,
	}, nil
}

// parseCrossRatioFields reads cross_ratio-specific fields from the raw map.
// Returns empty strings and scale=1 when agg is not kpiAggCrossRatio.
func parseCrossRatioFields(agg string, m map[string]any) (numer, denom string, scale float64) {
	scale = 1.0
	if agg != kpiAggCrossRatio {
		return numer, denom, scale
	}
	numer = mapStringVal(m, "numerator_group_1")
	denom = mapStringVal(m, "denominator_group_1")
	if v, ok := m["scale"].(float64); ok {
		scale = v
	}
	return numer, denom, scale
}

// parseKpiAgg reads and validates the aggregation key, defaulting to "sum".
func parseKpiAgg(m map[string]any, idx int) (string, error) {
	agg := mapStringVal(m, "agg")
	if agg == "" {
		agg = "sum"
	}
	if _, ok := allowedAggs[agg]; !ok {
		return "", fmt.Errorf("%w: entry %d agg %q is not one of sum/avg/min/max/last/cross_ratio", ErrInvalidKpiConfig, idx, agg)
	}
	return agg, nil
}

// parseKpiCompare reads and validates the compare key, defaulting to "none".
func parseKpiCompare(m map[string]any, idx int) (string, error) {
	compare := mapStringVal(m, "compare")
	if compare == "" {
		compare = "none"
	}
	if _, ok := allowedKpiCompares[compare]; !ok {
		return "", fmt.Errorf("%w: entry %d compare %q is not one of MoM/QoQ/YoY/YTD_vs_LY/none", ErrInvalidKpiConfig, idx, compare)
	}
	return compare, nil
}

// parseKpiPeriod reads and validates the optional per-KPI period scope, defaulting to "selected".
func parseKpiPeriod(m map[string]any, idx int) (string, error) {
	period := mapStringVal(m, "period")
	if period == "" {
		period = kpiPeriodSelected
	}
	if _, ok := allowedKpiPeriods[period]; !ok {
		return "", fmt.Errorf("%w: entry %d period %q is not one of selected/current_month/ytd/l12m", ErrInvalidKpiConfig, idx, period)
	}
	return period, nil
}

// mapBoolVal extracts a bool value from a map[string]any, returning false if absent or wrong type.
func mapBoolVal(m map[string]any, key string) bool {
	v, ok := m[key].(bool)
	if !ok {
		return false
	}
	return v
}

// MarshalToList converts a KpiConfig back to a JSON-friendly list of maps.
func (k KpiConfig) MarshalToList() []map[string]any {
	out := make([]map[string]any, len(k))
	for i, e := range k {
		m := map[string]any{
			"label":       e.Label,
			"value_field": e.ValueField,
			"agg":         e.Agg,
			"compare":     e.Compare,
			"format":      string(e.Format),
		}
		if e.MetricName != "" {
			m["metric_name"] = e.MetricName
		}
		if e.Period != "" && e.Period != kpiPeriodSelected {
			m["period"] = e.Period
		}
		if e.Decimals != 0 {
			m["decimals"] = e.Decimals
		}
		if e.ShowSparkline {
			m["show_sparkline"] = true
			if e.SparklinePeriods != 0 {
				m["sparkline_periods"] = e.SparklinePeriods
			}
		}
		if e.Agg == kpiAggCrossRatio {
			m["numerator_group_1"] = e.CrossRatioNumeratorGroup1
			m["denominator_group_1"] = e.CrossRatioDenominatorGroup1
			if e.CrossRatioScale != 0 && e.CrossRatioScale != 1 {
				m["scale"] = e.CrossRatioScale
			}
		}
		out[i] = m
	}
	return out
}
