package chartdata

import (
	"fmt"
	"strings"
	"time"

	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// compareNoneLC is the lowercase sentinel for "no compare overlay" passed from the frontend.
const compareNoneLC = "none"

// ViewerFilters carries the runtime filter state from the viewer page.
type ViewerFilters struct {
	PeriodPreset string    // L12M | L24M | THIS_YEAR | THIS_QTR | THIS_MONTH | ALL | CUSTOM
	PeriodFrom   time.Time // valid when CUSTOM
	PeriodTo     time.Time
	Compare      string   // none | MoM | QoQ | YoY | YTD | R12
	DrillPath    []string // depth 0 = aggregate, 1 = into group_2, 2 = into group_3
	Group1Filter []string // selected group_1 values from filter chips; empty = show all
	Group2Filter []string // selected group_2 values from filter chips; empty = show all
	// ForceTrend, when true, overrides the chart_config x_axis_field and forces isTrend=true
	// in Plan(). Used by the monthly-detail BFF to fetch time-series data from dashboards
	// (e.g. EBITDA) whose chart_config is categorical (waterfall), not period-grouped.
	ForceTrend bool
	// ComputedRatio, when non-nil, overrides the normal planning path and routes to
	// planComputedRatio. Used by the /computed BFF endpoint for secondary charts.
	ComputedRatio *dashboarddomain.ComputedRatioConfig
}

// Plan turns a Dashboard + ViewerFilters into a PlannedQuery against the right MV / fact table.
//
// Drill-depth rules:
//   - 0 → mv_bi_metric_g1 (or filtered by dashboard.filter_group_1)
//   - 1 → mv_bi_metric_g2
//   - 2 → bi_fact_metric (group_3 level, raw)
//   - >max_drill_level → ErrDrillTooDeep
//
// Multi-metric path: when chart_config.metric_filter.include_metrics is non-empty,
// bypasses MVs entirely and builds a UNION ALL query against bi_fact_metric directly.
// This path is used for SALES-type dashboards that render multiple metrics on one chart.
//
// Compare modes produce an extra `prev_value` column via LEFT JOIN on shifted periods.
func Plan(d *dashboarddomain.Dashboard, f ViewerFilters, now time.Time) (factmetric.PlannedQuery, error) {
	// Computed-ratio path: ViewerFilters.ComputedRatio is set (injected by the /computed BFF).
	// Bypasses MVs and metric_filter — builds a CASE-WHEN pivot grouped by the configured column.
	// When Denominator is empty, emits SUM(Numerator) per group (single-metric aggregation).
	if f.ComputedRatio != nil {
		cr := f.ComputedRatio
		return planComputedRatio(d, f, now, cr.Numerator, cr.Denominator, cr.Scale, cr.GroupBy)
	}

	// Multi-metric path: chart_config.metric_filter.include_metrics is set.
	// Bypasses MVs — builds a UNION ALL query per metric against bi_fact_metric directly.
	if metrics := metricFilter(d); len(metrics) > 0 {
		return planMultiMetric(d, f, now, metrics)
	}

	drillDepth := len(f.DrillPath)
	if drillDepth > d.MaxDrillLevel().Int() {
		return factmetric.PlannedQuery{}, fmt.Errorf("%w: depth %d > max %d", factmetric.ErrDrillTooDeep, drillDepth, d.MaxDrillLevel().Int())
	}

	period := ResolvePeriod(f.PeriodPreset, f.PeriodFrom, f.PeriodTo, d.PeriodGrain().String(), now)

	// Trend charts (x_axis_field="period") group by period over time; categorical charts
	// (waterfall/bar/donut) group by the drill-level group column.
	// ForceTrend overrides the chart config (used by monthly-detail BFF to pull time-series
	// from categorical dashboards such as EBITDA whose x_axis_field="group_2").
	isTrend := d.ChartConfig().XAxisField == "period" || f.ForceTrend

	switch drillDepth {
	case 0:
		return planLevel1(d, f, period, isTrend)
	case 1:
		return planLevel2(d, f, period, isTrend)
	case 2:
		return planLevel3(d, f, period, isTrend)
	}
	return factmetric.PlannedQuery{}, fmt.Errorf("%w: drill depth %d not supported", factmetric.ErrInvalidPlan, drillDepth)
}

// metricFilter extracts include_metrics from chart_config.metric_filter.
// Returns nil when absent or empty (single-metric / EBITDA pattern → use MV path).
func metricFilter(d *dashboarddomain.Dashboard) []string {
	metrics := d.ChartConfig().MetricFilter.IncludeMetrics
	if len(metrics) == 0 {
		return nil
	}
	return metrics
}

// planMultiMetric builds a UNION ALL query: one SELECT per metric_name.
// Each SELECT produces AggRows with Category=metric_name so the shaper (format.go)
// can split them into separate Series.
//
// Does NOT use MVs — queries bi_fact_metric directly since pivoting across
// metric_names requires per-metric filtering that MVs cannot provide.
//
// Safety note: metric_name values are inlined into the SQL string. This is safe because
// they come exclusively from chart_config.metric_filter.include_metrics (admin-configured,
// not user input). In addition, metric names are validated as UPPERCASE_SNAKE_CASE by the
// registry before being stored, so they contain no SQL-injection characters.
func planMultiMetric(d *dashboarddomain.Dashboard, f ViewerFilters, now time.Time, metrics []string) (factmetric.PlannedQuery, error) {
	period := ResolvePeriod(f.PeriodPreset, f.PeriodFrom, f.PeriodTo, d.PeriodGrain().String(), now)

	// Build common WHERE conditions (shared across all UNION branches).
	// $1 = type. Additional params indexed from $2.
	args := []any{d.FilterType()}
	idx := 2
	baseConds := []string{"type = $1", "agg_method = 'SUM'", "is_active = TRUE"}

	if grain := d.PeriodGrain().String(); grain != "" {
		baseConds = append(baseConds, fmt.Sprintf("periode_grain = $%d", idx))
		args = append(args, grain)
		idx++
	}
	if !period.From.IsZero() && !period.To.IsZero() {
		baseConds = append(baseConds, fmt.Sprintf("periode_date BETWEEN $%d AND $%d", idx, idx+1))
		args = append(args, period.From, period.To)
		idx += 2
	}
	if f.Compare != "" && f.Compare != "NONE" && f.Compare != compareNoneLC {
		baseConds = append(baseConds, fmt.Sprintf("scenario = $%d", idx))
		args = append(args, "ACTUAL")
		idx++
	}

	// Build group filters from dashboard pre-filter, drill path, and filter chips.
	baseConds, args, _ = appendGroupConds(baseConds, args, idx, d, f)

	allConds := baseConds
	where := strings.Join(allConds, " AND ")

	// One SELECT per metric — UNION ALL preserves all rows.
	// Category column = metric_name so format.go can split into separate Series.
	unions := make([]string, 0, len(metrics))
	for _, m := range metrics {
		unions = append(unions, fmt.Sprintf(`
SELECT '%s'::text AS category,
       periode_date,
       TO_CHAR(periode_date,'YYYYMM')::text AS periode_label,
       COALESCE(SUM(display_value), 0) AS value,
       0::numeric AS prev_value,
       0::int AS order_seq
FROM bi_fact_metric
WHERE %s AND metric_name = '%s'
GROUP BY periode_date`, m, where, m))
	}

	sql := strings.Join(unions, "\nUNION ALL\n") + "\nORDER BY periode_label, category"
	return factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: "bi_fact_metric"}, nil
}

