package chartdata

import (
	"context"
	"fmt"
	"strings"
	"time"

	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// ComputeKPIs runs one parameterized SUM/AVG/etc query per KPI entry and assembles
// the typed results. The KPI source table is always mv_bi_metric_g1 (or g2 when the
// dashboard pre-narrows via filter_group_1) so KPI values match the chart's top-line.
//
// Compare modes (MoM/QoQ/YoY/YTD_vs_LY) trigger a second query against a shifted
// period; "none" leaves CompareValue=0.
//
// Sparkline (when enabled) is a separate small query returning the last N period values
// for the same scope.
//
// Errors from individual KPIs propagate (we do not silently swallow partial failures).
func ComputeKPIs(
	ctx context.Context,
	repo factmetric.Repository,
	d *dashboarddomain.Dashboard,
	periodRange PeriodRange,
	now time.Time,
	group1Filters, group2Filters []string,
) ([]factmetric.KpiRow, error) {
	kpis := d.KpiConfig()
	if len(kpis) == 0 {
		return nil, nil
	}
	out := make([]factmetric.KpiRow, 0, len(kpis))
	for _, k := range kpis {
		row, err := computeOneKPI(ctx, repo, d, k, periodRange, now, group1Filters, group2Filters)
		if err != nil {
			return nil, fmt.Errorf("kpi %q: %w", k.Label, err)
		}
		out = append(out, row)
	}
	return out, nil
}

// computeOneKPI runs the current-period query and (if needed) the compare-period query.
//
//nolint:nestif // cohesive KPI compute pipeline; nesting reflects natural conditional branching
func computeOneKPI(
	ctx context.Context,
	repo factmetric.Repository,
	d *dashboarddomain.Dashboard,
	k dashboarddomain.KpiEntry,
	period PeriodRange,
	now time.Time,
	group1Filters, group2Filters []string,
) (factmetric.KpiRow, error) {
	if k.Agg == "cross_ratio" {
		return computeCrossRatioKPI(ctx, repo, d, k, period, now, group1Filters, group2Filters)
	}

	// Each KPI may scope its own window (e.g. "Current Month" vs "YTD") independently
	// of the period the viewer selected for the main chart; "selected" inherits it.
	effPeriod := resolveKPIPeriod(k.Period, period, now, d.PeriodGrain().String(), d.PeriodCalendar().String())
	currentVal, err := runKPIScalar(ctx, repo, d, k, effPeriod, "ACTUAL", now, group1Filters, group2Filters)
	if err != nil {
		return factmetric.KpiRow{}, err
	}
	row := factmetric.KpiRow{
		Label: k.Label,
		Value: currentVal,
	}
	if k.Compare != "none" && k.Compare != "" {
		comparePeriod, compareLabel := compareRange(effPeriod, k.Compare, d.PeriodGrain().String(), d.PeriodCalendar().String())
		if !comparePeriod.From.IsZero() {
			compareVal, err := runKPIScalar(ctx, repo, d, k, comparePeriod, "ACTUAL", now, group1Filters, group2Filters)
			if err != nil {
				return factmetric.KpiRow{}, err
			}
			row.CompareValue = compareVal
			row.ComparePeriodLabel = compareLabel
			row.DeltaAbs = currentVal - compareVal
			if compareVal != 0 {
				row.DeltaPct = (currentVal - compareVal) / abs(compareVal) * 100
			}
		}
	}
	if k.ShowSparkline {
		periods := k.SparklinePeriods
		if periods <= 0 {
			periods = 12
		}
		spark, err := runSparkline(ctx, repo, d, k, periods, now, group1Filters, group2Filters)
		if err != nil {
			return factmetric.KpiRow{}, err
		}
		row.Sparkline = spark
	}
	return row, nil
}

// computeCrossRatioKPI computes SUM(numerator_group_1) / NULLIF(SUM(denominator_group_1), 0) * scale
// (HD-08: per-side type override and dept filter).
// Both groups are queried from bi_fact_metric with the dashboard's type, grain, and the KPI's period scope.
// Returns 0 with no error when the denominator is 0 or data is absent.
func computeCrossRatioKPI(
	ctx context.Context,
	repo factmetric.Repository,
	d *dashboarddomain.Dashboard,
	k dashboarddomain.KpiEntry,
	period PeriodRange,
	now time.Time,
	group1Filters, _ []string, // group2Filters not applicable; group1Filters (dept) apply only to sides without a fixed group_1
) (factmetric.KpiRow, error) {
	// Follows the viewer's selected period (H002b); falls back to L12M when none (ALL).
	selected := period
	if selected.From.IsZero() || selected.To.IsZero() {
		selected = ResolvePeriodCal("L12M", time.Time{}, time.Time{}, d.PeriodGrain().String(), d.PeriodCalendar().String(), now)
	}
	effPeriod := resolveKPIPeriod(k.Period, selected, now, d.PeriodGrain().String(), d.PeriodCalendar().String())

	grain := d.PeriodGrain().String()
	ratioFor := func(per PeriodRange) (float64, error) {
		return crossRatioValue(ctx, repo, d.FilterType(), grain, k, group1Filters, per)
	}

	ratio, err := ratioFor(effPeriod)
	if err != nil {
		return factmetric.KpiRow{Label: k.Label}, err
	}
	row := factmetric.KpiRow{Label: k.Label, Value: ratio}
	if k.Compare == compareNoneLC || k.Compare == "" {
		return row, nil
	}
	cmpPeriod, cmpLabel := compareRange(effPeriod, k.Compare, grain, d.PeriodCalendar().String())
	if cmpPeriod.From.IsZero() {
		return row, nil
	}
	cmpVal, err := ratioFor(cmpPeriod)
	if err != nil {
		return factmetric.KpiRow{Label: k.Label}, err
	}
	row.CompareValue = cmpVal
	row.ComparePeriodLabel = cmpLabel
	row.DeltaAbs = ratio - cmpVal
	if cmpVal != 0 {
		row.DeltaPct = (ratio - cmpVal) / abs(cmpVal) * 100
	}
	return row, nil
}

// crossRatioQuery sums VALUE of one cross_ratio side. typ falls back to the dashboard type;
// a fixed group1 wins over the viewer's dept filter (group1Filters).
func crossRatioQuery(typ, defaultType, grain, group1 string, group1Filters []string, per PeriodRange) factmetric.PlannedQuery {
	if typ == "" {
		typ = defaultType
	}
	args := []any{typ, grain, per.From, per.To}
	conds := ""
	switch {
	case group1 != "":
		conds = " AND group_1 = $5"
		args = append(args, group1)
	case len(group1Filters) > 0:
		ph := make([]string, len(group1Filters))
		for i, v := range group1Filters {
			ph[i] = fmt.Sprintf("$%d", len(args)+1)
			args = append(args, v)
		}
		conds = " AND group_1 IN (" + strings.Join(ph, ",") + ")"
	}
	sql := `SELECT 'kpi'::text AS category, NULL::date AS periode_date,
                       ''::text AS periode_label,
                       COALESCE(SUM(display_value), 0) AS value,
                       0::numeric AS prev_value, 0::int AS order_seq
                FROM bi_fact_metric
                WHERE type = $1 AND periode_grain = $2
                  AND periode_date BETWEEN $3 AND $4` + conds + `
                  AND is_active = TRUE AND metric_name = 'VALUE'`
	return factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: "bi_fact_metric"}
}

