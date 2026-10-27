package chartdata

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/chart"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// tableModeEmployeeWeek is the chart_config.table_mode of the R2 employee table (H018).
const tableModeEmployeeWeek = "employee_week"

// Column keys of the employee table; each becomes one Series (= one table column).
const (
	colHours   = "HOURS"
	colExcess  = "EXCESS"
	colChronic = "CHRONIC"
)

// empSep separates NIK, dept and name inside AggRow.Category (unit separator, never in real data).
const empSep = "\x1f"

// isEmployeeTable reports whether the dashboard uses an employee table planner (employee_week,
// employee_ot). Both list NIKs, so both are gated as employee level (HD-07, HD-18).
func isEmployeeTable(d *dashboarddomain.Dashboard) bool {
	m := d.ChartConfig().TableMode
	return (m == tableModeEmployeeWeek || m == tableModeEmployeeOT) && IsHRType(d.FilterType())
}

// isEmployeeOT reports whether the dashboard uses the Employee Overtime planner (HD-18).
func isEmployeeOT(d *dashboarddomain.Dashboard) bool {
	return isEmployeeTable(d) && d.ChartConfig().TableMode == tableModeEmployeeOT
}

// planEmployeeTable builds the R2 employee table (H018) from bi_fact_metric, because the MVs drop
// facts whose group_2 is NULL (EXCESS/CHRONIC). The week is the latest HR_OT_WEEK week inside the
// resolved window (CHRONIC is taken for that week only). Only employees over the limit that week
// are listed. With DrillPath = [nik] it returns that employee's daily hours of the same week.
//
// Every value is a bound arg; only constants are concatenated into the SQL. Access is gated by
// RequestsEmployeeLevel (isEmployeeTable) in GetData and Preview before the plan is built.
// The week is the latest Monday whose Sunday is on/after the window start (K10, week belongs to
// the period of its Sunday) and whose Monday is on/before the window end.
func planEmployeeTable(d *dashboarddomain.Dashboard, f ViewerFilters, now time.Time) (factmetric.PlannedQuery, error) {
	if n := len(f.DrillPath); n > d.MaxDrillLevel().Int() {
		return factmetric.PlannedQuery{}, fmt.Errorf("%w: depth %d > max %d", factmetric.ErrDrillTooDeep, n, d.MaxDrillLevel().Int())
	}
	period := ResolvePeriodCal(f.PeriodPreset, f.PeriodFrom, f.PeriodTo, grainWeekly, d.PeriodCalendar().String(), wibDate(now))
	args := []any{d.FilterType(), period.From, period.To}
	const weekCTE = `WITH w AS (
  SELECT MAX(periode_date) AS d FROM bi_fact_metric
  WHERE type = $1 AND periode_grain = 'WEEKLY' AND is_active = TRUE
    AND periode_date + 6 >= $2 AND periode_date <= $3)`

	if len(f.DrillPath) > 0 {
		args = append(args, f.DrillPath[0], TypeWeekDailyName)
		sql := weekCTE + `
SELECT group_3::text AS category, periode_date, TO_CHAR(periode_date, 'YYYY-MM-DD')::text AS periode_label,
       SUM(value) AS value, 0::numeric AS prev_value, 0::int AS order_seq
FROM bi_fact_metric, w
WHERE type = $5 AND periode_grain = 'DAILY' AND is_active = TRUE AND group_3 = $4
  AND periode_date BETWEEN w.d AND w.d + 6
GROUP BY group_3, periode_date
ORDER BY periode_date`
		return factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: "bi_fact_metric"}, nil
	}

	args = append(args, TypeWeekExcessName, TypeChronicName)
	sql := weekCTE + `
SELECT f.group_3 || chr(31) || COALESCE(MAX(f.group_1), '') || chr(31) || COALESCE(MAX(e.emp_name), '') AS category,
       NULL::date AS periode_date,
       CASE f.type WHEN $1 THEN '` + colHours + `' WHEN $4 THEN '` + colExcess + `' ELSE '` + colChronic + `' END AS periode_label,
       CASE WHEN f.type = $5 THEN MAX(f.value) ELSE SUM(f.value) END AS value, 0::numeric AS prev_value, 0::int AS order_seq
FROM bi_fact_metric f
JOIN w ON f.periode_date = w.d
LEFT JOIN bi_hr_employee e ON e.nik = f.group_3
WHERE f.type IN ($1, $4, $5) AND f.periode_grain = 'WEEKLY' AND f.is_active = TRUE
  AND COALESCE(f.group_3, '') <> ''
  AND f.group_3 IN (SELECT group_3 FROM bi_fact_metric x, w WHERE x.type = $4 AND x.periode_grain = 'WEEKLY' AND x.periode_date = w.d AND x.is_active = TRUE AND x.value > 0)
GROUP BY f.group_3, f.type`
	return factmetric.PlannedQuery{SQL: sql, Args: args, TargetTable: "bi_fact_metric"}, nil
}

// Fact type names the employee table reads besides the dashboard's own HR_OT_WEEK.
const (
	TypeWeekExcessName = "HR_OT_WEEK_EXCESS"
	TypeChronicName    = "HR_OT_CHRONIC"
	TypeWeekDailyName  = "HR_OT_WEEK_DAILY"
)

// shapeEmployeeTable turns the (nik|dept|name, column, value) rows into one Series per column
// (HOURS, EXCESS, CHRONIC) over NIK categories ordered by hours descending (NIK breaks ties).
// Name and dept ride in DataPoint.Meta of every point.
func shapeEmployeeTable(rows []factmetric.AggRow, numFmt chart.NumberFormat, decimals int) ([]Series, []string) {
	type emp struct{ dept, name string }
	info := map[string]emp{}
	vals := map[string]map[string]float64{}
	for _, r := range rows {
		parts := strings.SplitN(r.Category, empSep, 3)
		nik := parts[0]
		if len(parts) == 3 {
			info[nik] = emp{dept: parts[1], name: parts[2]}
		}
		if vals[nik] == nil {
			vals[nik] = map[string]float64{}
		}
		vals[nik][r.PeriodLabel] += r.Value
	}
	niks := make([]string, 0, len(vals))
	for n := range vals {
		niks = append(niks, n)
	}
	sort.Slice(niks, func(i, j int) bool {
		hi, hj := vals[niks[i]][colHours], vals[niks[j]][colHours]
		if hi != hj {
			return hi > hj
		}
		return niks[i] < niks[j]
	})
	cols := []struct {
		key string
		fmt chart.NumberFormat
		dec int
	}{{colHours, numFmt, decimals}, {colExcess, numFmt, decimals}, {colChronic, chart.NumberFormat("raw"), 0}}
	series := make([]Series, 0, len(cols))
	for _, c := range cols {
		pts := make([]DataPoint, 0, len(niks))
		for _, n := range niks {
			v := vals[n][c.key]
			pts = append(pts, DataPoint{Category: n, Value: v, Label: chart.Format(v, c.fmt, c.dec),
				Meta: map[string]any{"name": info[n].name, "dept": info[n].dept}})
		}
		series = append(series, Series{Name: c.key, Points: pts})
	}
	return series, niks
}