// appendGroupConds appends group_1 / group_2 conditions for a multi-metric UNION query.
//
// Priority order:
//  1. Dashboard-level filter_group_1 (pre-pinned by config) → exact match, skips filter chips.
//  2. Drill path from the viewer page → sequential group_1 / group_2 exact matches.
//  3. Filter-chip selections → IN (...) conditions for user-toggled group values.
func appendGroupConds(
	conds []string, args []any, idx int,
	d *dashboarddomain.Dashboard, f ViewerFilters,
) ([]string, []any, int) {
	if d.FilterGroup1() != "" {
		conds = append(conds, fmt.Sprintf("group_1 = $%d", idx))
		args = append(args, d.FilterGroup1())
		idx++
	} else if len(f.DrillPath) > 0 {
		conds = append(conds, fmt.Sprintf("group_1 = $%d", idx))
		args = append(args, f.DrillPath[0])
		idx++
		if len(f.DrillPath) > 1 {
			conds = append(conds, fmt.Sprintf("group_2 = $%d", idx))
			args = append(args, f.DrillPath[1])
			idx++
		}
	}
	// Filter-chip selections (only when dashboard has not pinned group_1).
	if len(f.Group1Filter) > 0 && d.FilterGroup1() == "" {
		conds, args, idx = appendINClause(conds, args, idx, "group_1", f.Group1Filter)
	}
	if len(f.Group2Filter) > 0 {
		conds, args, idx = appendINClause(conds, args, idx, "group_2", f.Group2Filter)
	}
	return conds, args, idx
}

