package chartdata

import (
	"fmt"
	"strings"
	"time"

	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// ViewerFilters carries the runtime filter state from the viewer page.
type ViewerFilters struct {
	PeriodPreset string    // L12M | L24M | THIS_YEAR | THIS_QTR | THIS_MONTH | ALL | CUSTOM
	PeriodFrom   time.Time // valid when CUSTOM
	PeriodTo     time.Time
	Compare      string   // none | MoM | QoQ | YoY | YTD | R12
	DrillPath    []string // depth 0 = aggregate, 1 = into group_2, 2 = into group_3
}

// Plan turns a Dashboard + ViewerFilters into a PlannedQuery against the right MV / fact table.
//
// Drill-depth rules:
//   - 0 → mv_bi_metric_g1 (or filtered by dashboard.filter_group_1)
//   - 1 → mv_bi_metric_g2
//   - 2 → bi_fact_metric (group_3 level, raw)
//   - >max_drill_level → ErrDrillTooDeep
//
// Compare modes produce an extra `prev_value` column via LEFT JOIN on shifted periods.
func Plan(d *dashboarddomain.Dashboard, f ViewerFilters, now time.Time) (factmetric.PlannedQuery, error) {
	drillDepth := len(f.DrillPath)
	if drillDepth > d.MaxDrillLevel().Int() {
		return factmetric.PlannedQuery{}, fmt.Errorf("%w: depth %d > max %d", factmetric.ErrDrillTooDeep, drillDepth, d.MaxDrillLevel().Int())
	}

	period := ResolvePeriod(f.PeriodPreset, f.PeriodFrom, f.PeriodTo, d.PeriodGrain().String(), now)

	// Trend charts (x_axis_field="period") group by period over time; categorical charts
	// (waterfall/bar/donut) group by the drill-level group column.
	isTrend := d.ChartConfig().XAxisField == "period"

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
		sql := fmt.Sprintf(`
SELECT TO_CHAR(periode_date, 'YYYYMMDD')::text AS category, periode_date, periode_label,
       SUM(value) AS value, 0::numeric AS prev_value, 0::int AS order_seq
FROM %s
WHERE %s
GROUP BY periode_date, periode_label
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
	sql := fmt.Sprintf(`
WITH cur AS (
    SELECT periode_date, periode_label, SUM(value) AS value
    FROM %s
    WHERE %s
    GROUP BY periode_date, periode_label
),
prev AS (
    SELECT periode_date, SUM(value) AS value
    FROM %s
    WHERE %s
    GROUP BY periode_date
)
SELECT TO_CHAR(cur.periode_date, 'YYYYMMDD')::text AS category,
       cur.periode_date, cur.periode_label,
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