// crossRatioValue returns numerator / denominator * scale for one period; 0 when the denominator is 0.
func crossRatioValue(
	ctx context.Context,
	repo factmetric.Repository,
	defaultType, grain string,
	k dashboarddomain.KpiEntry,
	group1Filters []string,
	per PeriodRange,
) (float64, error) {
	numRows, err := repo.QueryAggregate(ctx, crossRatioQuery(k.CrossRatioNumeratorType, defaultType, grain, k.CrossRatioNumeratorGroup1, group1Filters, per))
	if err != nil {
		return 0, fmt.Errorf("cross ratio numerator: %w", err)
	}
	denRows, err := repo.QueryAggregate(ctx, crossRatioQuery(k.CrossRatioDenominatorType, defaultType, grain, k.CrossRatioDenominatorGroup1, group1Filters, per))
	if err != nil {
		return 0, fmt.Errorf("cross ratio denominator: %w", err)
	}
	var num, den float64
	if len(numRows) > 0 {
		num = numRows[0].Value
	}
	if len(denRows) > 0 {
		den = denRows[0].Value
	}
	if den == 0 {
		return 0, nil
	}
	scale := k.CrossRatioScale
	if scale == 0 {
		scale = 1
	}
	return num / den * scale, nil
}

// runKPIScalar returns a single aggregated value for the given KPI definition + period.
func runKPIScalar(
	ctx context.Context,
	repo factmetric.Repository,
	d *dashboarddomain.Dashboard,
	k dashboarddomain.KpiEntry,
	period PeriodRange,
	scenario string,
	_ time.Time,
	group1Filters, group2Filters []string,
) (float64, error) {
	// When metric_name is specified (multi-metric dashboards like SALES),
	// query bi_fact_metric directly — MVs don't support per-metric-name aggregation for KPIs.
	if k.MetricName != "" {
		return runKPIScalarDirect(ctx, repo, d, k, period, scenario, group1Filters, group2Filters)
	}

	source, group1Filter := kpiSourceTable(d)
	args := []any{d.FilterType()}
	idx := 2
	conds := []string{"type = $1"}
	if group1Filter != "" {
		conds = append(conds, fmt.Sprintf("group_1 = $%d", idx))
		args = append(args, group1Filter)
		idx++
	}
	if k.FilterGroup2 != "" {
		source = mvBiMetricG2
		conds = append(conds, fmt.Sprintf("group_2 = $%d", idx))
		args = append(args, k.FilterGroup2)
		idx++
	}
	// Apply viewer filter-chip group_1 selections when no dashboard-level pre-filter is set.
	if group1Filter == "" && len(group1Filters) > 0 {
		conds, args, idx = appendINClause(conds, args, idx, "group_1", group1Filters)
	}
	// group_2 filter is only applicable when the source has a group_2 column.
	// mv_bi_metric_g1 aggregates across all group_2 values and does not expose the column —
	// adding a group_2 condition against it would fail with "column group_2 does not exist".
	if len(group2Filters) > 0 && source != mvBiMetricG1 {
		conds, args, idx = appendINClause(conds, args, idx, "group_2", group2Filters)
	}
	_ = idx
	conds = append(conds, fmt.Sprintf("periode_grain = $%d", idx))
	args = append(args, d.PeriodGrain().String())
	idx++
	if !period.From.IsZero() && !period.To.IsZero() {
		conds = append(conds, fmt.Sprintf("periode_date BETWEEN $%d AND $%d", idx, idx+1))
		args = append(args, period.From, period.To)
		idx += 2
	}
	conds = append(conds, fmt.Sprintf("scenario = $%d", idx))
	args = append(args, scenario)

	aggSQL := mapAgg(k.Agg) + "(value)"
	sql := fmt.Sprintf(`
SELECT 'kpi'::text AS category, NULL::date AS periode_date, ''::text AS periode_label,
       COALESCE(%s, 0) AS value, 0::numeric AS prev_value, 0::int AS order_seq
FROM %s WHERE %s`, aggSQL, source, joinAnd(conds))

	rows, err := repo.QueryAggregate(ctx, factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: source})
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].Value, nil
}