// appendINClause adds a "col IN ($n,$n+1,...)" condition and advances idx.
func appendINClause(conds []string, args []any, idx int, col string, vals []string) ([]string, []any, int) {
	placeholders := make([]string, len(vals))
	for i, v := range vals {
		placeholders[i] = fmt.Sprintf("$%d", idx)
		args = append(args, v)
		idx++
	}
	conds = append(conds, col+" IN ("+strings.Join(placeholders, ",")+")")
	return conds, args, idx
}

// applyTrend overrides the category/order columns to group by period when the chart
// is a trend chart. Group filters (Group1/Group2) remain as WHERE conditions.
func applyTrend(a buildArgs, isTrend bool) buildArgs {
	if isTrend {
		a.CategoryCol = "periode_date"
		a.OrderCol = ""
	}
	return a
}

// planLevel1 — aggregate at group_1 within (or below) the dashboard's filter_type.
//
// When filter_group_1 is set, drilling at level 0 means "show group_2 breakdown for that fixed group_1",
// effectively treating the dashboard as already drilled one level. This matches the EBITDA pattern
// (filter_type=MIS, filter_group_1=EBITDA → waterfall shows G2 components).
func planLevel1(d *dashboarddomain.Dashboard, f ViewerFilters, period PeriodRange, isTrend bool) (factmetric.PlannedQuery, error) {
	if d.FilterGroup1() != "" {
		// Dashboard pre-narrows to a specific group_1 → render its group_2 breakdown.
		return buildPlan(applyTrend(buildArgs{
			Source:      "mv_bi_metric_g2",
			CategoryCol: "group_2",
			OrderCol:    "group_2_order",
			Type:        d.FilterType(),
			Group1:      d.FilterGroup1(),
			Period:      period,
			Grain:       d.PeriodGrain().String(),
			Compare:     f.Compare,
		}, isTrend))
	}
	return buildPlan(applyTrend(buildArgs{
		Source:      "mv_bi_metric_g1",
		CategoryCol: "group_1",
		OrderCol:    "group_1_order",
		Type:        d.FilterType(),
		Period:      period,
		Grain:       d.PeriodGrain().String(),
		Compare:     f.Compare,
	}, isTrend))
}

func planLevel2(d *dashboarddomain.Dashboard, f ViewerFilters, period PeriodRange, isTrend bool) (factmetric.PlannedQuery, error) {
	// DrillPath[0] picks a group_2 (when dashboard had filter_group_1) or a group_1 (when it didn't).
	if d.FilterGroup1() != "" {
		// Level 2 with pre-narrow → into group_3 of (filter_group_1, drill[0]).
		return buildPlan(applyTrend(buildArgs{
			Source:      "bi_fact_metric",
			CategoryCol: "group_3",
			OrderCol:    "group_3_order",
			Type:        d.FilterType(),
			Group1:      d.FilterGroup1(),
			Group2:      f.DrillPath[0],
			Period:      period,
			Grain:       d.PeriodGrain().String(),
			Compare:     f.Compare,
			RequireG3:   true,
		}, isTrend))
	}
	return buildPlan(applyTrend(buildArgs{
		Source:      "mv_bi_metric_g2",
		CategoryCol: "group_2",
		OrderCol:    "group_2_order",
		Type:        d.FilterType(),
		Group1:      f.DrillPath[0],
		Period:      period,
		Grain:       d.PeriodGrain().String(),
		Compare:     f.Compare,
	}, isTrend))
}

