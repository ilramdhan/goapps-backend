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
) ([]factmetric.KpiRow, error) {
	kpis := d.KpiConfig()
	if len(kpis) == 0 {
		return nil, nil
	}
	out := make([]factmetric.KpiRow, 0, len(kpis))
	for _, k := range kpis {
		row, err := computeOneKPI(ctx, repo, d, k, periodRange, now)
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
) (factmetric.KpiRow, error) {
	if k.Agg == "cross_ratio" {
		return computeCrossRatioKPI(ctx, repo, d, k, now)
	}

	// Each KPI may scope its own window (e.g. "Current Month" vs "YTD") independently
	// of the period the viewer selected for the main chart; "selected" inherits it.
	effPeriod := resolveKPIPeriod(k.Period, period, now)
	currentVal, err := runKPIScalar(ctx, repo, d, k, effPeriod, "ACTUAL", now)
	if err != nil {
		return factmetric.KpiRow{}, err
	}
	row := factmetric.KpiRow{
		Label: k.Label,
		Value: currentVal,
	}
	if k.Compare != "none" && k.Compare != "" {
		comparePeriod, compareLabel := compareRange(effPeriod, k.Compare, d.PeriodGrain().String(), now)
		if !comparePeriod.From.IsZero() {
			compareVal, err := runKPIScalar(ctx, repo, d, k, comparePeriod, "ACTUAL", now)
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
		spark, err := runSparkline(ctx, repo, d, k, periods, now)
		if err != nil {
			return factmetric.KpiRow{}, err
		}
		row.Sparkline = spark
	}
	return row, nil
}

// computeCrossRatioKPI computes SUM(numerator_group_1) / NULLIF(SUM(denominator_group_1), 0) * scale.
// Both groups are queried from bi_fact_metric with the dashboard's type, grain, and the KPI's period scope.
// Returns 0 with no error when the denominator is 0 or data is absent.
func computeCrossRatioKPI(
	ctx context.Context,
	repo factmetric.Repository,
	d *dashboarddomain.Dashboard,
	k dashboarddomain.KpiEntry,
	now time.Time,
) (factmetric.KpiRow, error) {
	effPeriod := resolveKPIPeriod(k.Period, PeriodRange{
		From: shiftByMonths(now, -12),
		To:   now,
	}, now)

	grain := d.PeriodGrain().String()
	buildQuery := func(group1 string) factmetric.PlannedQuery {
		args := []any{d.FilterType(), group1, grain, effPeriod.From, effPeriod.To}
		sql := `SELECT 'kpi'::text AS category, NULL::date AS periode_date,
                       ''::text AS periode_label,
                       COALESCE(SUM(display_value), 0) AS value,
                       0::numeric AS prev_value, 0::int AS order_seq
                FROM bi_fact_metric
                WHERE type = $1 AND group_1 = $2 AND periode_grain = $3
                  AND periode_date BETWEEN $4 AND $5
                  AND is_active = TRUE AND metric_name = 'VALUE'`
		return factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: "bi_fact_metric"}
	}

	numRows, err := repo.QueryAggregate(ctx, buildQuery(k.CrossRatioNumeratorGroup1))
	if err != nil {
		return factmetric.KpiRow{Label: k.Label}, fmt.Errorf("cross ratio numerator: %w", err)
	}

	denRows, err := repo.QueryAggregate(ctx, buildQuery(k.CrossRatioDenominatorGroup1))
	if err != nil {
		return factmetric.KpiRow{Label: k.Label}, fmt.Errorf("cross ratio denominator: %w", err)
	}

	var num, den float64
	if len(numRows) > 0 {
		num = numRows[0].Value
	}
	if len(denRows) > 0 {
		den = denRows[0].Value
	}

	var ratio float64
	if den != 0 {
		scale := k.CrossRatioScale
		if scale == 0 {
			scale = 1
		}
		ratio = num / den * scale
	}
	return factmetric.KpiRow{Label: k.Label, Value: ratio}, nil
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
) (float64, error) {
	// When metric_name is specified (multi-metric dashboards like SALES),
	// query bi_fact_metric directly — MVs don't support per-metric-name aggregation for KPIs.
	if k.MetricName != "" {
		return runKPIScalarDirect(ctx, repo, d, k, period, scenario)
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

// runSparkline returns the last N period values for the same KPI scope.
func runSparkline(
	ctx context.Context,
	repo factmetric.Repository,
	d *dashboarddomain.Dashboard,
	k dashboarddomain.KpiEntry,
	periods int,
	now time.Time,
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
	conds = append(conds, fmt.Sprintf("periode_grain = $%d", idx))
	args = append(args, d.PeriodGrain().String())
	idx++

	// Use the last N periods up to `now`.
	startDate := shiftByMonths(now, -periods+1)
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
) (float64, error) {
	args := []any{d.FilterType(), k.MetricName}
	idx := 3
	conds := []string{"type = $1", "metric_name = $2", "is_active = TRUE"}

	if d.FilterGroup1() != "" {
		conds = append(conds, fmt.Sprintf("group_1 = $%d", idx))
		args = append(args, d.FilterGroup1())
		idx++
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

// kpiSourceTable picks the right MV given whether the dashboard pre-filters group_1.
// The source table is always mv_bi_metric_g1 in this implementation; different MVs may
// be introduced in future when per-grain materialized views are available.
//
//nolint:unparam // source is always mv_bi_metric_g1; reserved for future per-grain MV routing
func kpiSourceTable(d *dashboarddomain.Dashboard) (source, group1Filter string) {
	if d.FilterGroup1() != "" {
		return "mv_bi_metric_g1", d.FilterGroup1()
	}
	return "mv_bi_metric_g1", ""
}

// resolveKPIPeriod maps a per-KPI period scope to a concrete date window. "selected" (and any
// unrecognized value) inherits the viewer's selected period; the fixed scopes are relative to now
// so the KPI's label stays accurate regardless of what the viewer picked.
func resolveKPIPeriod(scope string, selected PeriodRange, now time.Time) PeriodRange {
	switch scope {
	case "current_month":
		return PeriodRange{From: firstOfMonth(now), To: now}
	case "ytd":
		return PeriodRange{From: time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()), To: now}
	case "l12m":
		return PeriodRange{From: shiftByMonths(now, -12), To: now}
	default:
		return selected
	}
}

// compareRange computes the [from, to] window for a KPI compare mode.
func compareRange(current PeriodRange, mode, grain string, _ time.Time) (PeriodRange, string) {
	if current.From.IsZero() || current.To.IsZero() {
		return PeriodRange{}, ""
	}
	switch mode {
	case "MoM":
		return PeriodRange{From: shiftByMonths(current.From, -1), To: shiftByMonths(current.To, -1)},
			PeriodLabel(shiftByMonths(current.To, -1), grain)
	case "QoQ":
		return PeriodRange{From: shiftByMonths(current.From, -3), To: shiftByMonths(current.To, -3)},
			PeriodLabel(shiftByMonths(current.To, -3), grain)
	case "YoY":
		return PeriodRange{From: current.From.AddDate(-1, 0, 0), To: current.To.AddDate(-1, 0, 0)},
			PeriodLabel(current.To.AddDate(-1, 0, 0), grain)
	case "YTD_vs_LY":
		// YTD vs LY: shift both ends back 1 year (year-to-date comparable window)
		return PeriodRange{From: current.From.AddDate(-1, 0, 0), To: current.To.AddDate(-1, 0, 0)},
			"YTD " + current.To.AddDate(-1, 0, 0).Format("2006")
	}
	return PeriodRange{}, ""
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