// sparklineStart returns the first date of the last n sparkline periods up to now. WEEKLY counts
// weeks (n Mondays back), not months: sparkline_periods=12 on a weekly dashboard is 12 points.
func sparklineStart(n int, grain, calendar string, now time.Time) time.Time {
	switch {
	case grain == grainWeekly:
		return mondayOf(now).AddDate(0, 0, -7*(n-1))
	case calendar == PayrollCalendar:
		return PayrollTrailingStart(n, grain, now)
	default:
		return shiftByMonths(now, -n+1)
	}
}

// runSparkline returns the last N period values for the same KPI scope.
func runSparkline(
	ctx context.Context,
	repo factmetric.Repository,
	d *dashboarddomain.Dashboard,
	k dashboarddomain.KpiEntry,
	periods int,
	now time.Time,
	group1Filters, group2Filters []string,
) ([]float64, error) {
	source, group1Filter := kpiSourceTable(d)
	args := []any{d.FilterType()}
	idx := 2
	conds := []string{"type = $1"}
	if group1Filter != "" {
		conds = append(conds, fmt.Sprintf("group_1 = $%d", idx))
		args = append(args, group1Filter)
		idx++
	}
	if group1Filter == "" && len(group1Filters) > 0 {
		conds, args, idx = appendINClause(conds, args, idx, "group_1", group1Filters)
	}
	if k.FilterGroup2 != "" {
		source = mvBiMetricG2
		conds = append(conds, fmt.Sprintf("group_2 = $%d", idx))
		args = append(args, k.FilterGroup2)
		idx++
	}
	// mv_bi_metric_g1 does not have a group_2 column — skip the filter to avoid SQL error.
	if len(group2Filters) > 0 && source != mvBiMetricG1 {
		conds, args, idx = appendINClause(conds, args, idx, "group_2", group2Filters)
	}
	conds = append(conds, fmt.Sprintf("periode_grain = $%d", idx))
	args = append(args, d.PeriodGrain().String())
	idx++

	// Use the last N periods up to `now`.
	startDate := sparklineStart(periods, d.PeriodGrain().String(), d.PeriodCalendar().String(), now)
	conds = append(conds, fmt.Sprintf("periode_date >= $%d AND periode_date <= $%d", idx, idx+1))
	args = append(args, startDate, now)
	idx += 2
	conds = append(conds, fmt.Sprintf("scenario = $%d", idx))
	args = append(args, "ACTUAL")

	aggSQL := mapAgg(k.Agg) + "(value)"
	sql := fmt.Sprintf(`
SELECT TO_CHAR(periode_date,'YYYYMM')::text AS category, periode_date, ''::text AS periode_label,
       COALESCE(%s, 0) AS value, 0::numeric AS prev_value, 0::int AS order_seq
FROM %s WHERE %s
GROUP BY periode_date
ORDER BY periode_date`, aggSQL, source, joinAnd(conds))

	rows, err := repo.QueryAggregate(ctx, factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: source})
	if err != nil {
		return nil, err
	}
	vals := make([]float64, 0, len(rows))
	for _, r := range rows {
		vals = append(vals, r.Value)
	}
	return vals, nil
}