func planLevel3(d *dashboarddomain.Dashboard, f ViewerFilters, period PeriodRange, isTrend bool) (factmetric.PlannedQuery, error) {
	// Without filter_group_1: drill[0]=group_1, drill[1]=group_2 → render group_3.
	// With filter_group_1:    drill[0]=group_2, drill[1]=group_3 → already at max depth (level3 isn't drillable).
	if d.FilterGroup1() != "" {
		return factmetric.PlannedQuery{}, fmt.Errorf("%w: cannot drill past group_3 when dashboard pre-filters group_1", factmetric.ErrInvalidPlan)
	}
	return buildPlan(applyTrend(buildArgs{
		Source:      "bi_fact_metric",
		CategoryCol: "group_3",
		OrderCol:    "group_3_order",
		Type:        d.FilterType(),
		Group1:      f.DrillPath[0],
		Group2:      f.DrillPath[1],
		Period:      period,
		Grain:       d.PeriodGrain().String(),
		Compare:     f.Compare,
		RequireG3:   true,
	}, isTrend))
}

// buildArgs are the inputs to buildPlan.
type buildArgs struct {
	Source      string
	CategoryCol string
	OrderCol    string
	Type        string
	Group1      string
	Group2      string
	Period      PeriodRange
	Grain       string
	Scenario    string
	Compare     string
	RequireG3   bool // when true, add `AND group_3 IS NOT NULL`
}

// buildPlan emits the canonical 6-column SELECT used by BiFactMetricRepository.QueryAggregate.
//
// Columns (in order): category, period_date, period_label, value, prev_value, order_seq.
//
// For categorical charts (waterfall/bar/donut) one row per distinct category is returned
// with prev_value=0 (compare overlays are not meaningful for those types).
//
// For trend charts (CategoryCol="periode_date") with an active compare mode, a self-join
// to a period-shifted aggregate populates prev_value so the frontend renders a comparison
// line. The shift is month-based (MoM=1, QoQ=3, YoY/R12=12); YTD/none get no overlay.
func buildPlan(a buildArgs) (factmetric.PlannedQuery, error) {
	if a.Scenario == "" {
		a.Scenario = "ACTUAL"
	}

	// Core conditions (type/group/grain/scenario) are shared by current + previous
	// aggregates; date conditions apply only to the current window.
	coreConds := []string{"type = $1"}
	args := []any{a.Type}
	idx := 2
	if a.Group1 != "" {
		coreConds = append(coreConds, fmt.Sprintf("group_1 = $%d", idx))
		args = append(args, a.Group1)
		idx++
	}
	if a.Group2 != "" {
		coreConds = append(coreConds, fmt.Sprintf("group_2 = $%d", idx))
		args = append(args, a.Group2)
		idx++
	}
	if a.RequireG3 {
		coreConds = append(coreConds, "group_3 IS NOT NULL")
	}
	coreConds = append(coreConds, fmt.Sprintf("periode_grain = $%d", idx))
	args = append(args, a.Grain)
	idx++
	coreConds = append(coreConds, fmt.Sprintf("scenario = $%d", idx))
	args = append(args, a.Scenario)
	idx++

	dateConds := append([]string{}, coreConds...)
	hasDateWindow := !a.Period.From.IsZero() && !a.Period.To.IsZero()
	if hasDateWindow {
		dateConds = append(dateConds, fmt.Sprintf("periode_date BETWEEN $%d AND $%d", idx, idx+1))
		args = append(args, a.Period.From, a.Period.To)
	}

	groupByPeriod := a.CategoryCol == "periode_date"

	// Trend chart + real compare mode → emit a self-join for prev_value.
	if groupByPeriod {
		if shift := compareShiftMonths(a.Compare); shift > 0 {
			return buildTrendComparePlan(a, coreConds, dateConds, args, shift), nil
		}
		// NB: the materialized views (mv_bi_metric_g1/g2) do NOT carry periode_label,
		// so the label is derived from periode_date via TO_CHAR (YYYYMM) here.
		sql := fmt.Sprintf(`
SELECT TO_CHAR(periode_date, 'YYYYMM')::text AS category, periode_date,
       TO_CHAR(periode_date, 'YYYYMM')::text AS periode_label,
       SUM(value) AS value, 0::numeric AS prev_value, 0::int AS order_seq
FROM %s
WHERE %s
GROUP BY periode_date
ORDER BY periode_date`, a.Source, strings.Join(dateConds, " AND "))
		return factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: a.Source}, nil
	}

	// Categorical chart: one row per distinct category, summed over the window.
	categoryExpr := "COALESCE(" + a.CategoryCol + ", '')"
	orderExpr := "0"
	if a.OrderCol != "" {
		orderExpr = "COALESCE(MAX(" + a.OrderCol + "), 0)"
	}
	sql := fmt.Sprintf(`
SELECT %s AS category, NULL::date AS periode_date, ''::text AS periode_label,
       SUM(value) AS value, 0::numeric AS prev_value, %s AS order_seq
FROM %s
WHERE %s
GROUP BY %s
ORDER BY order_seq NULLS LAST, category`,
		categoryExpr, orderExpr, a.Source, strings.Join(dateConds, " AND "), a.CategoryCol)

	return factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: a.Source}, nil
}

