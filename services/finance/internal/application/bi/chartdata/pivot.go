package chartdata

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// xAxisPeriod is the chart_config x_axis_field value meaning "one column per period".
const xAxisPeriod = "period"

// Period grains with their own handling (WEEKLY label/shift, DAILY Go label in PeriodLabel).
const (
	grainDaily  = "DAILY"
	grainWeekly = "WEEKLY"
)

// wibZone is the business time zone of the HR reports (Asia/Jakarta, UTC+7).
var wibZone = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*60*60)
	}
	return loc
}()

// wibDate returns t's calendar date in WIB as a UTC-midnight date, the form periode_date uses.
func wibDate(t time.Time) time.Time {
	w := t.In(wibZone)
	return time.Date(w.Year(), w.Month(), w.Day(), 0, 0, 0, 0, time.UTC)
}

// pivotDims returns the x (category) and series columns of a two-dimensional chart, or ok=false
// when the dashboard renders the default one-dimensional series (H013).
//
// A stacked_bar pivots x_axis_field × stack_by_field (e.g. dept × OT type); a heatmap pivots
// x_axis_field × y_axis_field (e.g. dept × week). Both columns come from a closed whitelist —
// group_3 is never a pivot dimension, so the pivot cannot expose NIKs (HD-07). The pivot only
// applies to the top level of an unpinned dashboard: drilling, force-trend and computed-ratio
// requests keep the existing planner paths.
func pivotDims(d *dashboarddomain.Dashboard, f ViewerFilters) (x, series string, ok bool) {
	if len(f.DrillPath) > 0 || f.ForceTrend || f.ComputedRatio != nil || d.FilterGroup1() != "" {
		return "", "", false
	}
	cfg := d.ChartConfig()
	switch d.ChartType().String() {
	case "stacked_bar":
		series = cfg.StackByField
	case "heatmap":
		series = cfg.YAxisField
	default:
		return "", "", false
	}
	x = cfg.XAxisField
	if !pivotColumn(x) || !pivotColumn(series) || x == series {
		return "", "", false
	}
	return x, series, true
}

// pivotColumn whitelists the pivot dimensions: period, group_1 or group_2 (never group_3).
func pivotColumn(c string) bool {
	return c == xAxisPeriod || c == colGroup1 || c == colGroup2
}

// IsPivot reports whether Plan/Shape use the two-dimensional pivot path for these filters.
func IsPivot(d *dashboarddomain.Dashboard, f ViewerFilters) bool {
	_, _, ok := pivotDims(d, f)
	return ok
}

// periodLabelSQL returns the SQL expression formatting periode_date as the trend period_label.
// Only WEEKLY gets its own label (HD-05: IYYY-"W"IW, matches PeriodLabel); every other grain keeps
// the YYYYMM label the planner always emitted, because the viewer reads 6-digit labels as periods
// (PR #236 review: DAILY/QUARTERLY/YEARLY dashboards must not change).
func periodLabelSQL(grain string) string { return periodLabelExpr(grain, "periode_date") }

// periodLabelExpr is periodLabelSQL for an arbitrary (trusted, constant) date column.
func periodLabelExpr(grain, col string) string {
	if grain == grainWeekly {
		return "TO_CHAR(" + col + `, 'IYYY-"W"IW')`
	}
	return "TO_CHAR(" + col + ", 'YYYYMM')"
}

// planPivot builds the two-dimensional aggregate. Rows reuse the canonical 6-column shape with
// category = series value and periode_label = x value (period label or group value); shapePivot
// turns them into one Series per series value.
//
// Reads mv_bi_metric_g2 (both pivot columns are group_1/group_2), so facts with group_2 NULL are
// not part of a pivot. chart_config.filter_group_2, when set, pins group_2 (e.g. OVER_LIMIT).
func planPivot(d *dashboarddomain.Dashboard, f ViewerFilters, now time.Time, x, series string) (factmetric.PlannedQuery, error) {
	grain := d.PeriodGrain().String()
	period := ResolvePeriodCal(f.PeriodPreset, f.PeriodFrom, f.PeriodTo, grain, d.PeriodCalendar().String(), now)

	args := []any{d.FilterType(), grain, "ACTUAL"}
	conds := []string{"type = $1", "periode_grain = $2", "scenario = $3"}
	idx := 4
	if !period.From.IsZero() && !period.To.IsZero() {
		conds = append(conds, fmt.Sprintf("periode_date BETWEEN $%d AND $%d", idx, idx+1))
		args = append(args, period.From, period.To)
		idx += 2
	}
	if g2 := d.ChartConfig().FilterGroup2; g2 != "" {
		conds = append(conds, fmt.Sprintf("group_2 = $%d", idx))
		args = append(args, g2)
		idx++
	}
	if len(f.Group1Filter) > 0 {
		conds, args, idx = appendINClause(conds, args, idx, colGroup1, f.Group1Filter)
	}
	if len(f.Group2Filter) > 0 {
		conds, args, _ = appendINClause(conds, args, idx, colGroup2, f.Group2Filter)
	}

	// x/series are whitelisted column names (pivotDims), never user input.
	sql := fmt.Sprintf(`
SELECT %s::text AS category, NULL::date AS periode_date, %s::text AS periode_label,
       SUM(value) AS value, 0::numeric AS prev_value, 0::int AS order_seq
FROM mv_bi_metric_g2
WHERE %s
GROUP BY 1, 3`, pivotExpr(series, grain), pivotExpr(x, grain), strings.Join(conds, " AND "))
	return factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: "mv_bi_metric_g2"}, nil
}