// runKPIScalarDirect queries bi_fact_metric directly with a metric_name filter.
// Used for multi-metric dashboards (SALES type) where KPIs need per-metric aggregation.
func runKPIScalarDirect(
	ctx context.Context,
	repo factmetric.Repository,
	d *dashboarddomain.Dashboard,
	k dashboarddomain.KpiEntry,
	period PeriodRange,
	scenario string,
	group1Filters, group2Filters []string,
) (float64, error) {
	args := []any{d.FilterType(), k.MetricName}
	idx := 3
	conds := []string{"type = $1", "metric_name = $2", "is_active = TRUE"}
	if k.FilterGroup2 != "" {
		conds = append(conds, fmt.Sprintf("group_2 = $%d", idx))
		args = append(args, k.FilterGroup2)
		idx++
	}

	if d.FilterGroup1() != "" {
		conds = append(conds, fmt.Sprintf("group_1 = $%d", idx))
		args = append(args, d.FilterGroup1())
		idx++
	} else if len(group1Filters) > 0 {
		conds, args, idx = appendINClause(conds, args, idx, "group_1", group1Filters)
	}
	if len(group2Filters) > 0 {
		conds, args, idx = appendINClause(conds, args, idx, "group_2", group2Filters)
	}
	if g := d.PeriodGrain().String(); g != "" {
		conds = append(conds, fmt.Sprintf("periode_grain = $%d", idx))
		args = append(args, g)
		idx++
	}
	if !period.From.IsZero() && !period.To.IsZero() {
		conds = append(conds, fmt.Sprintf("periode_date BETWEEN $%d AND $%d", idx, idx+1))
		args = append(args, period.From, period.To)
		idx += 2
	}
	conds = append(conds, fmt.Sprintf("scenario = $%d", idx))
	args = append(args, scenario)

	aggSQL := mapAgg(k.Agg) + "(display_value)"
	sql := fmt.Sprintf(`
SELECT 'kpi'::text AS category, NULL::date AS periode_date, ''::text AS periode_label,
       COALESCE(%s, 0) AS value, 0::numeric AS prev_value, 0::int AS order_seq
FROM bi_fact_metric WHERE %s`, aggSQL, joinAnd(conds))

	rows, err := repo.QueryAggregate(ctx, factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: "bi_fact_metric"})
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].Value, nil
}