// buildTrendComparePlan assembles a CTE query: `cur` aggregates the current window per
// period; `prev` aggregates the whole series (no date filter) per period; the outer
// select left-joins prev on (cur.periode_date - <shift> months) to populate prev_value.
//
// shiftMonths is a fixed integer derived from the compare mode (not user input), so the
// INTERVAL literal is safe to inline.
func buildTrendComparePlan(a buildArgs, coreConds, dateConds []string, args []any, shiftMonths int) factmetric.PlannedQuery {
	// NB: the materialized views don't carry periode_label; it is derived via TO_CHAR.
	sql := fmt.Sprintf(`
WITH cur AS (
    SELECT periode_date, SUM(value) AS value
    FROM %s
    WHERE %s
    GROUP BY periode_date
),
prev AS (
    SELECT periode_date, SUM(value) AS value
    FROM %s
    WHERE %s
    GROUP BY periode_date
)
SELECT TO_CHAR(cur.periode_date, 'YYYYMM')::text AS category,
       cur.periode_date,
       TO_CHAR(cur.periode_date, 'YYYYMM')::text AS periode_label,
       cur.value,
       COALESCE(prev.value, 0) AS prev_value,
       0::int AS order_seq
FROM cur
LEFT JOIN prev ON prev.periode_date = (cur.periode_date - INTERVAL '%d months')
ORDER BY cur.periode_date`,
		a.Source, strings.Join(dateConds, " AND "),
		a.Source, strings.Join(coreConds, " AND "),
		shiftMonths)

	return factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: a.Source}
}