// pivotExpr returns the SQL expression of a whitelisted pivot column.
func pivotExpr(c, grain string) string {
	if c == xAxisPeriod {
		return periodLabelSQL(grain)
	}
	return "COALESCE(" + c + ", '')"
}

// shapePivot turns pivot rows into one Series per series value over the sorted x categories.
// Both axes are sorted ascending (period labels sort chronologically); missing cells are 0.
//
// fill lists x labels that must appear even without rows (weeks of the window, H018): an
// empty cell is a real 0, not a missing column.
func shapePivot(rows []factmetric.AggRow, numFmt chart.NumberFormat, decimals int, fill []string) ([]Series, []string) {
	cells := make(map[string]map[string]float64)
	xSet := make(map[string]struct{})
	for _, r := range rows {
		if cells[r.Category] == nil {
			cells[r.Category] = make(map[string]float64)
		}
		cells[r.Category][r.PeriodLabel] += r.Value
		xSet[r.PeriodLabel] = struct{}{}
	}
	for _, l := range fill {
		xSet[l] = struct{}{}
	}
	cats := sortedKeys(xSet)
	names := make([]string, 0, len(cells))
	for n := range cells {
		names = append(names, n)
	}
	sort.Strings(names)

	series := make([]Series, 0, len(names))
	for _, n := range names {
		pts := make([]DataPoint, 0, len(cats))
		for _, c := range cats {
			v := cells[n][c]
			pts = append(pts, DataPoint{Category: c, Value: v, Label: chart.Format(v, numFmt, decimals)})
		}
		series = append(series, Series{Name: n, Points: pts})
	}
	return series, cats
}

// sortedKeys sorts pivot labels ascending; labels starting with a number (hour buckets
// "0-7" < "7-14" < "14-18", H012) compare numerically on that prefix, ">" buckets last.
func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		ni, okI := leadingNumber(out[i])
		nj, okJ := leadingNumber(out[j])
		if okI && okJ && ni != nj {
			return ni < nj
		}
		return out[i] < out[j]
	})
	return out
}

// leadingNumber parses the integer prefix of a label; ">N" sorts after every "N-…" bucket.
func leadingNumber(s string) (int, bool) {
	if rest, ok := strings.CutPrefix(s, ">"); ok {
		n, isNum := leadingNumber(rest)
		return n + 1_000_000, isNum
	}
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:end])
	return n, err == nil
}

// pivotFillLabels returns the Monday labels of the resolved window when the pivot x axis is the
// period of a WEEKLY dashboard (otherwise nil), so a week without facts shows 0 instead of no
// column (H018). The window is resolved with the same WIB anchor Plan uses for WEEKLY
// (resolveAsOf); DAILY is not filled because its anchor is the latest loaded date.
func pivotFillLabels(d *dashboarddomain.Dashboard, f ViewerFilters, now time.Time, x string) []string {
	if x != xAxisPeriod || d.PeriodGrain().String() != grainWeekly {
		return nil
	}
	p := ResolvePeriodCal(f.PeriodPreset, f.PeriodFrom, f.PeriodTo, grainWeekly, d.PeriodCalendar().String(), wibDate(now))
	if p.From.IsZero() || p.To.IsZero() || p.To.Before(p.From) {
		return nil
	}
	from := dayOf(p.From)
	cur := mondayOf(from)
	if cur.Before(from) {
		cur = cur.AddDate(0, 0, 7)
	}
	var out []string
	for ; !cur.After(p.To) && len(out) < maxFillLabels; cur = cur.AddDate(0, 0, 7) {
		out = append(out, PeriodLabel(cur, grainWeekly))
	}
	return out
}

// maxFillLabels caps zero-filled x labels (≈ 15 years of weeks).
const maxFillLabels = 800