// mvBiMetricG1 is the materialized view aggregated at group_1 level.
// It does not expose group_2 — adding a group_2 WHERE condition against it fails
// with "column group_2 does not exist". runKPIScalar and runSparkline guard on this.
const mvBiMetricG1 = "mv_bi_metric_g1"

// mvBiMetricG2 is the materialized view aggregated at group_2 level (KPI filter_group_2).
const mvBiMetricG2 = "mv_bi_metric_g2"

// kpiSourceTable picks the right MV given whether the dashboard pre-filters group_1.
// The source table is always mv_bi_metric_g1 in this implementation; different MVs may
// be introduced in future when per-grain materialized views are available.
//
//nolint:unparam // source is always mv_bi_metric_g1; reserved for future per-grain MV routing
func kpiSourceTable(d *dashboarddomain.Dashboard) (source, group1Filter string) {
	if d.FilterGroup1() != "" {
		return mvBiMetricG1, d.FilterGroup1()
	}
	return mvBiMetricG1, ""
}

// resolveKPIPeriod maps a per-KPI period scope to a concrete date window. "selected" (and any
// unrecognized value) inherits the viewer's selected period; the fixed scopes are relative to now
// so the KPI's label stays accurate regardless of what the viewer picked.
//
// "selected_ytd" is a dynamic scope: it computes Jan 1 of the selected period's year to
// selected.To. This lets KPIs like "YTD EBITDA" follow the month picker. When the user
// selects May 2026 the KPI shows Jan to May 2026 (not Jan to today as "ytd" would).
// When no month is selected, selected_ytd anchors on selected.To (e.g. today = Jun 2026)
// giving Jan to Jun 2026, which is still the current YTD. Combined with compare="YTD_vs_LY":
//
//	May 2026 -> value=Jan-May 2026, compare=Jan-May 2025
//	May 2025 -> value=Jan-May 2025, compare=Jan-May 2024
//	Jan 2026 -> value=Jan 2026,     compare=Jan 2025 (actual YoY for January)
func resolveKPIPeriod(scope string, selected PeriodRange, now time.Time, grain, calendar string) PeriodRange {
	switch scope {
	case "current_month":
		if calendar == PayrollCalendar {
			return ResolvePeriodCal("THIS_MONTH", time.Time{}, time.Time{}, grain, calendar, now)
		}
		return PeriodRange{From: firstOfMonth(now), To: now}
	case "ytd":
		return PeriodRange{From: time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()), To: now}
	case "this_week":
		return ResolvePeriodCal("THIS_WEEK", time.Time{}, time.Time{}, grain, calendar, now)
	case "last_week":
		return ResolvePeriodCal("LAST_WEEK", time.Time{}, time.Time{}, grain, calendar, now)
	case "selected_ytd":
		// YTD anchored to the selected period's end date: Jan 1 of that year to selected.To.
		// Falls back to current YTD when the selected period is zero.
		anchor := selected.To
		if anchor.IsZero() {
			anchor = now
		}
		return PeriodRange{
			From: time.Date(anchor.Year(), 1, 1, 0, 0, 0, 0, anchor.Location()),
			To:   anchor,
		}
	case "l12m":
		if calendar == PayrollCalendar {
			return ResolvePeriodCal("L12M", time.Time{}, time.Time{}, grain, calendar, now)
		}
		return PeriodRange{From: shiftByMonths(now, -12), To: now}
	default:
		return selected
	}
}