// planComputedRatio builds either a ratio query or a single-metric aggregation query,
// grouped by groupBy (e.g. "group_2" or "group_1"), for the period resolved from ViewerFilters.
//
// When denominator is non-empty:
//
//	ROUND(SUM(numerator_metric) / NULLIF(SUM(denominator_metric), 0) * scale, 2)
//
// When denominator is empty:
//
//	COALESCE(SUM(numerator_metric), 0) * scale  (single-metric group aggregation)
//
// Result: one AggRow per non-null groupBy value where Category=groupBy, Value=computed result.
//
// Safety note: numerator, denominator are metric_name values sourced from admin-configured
// chart_config (not user input) and are validated as UPPERCASE_SNAKE_CASE by the registry,
// so inlining them into the SQL does not create an injection risk. scale is a float64 constant.
func planComputedRatio( //nolint:unparam
	d *dashboarddomain.Dashboard, f ViewerFilters, now time.Time,
	numerator, denominator string, scale float64, groupBy string,
) (factmetric.PlannedQuery, error) {
	if groupBy == "" {
		groupBy = "group_2"
	}
	// Derive the order column from the groupBy column (e.g. group_2 → group_2_order).
	orderCol := groupBy + "_order"

	period := ResolvePeriod(f.PeriodPreset, f.PeriodFrom, f.PeriodTo, d.PeriodGrain().String(), now)
	args := []any{d.FilterType()}
	idx := 2
	conds := []string{"type = $1", "agg_method = 'SUM'", "is_active = TRUE"}

	if grain := d.PeriodGrain().String(); grain != "" {
		conds = append(conds, fmt.Sprintf("periode_grain = $%d", idx))
		args = append(args, grain)
		idx++
	}
	if !period.From.IsZero() && !period.To.IsZero() {
		conds = append(conds, fmt.Sprintf("periode_date BETWEEN $%d AND $%d", idx, idx+1))
		args = append(args, period.From, period.To)
		idx += 2
	}

	// Dashboard-level group_1 pre-filter or drill path (mirrors appendGroupConds logic).
	if d.FilterGroup1() != "" {
		conds = append(conds, fmt.Sprintf("group_1 = $%d", idx))
		args = append(args, d.FilterGroup1())
		idx++
	} else if len(f.DrillPath) > 0 {
		conds = append(conds, fmt.Sprintf("group_1 = $%d", idx))
		args = append(args, f.DrillPath[0])
		idx++
	} else if len(f.Group1Filter) > 0 {
		// Filter-chip group_1 selections (e.g. ["Export", "Local"]) narrow the ratio
		// to only those delivery types, mirroring appendGroupConds behaviour.
		conds, args, idx = appendINClause(conds, args, idx, "group_1", f.Group1Filter)
	}
	// Filter-chip group_2 selections narrow the computed groupBy dimension when
	// groupBy=="group_2" (e.g. show margin % only for selected product categories).
	if len(f.Group2Filter) > 0 && groupBy == "group_2" {
		conds, args, idx = appendINClause(conds, args, idx, "group_2", f.Group2Filter)
	}
	_ = idx // consumed by appendINClause above; satisfy linter

	where := strings.Join(conds, " AND ")
	// scale is a config constant (not user input), safe to inline.
	scaleStr := fmt.Sprintf("%.6g", scale)

	var sql string
	if denominator == "" {
		// Single-metric aggregation: SUM(numerator) per groupBy column.
		sql = fmt.Sprintf(`
SELECT %s::text AS category,
       NULL::date    AS periode_date,
       ''::text      AS periode_label,
       COALESCE(SUM(display_value), 0) * %s AS value,
       0::numeric AS prev_value,
       MAX(%s) AS order_seq
FROM bi_fact_metric
WHERE %s
  AND metric_name = '%s'
  AND %s IS NOT NULL
GROUP BY %s
ORDER BY value DESC NULLS LAST`,
			groupBy, scaleStr, orderCol,
			where,
			numerator,
			groupBy,
			groupBy)
	} else {
		// Ratio query: SUM(numerator) / NULLIF(SUM(denominator), 0) * scale.
		sql = fmt.Sprintf(`
SELECT %s::text AS category,
       NULL::date    AS periode_date,
       ''::text      AS periode_label,
       ROUND(
         SUM(CASE WHEN metric_name = '%s' THEN display_value END) /
         NULLIF(SUM(CASE WHEN metric_name = '%s' THEN display_value END), 0) * %s,
         2
       ) AS value,
       0::numeric AS prev_value,
       MAX(%s) AS order_seq
FROM bi_fact_metric
WHERE %s
  AND metric_name IN ('%s', '%s')
  AND %s IS NOT NULL
GROUP BY %s
ORDER BY value DESC NULLS LAST`,
			groupBy,
			numerator, denominator, scaleStr,
			orderCol,
			where,
			numerator, denominator,
			groupBy,
			groupBy)
	}

	return factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: "bi_fact_metric"}, nil
}

// compareShiftMonths maps a compare mode to a month offset for the overlay self-join.
// Returns 0 when no period-shifted overlay applies (none / YTD / unknown).
func compareShiftMonths(compare string) int {
	switch compare {
	case "MoM":
		return 1
	case "QoQ":
		return 3
	case "YoY", "R12":
		return 12
	default:
		return 0
	}
}