// KPI compare-mode keys.
const (
	kpiCmpMoM = "MoM"
	kpiCmpQoQ = "QoQ"
	kpiCmpYoY = "YoY"
)

// compareRange computes the [from, to] window for a KPI compare mode.
func compareRange(current PeriodRange, mode, grain, calendar string) (PeriodRange, string) {
	if current.From.IsZero() || current.To.IsZero() {
		return PeriodRange{}, ""
	}
	if grain == grainWeekly {
		return compareRangeWeekly(current, mode, grain)
	}
	if calendar == PayrollCalendar {
		// Payroll periods are 26..25: shift both ends by whole months on their own day (25/26 never
		// overflow) instead of snapping to the 1st, so MONTHLY facts dated the 25th line up (HD-06).
		switch mode {
		case kpiCmpMoM:
			return shiftPayroll(current, -1, grain)
		case kpiCmpQoQ:
			return shiftPayroll(current, -3, grain)
		}
	}
	switch mode {
	case kpiCmpMoM:
		return PeriodRange{From: shiftByMonths(current.From, -1), To: shiftByMonths(current.To, -1)},
			PeriodLabel(shiftByMonths(current.To, -1), grain)
	case kpiCmpQoQ:
		return PeriodRange{From: shiftByMonths(current.From, -3), To: shiftByMonths(current.To, -3)},
			PeriodLabel(shiftByMonths(current.To, -3), grain)
	case kpiCmpYoY:
		return PeriodRange{From: current.From.AddDate(-1, 0, 0), To: current.To.AddDate(-1, 0, 0)},
			PeriodLabel(current.To.AddDate(-1, 0, 0), grain)
	case "YTD_vs_LY":
		// YTD vs LY: shift both ends back 1 year (year-to-date comparable window)
		return PeriodRange{From: current.From.AddDate(-1, 0, 0), To: current.To.AddDate(-1, 0, 0)},
			"YTD " + current.To.AddDate(-1, 0, 0).Format("2006")
	}
	return PeriodRange{}, ""
}

// shiftPayroll moves a payroll-calendar window back by `months` whole months.
func shiftPayroll(current PeriodRange, months int, grain string) (PeriodRange, string) {
	to := current.To.AddDate(0, months, 0)
	return PeriodRange{From: current.From.AddDate(0, months, 0), To: to}, PeriodLabel(to, grain)
}

// compareRangeWeekly shifts in whole weeks so the Monday anchor of WEEKLY facts is kept
// (HD-05: prev = −7 days, YoY = −52 weeks).
func compareRangeWeekly(current PeriodRange, mode, grain string) (PeriodRange, string) {
	var days int
	switch mode {
	case kpiCmpMoM:
		days = -7
	case kpiCmpQoQ:
		days = -13 * 7
	case kpiCmpYoY, "YTD_vs_LY":
		days = -52 * 7
	default:
		return PeriodRange{}, ""
	}
	to := current.To.AddDate(0, 0, days)
	return PeriodRange{From: current.From.AddDate(0, 0, days), To: to}, PeriodLabel(to, grain)
}

// mapAgg translates a KPI agg key to a SQL aggregate function.
func mapAgg(agg string) string {
	switch agg {
	case "avg":
		return "AVG"
	case "min":
		return "MIN"
	case "max":
		return "MAX"
	case "last":
		// "last" is interpreted as the value at the max period_date.
		// We approximate via MAX which works only when values monotonically increase.
		// For accurate "last" semantics, an explicit subquery is needed; that's a
		// future refinement when a use-case appears.
		return "MAX"
	}
	return "SUM"
}

// joinAnd joins WHERE conditions with " AND ".
func joinAnd(conds []string) string {
	return strings.Join(conds, " AND ")
}

// abs avoids importing math just for one call.
func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
