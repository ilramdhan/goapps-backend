package chartdata_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	chartdata "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
	redisinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/redis"
)

func employeeOTCfg() map[string]any {
	return map[string]any{"x_axis_field": "group_1", "y_axis_field": "value", "table_mode": "employee_ot"}
}

func employeeOTDashboard(t *testing.T) *dashboarddomain.Dashboard {
	t.Helper()
	return newPivotDashboard(t, "data_table", "WEEKLY", "", employeeOTCfg())
}

func isoDay(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

// otNow is 2026-10-21 10:00 WIB (Wednesday), inside payroll period 26 Sep – 25 Oct.
var otNow = time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC)

func TestEmployeeOT_GatedWithoutPermission(t *testing.T) {
	dash := employeeOTDashboard(t)
	repo := &mockDashboardRepo{getByCode: func(context.Context, string) (*dashboarddomain.Dashboard, error) { return dash, nil }}
	fact := &mockFactRepo{}
	h := chartdata.NewGetDataHandler(repo, fact, newMockCache(), func(any) string { return "h" }).WithNow(func() time.Time { return otNow })
	_, err := h.Handle(context.Background(), chartdata.GetDataQuery{
		DashboardCode: "HR_OT_EMPLOYEES", CanViewWorkforce: true,
		Filters: chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: "DEPT"},
	})
	if !errors.Is(err, dashboarddomain.ErrForbidden) || fact.queries != 0 {
		t.Fatalf("want forbidden without query, err=%v queries=%d", err, fact.queries)
	}
	res, err := h.Handle(context.Background(), chartdata.GetDataQuery{
		DashboardCode: "HR_OT_EMPLOYEES", CanViewWorkforce: true, CanViewEmployee: true,
		Filters: chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: "DEPT"},
	})
	if err != nil {
		t.Fatalf("with permission: %v", err)
	}
	// Range labels follow the handler clock (WIB today = Wed 21 Oct 2026), not time.Now.
	ranges := res.Config["employee_ot"].(map[string]any)["ranges"].(map[string]any)
	if ranges["period_from"] != "2026-09-26" || ranges["period_to"] != "2026-10-25" ||
		ranges["week_from"] != "2026-10-19" || ranges["week_to"] != "2026-10-25" || ranges["week_label"] != "2026-W43" {
		t.Errorf("ranges = %v", ranges)
	}
}

func TestEmployeeOT_GroupByWhitelist(t *testing.T) {
	d := employeeOTDashboard(t)
	for _, g := range []string{"", "DIVISION", "dept", " Dept ", "company"} {
		if _, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: g}, otNow); err != nil {
			t.Errorf("group_by %q: %v", g, err)
		}
	}
	for _, g := range []string{"group_1", "e.nik", "DEPT; DROP TABLE bi_fact_metric--", "1) OR (1=1"} {
		_, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: g}, otNow)
		if !errors.Is(err, factmetric.ErrInvalidPlan) {
			t.Errorf("group_by %q: want ErrInvalidPlan, got %v", g, err)
		}
	}
}

func TestEmployeeOT_GroupFilterIsBound(t *testing.T) {
	d := employeeOTDashboard(t)
	evil := "PRD') OR ('1'='1"
	p, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: "DEPT", GroupFilter: []string{evil, "QA"}}, otNow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.SQL, evil) || !strings.Contains(p.SQL, "COALESCE(e.dept_name, '') IN ($13,$14)") {
		t.Errorf("filter not bound:\n%s", p.SQL)
	}
	if p.Args[12] != evil || p.Args[13] != "QA" {
		t.Errorf("args = %v", p.Args)
	}
	if _, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupFilter: []string{"QA"}}, otNow); !errors.Is(err, factmetric.ErrInvalidPlan) {
		t.Errorf("filter without group_by: %v", err)
	}
	many := make([]string, 501)
	if _, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: "DEPT", GroupFilter: many}, otNow); !errors.Is(err, factmetric.ErrInvalidPlan) {
		t.Errorf("too many filter values: %v", err)
	}
}

func TestEmployeeOT_CacheHashDiffersPerGroup(t *testing.T) {
	base := chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH"}
	byDept, byDiv := base, base
	byDept.GroupBy, byDiv.GroupBy = "DEPT", "DIVISION"
	filtered := byDept
	filtered.GroupFilter = []string{"PRD"}
	hashes := map[string]bool{}
	for _, f := range []chartdata.ViewerFilters{base, byDept, byDiv, filtered} {
		hashes[redisinfra.HashFilters(f)] = true
	}
	if len(hashes) != 4 {
		t.Fatalf("want 4 distinct hashes, got %d", len(hashes))
	}
}

func TestEmployeeOT_RejectsWeekAndDayPresets(t *testing.T) {
	d := employeeOTDashboard(t)
	for _, p := range []string{"THIS_WEEK", "LAST_WEEK", "YESTERDAY", "CUSTOM", "ALL", "L24M"} {
		if _, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: p}, otNow); !errors.Is(err, factmetric.ErrInvalidPlan) {
			t.Errorf("preset %s: want ErrInvalidPlan, got %v", p, err)
		}
	}
	// Empty preset falls back to the dashboard default (L12M here).
	if _, err := chartdata.Plan(d, chartdata.ViewerFilters{}, otNow); err != nil {
		t.Errorf("default preset: %v", err)
	}
}

func TestResolveEmployeeOTWindows(t *testing.T) {
	cases := []struct {
		name, preset, today              string
		from, to, lastStart, week, first string
	}{
		// Week 23–29 Nov 2026 crosses the 25th/26th; its Sunday (29 Nov) is in period 26 Nov – 25 Dec (K10).
		{"minggu lintas 25/26 di periode Desember", "THIS_MONTH", "2026-11-27", "2026-12-25", "2026-12-25", "2026-11-26", "2026-11-23", "2026-11-26"},
		// Period 26 Oct – 25 Nov: last Sunday on/before 25 Nov is 22 Nov → week 16–22 Nov.
		{"periode lalu: minggu sebelum lintas", "LAST_MONTH", "2026-11-27", "2026-11-25", "2026-11-25", "2026-10-26", "2026-11-16", "2026-10-26"},
		// Running week (HD-17): today Wed 21 Oct, week 19–25 Oct belongs to period ending 25 Oct.
		{"minggu berjalan parsial", "THIS_MONTH", "2026-10-21", "2026-10-25", "2026-10-25", "2026-09-26", "2026-10-19", "2026-09-26"},
		{"L12M", "L12M", "2026-10-21", "2025-11-25", "2026-10-25", "2026-09-26", "2026-10-19", "2025-10-26"},
		{"THIS_QTR memuat periode berjalan", "THIS_QTR", "2026-10-21", "2026-10-25", "2026-10-25", "2026-09-26", "2026-10-19", "2026-09-26"},
		{"THIS_YEAR", "THIS_YEAR", "2026-10-21", "2026-01-25", "2026-10-25", "2026-09-26", "2026-10-19", "2025-12-26"},
		// From the 26th the running period belongs to the next month (K3): 28 Dec 2026 is period 202701,
		// so THIS_YEAR / THIS_QTR count periods of 2027 / Q1-2027, never 13 periods across years.
		{"THIS_YEAR setelah tgl 26 Des = tahun periode berjalan", "THIS_YEAR", "2026-12-28", "2027-01-25", "2027-01-25", "2026-12-26", "2026-12-28", "2026-12-26"},
		{"THIS_QTR setelah tgl 26 Des", "THIS_QTR", "2026-12-28", "2027-01-25", "2027-01-25", "2026-12-26", "2026-12-28", "2026-12-26"},
		{"THIS_QTR setelah tgl 26 Sep = Q4", "THIS_QTR", "2026-09-28", "2026-10-25", "2026-10-25", "2026-09-26", "2026-09-28", "2026-09-26"},
		{"THIS_QTR akhir kuartal", "THIS_QTR", "2026-12-20", "2026-10-25", "2026-12-25", "2026-11-26", "2026-12-14", "2026-09-26"},
		{"hari ini tgl 26", "THIS_MONTH", "2026-10-26", "2026-11-25", "2026-11-25", "2026-10-26", "2026-10-26", "2026-10-26"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, err := chartdata.ResolveEmployeeOTWindows(c.preset, isoDay(c.today))
			if err != nil {
				t.Fatal(err)
			}
			got := fmt.Sprintf("%s %s %s %s %s", w.PeriodFrom.Format(time.DateOnly), w.PeriodTo.Format(time.DateOnly),
				w.LastStart.Format(time.DateOnly), w.Week.Format(time.DateOnly), w.FirstStart.Format(time.DateOnly))
			want := strings.Join([]string{c.from, c.to, c.lastStart, c.week, c.first}, " ")
			if got != want {
				t.Errorf("got  %s\nwant %s", got, want)
			}
		})
	}
}

func TestEmployeeOT_PlanArgsCrossingWeek(t *testing.T) {
	d := employeeOTDashboard(t)
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	p, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Args[1] != isoDay("2026-12-25") || p.Args[2] != isoDay("2026-12-25") || p.Args[6] != isoDay("2026-11-23") {
		t.Errorf("args = %v", p.Args)
	}
	if p.Args[7] != chartdata.EmployeeOTMaxRows+1 || !strings.Contains(p.SQL, "LIMIT $8") {
		t.Errorf("row limit missing: %v", p.Args[7])
	}
}

func otRow(nik, group, col string, v float64) factmetric.AggRow {
	return otRowCompany(nik, group, "", col, v)
}

func otRowCompany(nik, group, company, col string, v float64) factmetric.AggRow {
	sep := "\x1f"
	return factmetric.AggRow{Category: nik + sep + group + sep + "Nama " + nik + sep + "DIV" + sep + group + sep + company, PeriodLabel: col, Value: v}
}

func TestShapeEmployeeOT_UnionZeroColumnsAndSubtotals(t *testing.T) {
	rows := []factmetric.AggRow{
		otRow("N1", "PRD", "PERIOD_HOURS", 40), // R1 only: week columns 0
		otRow("N2", "PRD", "W:2026-10-19", 16), // R2 only: period hours 0
		otRow("N2", "PRD", "EXCESS", 2),
		otRow("N2", "PRD", "CHRONIC", 3),
		otRow("N3", "QA", "PERIOD_HOURS", 10),
		otRow("N3", "QA", "W:2026-10-19", 5),
	}
	series, cats, block := chartdata.ShapeEmployeeOTForTest(rows, chartdata.ViewerFilters{GroupBy: "dept"})
	if strings.Join(cats, ",") != "N1,N2,N3" {
		t.Fatalf("cats = %v", cats)
	}
	if len(series) != 5 || series[0].Name != "PERIOD_HOURS" || series[3].Name != "CHRONIC" || series[4].Name != "W:2026-10-19" {
		t.Fatalf("series = %+v", series)
	}
	if series[0].Points[1].Value != 0 || series[4].Points[0].Value != 0 || series[3].Points[1].Value != 3 {
		t.Errorf("zero fill wrong: %+v", series)
	}
	if series[0].Points[0].Meta["group"] != "PRD" || series[0].Points[0].Meta["name"] != "Nama N1" {
		t.Errorf("meta = %v", series[0].Points[0].Meta)
	}
	if block["group_by"] != "DEPT" || block["truncated"] != false {
		t.Errorf("block = %v", block)
	}
	subs := block["subtotals"].([]any)
	prd := subs[0].(map[string]any)
	if len(subs) != 2 || prd["group"] != "PRD" || prd["employees"] != 2.0 || prd["PERIOD_HOURS"] != 40.0 || prd["W:2026-10-19"] != 16.0 || prd["EXCESS"] != 2.0 {
		t.Errorf("subtotals = %v", subs)
	}
	if _, ok := prd["CHRONIC"]; ok {
		t.Error("CHRONIC must not be summed")
	}
	// PRD: N1 40 h, N2 0 h (EXCESS 2) -> avg 20.0, 1 of 2 over. QA: N3 10 h, no excess.
	qa := subs[1].(map[string]any)
	if prd["avg_period_hours"] != 20.0 || prd["over_pct"] != 50.0 || qa["avg_period_hours"] != 10.0 || qa["over_pct"] != 0.0 {
		t.Errorf("subtotal metrics = %v / %v", prd, qa)
	}
	sum := block["summary"].(map[string]any)
	if sum["employees"] != 3.0 || sum["total_period_hours"] != 50.0 || sum["over_limit"] != 1.0 || sum["chronic"] != 1.0 {
		t.Errorf("summary = %v", sum)
	}
	opts := block["group_options"].([]any)
	if len(opts) != 2 || opts[0] != "DIVISION" || opts[1] != "DEPT" {
		t.Errorf("group_options = %v (COMPANY must be absent without company data, HD-19)", opts)
	}
}

func TestShapeEmployeeOT_CompanyOptionWhenLoaded(t *testing.T) {
	rows := []factmetric.AggRow{
		otRowCompany("N1", "PT. Mutu Gading Tekstil", "PT. Mutu Gading Tekstil", "PERIOD_HOURS", 40),
		otRowCompany("N2", "", "", "PERIOD_HOURS", 10),
	}
	series, _, block := chartdata.ShapeEmployeeOTForTest(rows, chartdata.ViewerFilters{GroupBy: "company"})
	opts := block["group_options"].([]any)
	if len(opts) != 3 || opts[2] != "COMPANY" {
		t.Errorf("group_options = %v (COMPANY must be offered once loaded, HD-19)", opts)
	}
	if block["group_by"] != "COMPANY" {
		t.Errorf("group_by = %v", block["group_by"])
	}
	if series[0].Points[1].Meta["company"] != "PT. Mutu Gading Tekstil" || series[0].Points[0].Meta["company"] != "" {
		t.Errorf("meta = %v / %v", series[0].Points[0].Meta, series[0].Points[1].Meta)
	}
}

func TestEmployeeOT_GroupByCompanyUsesEmployeeCompany(t *testing.T) {
	p, err := chartdata.Plan(employeeOTDashboard(t), chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: "COMPANY",
		GroupFilter: []string{"PT. Mutu Gading Tekstil"}}, otNow)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.SQL, "COALESCE(e.company, '')") || strings.Contains(p.SQL, "Mutu Gading") {
		t.Errorf("company group must be a bound filter on e.company:\n%s", p.SQL)
	}
}

func TestShapeEmployeeOT_Truncated(t *testing.T) {
	rows := make([]factmetric.AggRow, 0, chartdata.EmployeeOTMaxRows+1)
	for i := 0; i <= chartdata.EmployeeOTMaxRows; i++ {
		rows = append(rows, otRow(fmt.Sprintf("N%05d", i), "PRD", "PERIOD_HOURS", float64(i+1)))
	}
	_, cats, block := chartdata.ShapeEmployeeOTForTest(rows, chartdata.ViewerFilters{})
	if len(cats) != chartdata.EmployeeOTMaxRows || block["truncated"] != true {
		t.Fatalf("len=%d truncated=%v", len(cats), block["truncated"])
	}
	for _, c := range cats {
		if c == "N00000" {
			t.Fatal("lowest-ranked employee must be dropped")
		}
	}
}

func TestEmployeeOT_DrillDays(t *testing.T) {
	d := employeeOTDashboard(t)
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	p, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", DrillPath: []string{"N1"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	// ACTUAL starts at the R2 Monday (23 Nov) so the week days before the 26th are not shown as 0.
	if p.Args[0] != "N1" || p.Args[2] != isoDay("2026-11-23") || p.Args[3] != isoDay("2026-12-25") ||
		p.Args[5] != isoDay("2026-11-23") || p.Args[6] != isoDay("2026-11-29") || !strings.Contains(p.SQL, "'DAILY'") {
		t.Errorf("drill args = %v", p.Args)
	}
	if _, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", DrillPath: []string{"a", "b", "c", "d"}}, now); !errors.Is(err, factmetric.ErrDrillTooDeep) {
		t.Errorf("too deep: %v", err)
	}
	series, days := chartdata.ShapeEmployeeOTDaysForTest([]factmetric.AggRow{
		{Category: "ACTUAL", PeriodLabel: "2026-11-24", Value: 3},
		{Category: "ACTUAL", PeriodLabel: "2026-11-27", Value: 2},
		{Category: "LIMIT", PeriodLabel: "2026-11-24", Value: 3},
		{Category: "LIMIT", PeriodLabel: "2026-11-23", Value: 8},
	})
	if strings.Join(days, ",") != "2026-11-23,2026-11-24,2026-11-27" || len(series) != 2 {
		t.Fatalf("days = %v", days)
	}
	if series[0].Points[0].Value != 0 || series[1].Points[0].Value != 8 || series[1].Points[2].Value != 0 {
		t.Errorf("series = %+v", series)
	}
}

func TestShapeEmployeeOT_SummaryEmptyAndRounding(t *testing.T) {
	_, _, block := chartdata.ShapeEmployeeOTForTest(nil, chartdata.ViewerFilters{})
	sum := block["summary"].(map[string]any)
	if sum["employees"] != 0.0 || sum["total_period_hours"] != 0.0 || sum["over_limit"] != 0.0 || sum["chronic"] != 0.0 {
		t.Errorf("empty summary = %v", sum)
	}
	if subs := block["subtotals"].([]any); len(subs) != 0 {
		t.Errorf("empty subtotals = %v", subs)
	}
	rows := []factmetric.AggRow{
		otRow("N1", "PRD", "PERIOD_HOURS", 10), otRow("N1", "PRD", "EXCESS", 1),
		otRow("N2", "PRD", "PERIOD_HOURS", 10.05),
		otRow("N3", "PRD", "PERIOD_HOURS", 0.5),
	}
	_, _, block = chartdata.ShapeEmployeeOTForTest(rows, chartdata.ViewerFilters{})
	prd := block["subtotals"].([]any)[0].(map[string]any)
	if prd["avg_period_hours"] != 6.9 || prd["over_pct"] != 33.3 { // 20.55/3 = 6.85 -> 6.9, 1/3 -> 33.3
		t.Errorf("rounding = %v", prd)
	}
}

func TestShapeEmployeeOT_TruncatedSummaryCoversListedRows(t *testing.T) {
	rows := make([]factmetric.AggRow, 0, chartdata.EmployeeOTMaxRows+1)
	for i := 0; i <= chartdata.EmployeeOTMaxRows; i++ {
		rows = append(rows, otRow(fmt.Sprintf("N%05d", i), "PRD", "PERIOD_HOURS", 1))
	}
	_, _, block := chartdata.ShapeEmployeeOTForTest(rows, chartdata.ViewerFilters{})
	if got := block["summary"].(map[string]any)["employees"]; got != float64(chartdata.EmployeeOTMaxRows) {
		t.Errorf("summary employees = %v, want %d (listed rows only)", got, chartdata.EmployeeOTMaxRows)
	}
}

func TestResolveEmployeeOTWindows_Prev(t *testing.T) {
	cases := []struct{ name, preset, today, prevFrom, prevTo string }{
		{"THIS_MONTH", "THIS_MONTH", "2026-10-21", "2026-09-25", "2026-09-25"},
		{"THIS_MONTH tepi tgl 25", "THIS_MONTH", "2026-10-25", "2026-09-25", "2026-09-25"},
		{"THIS_MONTH tepi tgl 26", "THIS_MONTH", "2026-10-26", "2026-10-25", "2026-10-25"},
		{"THIS_MONTH awal tahun", "THIS_MONTH", "2026-01-10", "2025-12-25", "2025-12-25"},
		{"LAST_MONTH", "LAST_MONTH", "2026-10-21", "2026-08-25", "2026-08-25"},
		{"THIS_QTR 1 periode", "THIS_QTR", "2026-10-21", "2026-09-25", "2026-09-25"},
		{"THIS_QTR 3 periode", "THIS_QTR", "2026-12-20", "2026-07-25", "2026-09-25"},
		{"THIS_YEAR awal tahun", "THIS_YEAR", "2026-01-10", "2025-12-25", "2025-12-25"},
		{"THIS_YEAR 10 periode", "THIS_YEAR", "2026-10-21", "2025-03-25", "2025-12-25"},
		{"L12M", "L12M", "2026-10-21", "2024-11-25", "2025-10-25"},
		{"L12M tgl 26", "L12M", "2026-10-26", "2024-12-25", "2025-11-25"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, err := chartdata.ResolveEmployeeOTWindows(c.preset, isoDay(c.today))
			if err != nil {
				t.Fatal(err)
			}
			got := w.PrevFrom.Format(time.DateOnly) + " " + w.PrevTo.Format(time.DateOnly)
			if want := c.prevFrom + " " + c.prevTo; got != want {
				t.Errorf("got %s want %s", got, want)
			}
		})
	}
}

func TestEmployeeOT_PlanPrevArgs(t *testing.T) {
	p, err := chartdata.Plan(employeeOTDashboard(t), chartdata.ViewerFilters{PeriodPreset: "THIS_QTR"}, otNow)
	if err != nil {
		t.Fatal(err)
	}
	if p.Args[8] != isoDay("2026-09-25") || p.Args[9] != isoDay("2026-09-25") || !strings.Contains(p.SQL, "BETWEEN $9 AND $10") {
		t.Errorf("prev args = %v", p.Args)
	}
}

func prevTotalRows(hours, employees float64) []factmetric.AggRow {
	return []factmetric.AggRow{
		{Category: "\x1ePREV_TOTAL", PeriodLabel: "PREV_TOTAL_HOURS", Value: hours},
		{Category: "\x1ePREV_TOTAL", PeriodLabel: "PREV_TOTAL_EMPLOYEES", Value: employees},
	}
}

func TestShapeEmployeeOT_PrevDelta(t *testing.T) {
	rows := []factmetric.AggRow{
		otRow("N1", "PRD", "PERIOD_HOURS", 30), otRow("N1", "PRD", "PREV_HOURS", 20), // +50%
		otRow("N2", "PRD", "PERIOD_HOURS", 10),                                     // no previous: "-"
		otRow("N3", "PRD", "PERIOD_HOURS", 5), otRow("N3", "PRD", "PREV_HOURS", 0), // prev = 0: "-", not 0% / inf
		otRow("N4", "PRD", "PERIOD_HOURS", 8), otRow("N4", "PRD", "PREV_HOURS", 16), // -50%
	}
	rows = append(rows, prevTotalRows(100, 10)...) // previous window: 10 people, 100 h in total
	series, cats, block := chartdata.ShapeEmployeeOTForTest(rows, chartdata.ViewerFilters{})
	if len(series) != 4 || len(cats) != 4 {
		t.Fatalf("marker rows must not become employees or columns: %d series, %v", len(series), cats)
	}
	want := map[string]any{"N1": 50.0, "N2": nil, "N3": nil, "N4": -50.0}
	for i, c := range cats {
		if got := series[0].Points[i].Meta["delta_pct"]; got != want[c] {
			t.Errorf("%s delta = %v, want %v", c, got, want[c])
		}
	}
	prev := block["summary"].(map[string]any)["prev"].(map[string]any)
	// Listed now: 53 h, 4 people. Previous total: 100 h, 10 people, so the drop shows (-47%, -60%).
	if prev["total_period_hours"] != 100.0 || prev["hours_delta_pct"] != -47.0 || prev["employees"] != 10.0 || prev["employees_delta_pct"] != -60.0 {
		t.Errorf("prev = %v", prev)
	}
}

func TestShapeEmployeeOT_PrevEmptyIsNil(t *testing.T) {
	_, _, block := chartdata.ShapeEmployeeOTForTest([]factmetric.AggRow{otRow("N1", "PRD", "PERIOD_HOURS", 5)}, chartdata.ViewerFilters{})
	if p := block["summary"].(map[string]any)["prev"]; p != nil {
		t.Errorf("prev = %v, want nil", p)
	}
	// Marker rows with zero employees (previous window empty) are also "nothing to compare".
	rows := append([]factmetric.AggRow{otRow("N1", "PRD", "PERIOD_HOURS", 5)}, prevTotalRows(0, 0)...)
	_, _, block = chartdata.ShapeEmployeeOTForTest(rows, chartdata.ViewerFilters{})
	if p := block["summary"].(map[string]any)["prev"]; p != nil {
		t.Errorf("empty previous window: prev = %v, want nil", p)
	}
}

func TestEmployeeOT_PrevTotalRespectsGroupFilter(t *testing.T) {
	p, err := chartdata.Plan(employeeOTDashboard(t), chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", GroupBy: "DEPT", GroupFilter: []string{"QA"}}, otNow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(p.SQL, "COALESCE(e.dept_name, '') IN ($13)") != 2 || !strings.Contains(p.SQL, "PREV_TOTAL_EMPLOYEES") {
		t.Errorf("group filter must apply to the listed rows and the previous total:\n%s", p.SQL)
	}
}

func weekKeys(t *testing.T, preset, today string) []string {
	t.Helper()
	w, err := chartdata.ResolveEmployeeOTWindows(preset, isoDay(today))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(w.Weeks))
	for _, m := range w.Weeks {
		out = append(out, m.Format(time.DateOnly))
	}
	return out
}

// HD-25: weeks of the period belong to the period of their Sunday (K10).
func TestEmployeeOT_WeeksOfPeriod(t *testing.T) {
	// Period 26 Sep – 25 Oct 2026 has 5 Sundays (27 Sep, 4, 11, 18, 25 Oct); the first week starts Mon 21 Sep.
	got := strings.Join(weekKeys(t, "LAST_MONTH", "2026-11-10"), ",")
	if want := "2026-09-21,2026-09-28,2026-10-05,2026-10-12,2026-10-19"; got != want {
		t.Errorf("5-week period:\n got %s\nwant %s", got, want)
	}
	// Period 26 Oct – 25 Nov 2026 has 4 Sundays (1, 8, 15, 22 Nov).
	got = strings.Join(weekKeys(t, "THIS_MONTH", "2026-11-25"), ",")
	if want := "2026-10-26,2026-11-02,2026-11-09,2026-11-16"; got != want {
		t.Errorf("4-week period:\n got %s\nwant %s", got, want)
	}
	// The running period only lists weeks that already started (partial last week allowed, HD-17).
	got = strings.Join(weekKeys(t, "THIS_MONTH", "2026-10-21"), ",")
	if want := "2026-09-21,2026-09-28,2026-10-05,2026-10-12,2026-10-19"; got != want {
		t.Errorf("running period: got %s", got)
	}
	if got := weekKeys(t, "THIS_MONTH", "2026-10-07"); len(got) != 3 || got[2] != "2026-10-05" {
		t.Errorf("early in period: %v", got)
	}
}

func TestEmployeeOT_PlanWeekWindowAndRanges(t *testing.T) {
	p, err := chartdata.Plan(employeeOTDashboard(t), chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH"}, otNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Args) != 12 || p.Args[10] != isoDay("2026-09-21") || p.Args[11] != isoDay("2026-09-21") || p.Args[6] != isoDay("2026-10-19") {
		t.Errorf("week window args = %v", p.Args)
	}
	if !strings.Contains(p.SQL, "BETWEEN $11 AND $7") {
		t.Error("week facts must span the whole R1 window")
	}
}

func TestEmployeeOT_WeeksBlockInResponse(t *testing.T) {
	dash := employeeOTDashboard(t)
	repo := &mockDashboardRepo{getByCode: func(context.Context, string) (*dashboarddomain.Dashboard, error) { return dash, nil }}
	h := chartdata.NewGetDataHandler(repo, &mockFactRepo{}, newMockCache(), func(any) string { return "h" }).WithNow(func() time.Time { return otNow })
	res, err := h.Handle(context.Background(), chartdata.GetDataQuery{
		DashboardCode: "HR_OT_EMPLOYEES", CanViewWorkforce: true, CanViewEmployee: true,
		Filters: chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH"},
	})
	if err != nil {
		t.Fatal(err)
	}
	weeks := res.Config["employee_ot"].(map[string]any)["weeks"].([]any)
	if len(weeks) != 5 {
		t.Fatalf("weeks = %v", weeks)
	}
	first, last := weeks[0].(map[string]any), weeks[4].(map[string]any)
	if first["key"] != "W:2026-09-21" || first["label"] != "W1" || first["from"] != "2026-09-21" || first["to"] != "2026-09-27" || first["partial"] != false {
		t.Errorf("first = %v", first)
	}
	if last["label"] != "W5" || last["partial"] != true || last["to"] != "2026-10-25" {
		t.Errorf("last = %v", last)
	}
}

// 14 h exactly is compliant: only EXCESS > 0 counts as a week over (HD-16, HD-25); 14.5 is over.
func TestShapeEmployeeOT_WeekCellsAndOver(t *testing.T) {
	rows := []factmetric.AggRow{
		otRow("N1", "PRD", "PERIOD_HOURS", 30), otRow("N1", "PRD", "W:2026-10-12", 14), otRow("N1", "PRD", "W:2026-10-19", 14),
		otRow("N2", "PRD", "PERIOD_HOURS", 31), otRow("N2", "PRD", "W:2026-10-12", 14.5), otRow("N2", "PRD", "W:2026-10-19", 16),
		otRow("N2", "PRD", "EXCESS", 2), otRow("N2", "PRD", "WEEKS_OVER", 2),
	}
	series, _, block := chartdata.ShapeEmployeeOTForTest(rows, chartdata.ViewerFilters{})
	names := []string{}
	for _, s := range series {
		names = append(names, s.Name)
	}
	if got := strings.Join(names, ","); got != "PERIOD_HOURS,EXCESS,WEEKS_OVER,CHRONIC,W:2026-10-12,W:2026-10-19" {
		t.Fatalf("series = %s", got)
	}
	sum := block["summary"].(map[string]any)
	if sum["over_limit"] != 1.0 {
		t.Errorf("only N2 is over: %v", sum)
	}
	if series[2].Points[0].Value != 2 || series[2].Points[1].Value != 0 {
		t.Errorf("weeks over = %+v", series[2].Points)
	}
}

// Over weeks without a column ride on the row meta (HD-26, H053); they never become series.
func TestShapeEmployeeOT_HiddenOverInMeta(t *testing.T) {
	rows := []factmetric.AggRow{
		otRow("N1", "PRD", "PERIOD_HOURS", 40), otRow("N1", "PRD", "W:2026-10-19", 10),
		otRow("N1", "PRD", "HO:2026-07-20", 24.6), otRow("N1", "PRD", "HO:2026-06-01", 15),
		otRow("N1", "PRD", "EXCESS", 10.6),
	}
	series, _, _ := chartdata.ShapeEmployeeOTForTest(rows, chartdata.ViewerFilters{})
	for _, s := range series {
		if strings.HasPrefix(s.Name, "HO:") {
			t.Fatalf("hidden over became a series: %s", s.Name)
		}
	}
	m := series[0].Points[0].Meta
	ho := m["hidden_over"].([]any)
	if len(ho) != 2 || m["hidden_over_count"] != 2 {
		t.Fatalf("meta = %v", m)
	}
	if first := ho[0].(map[string]any); first["from"] != "2026-07-20" || first["hours"] != 24.6 {
		t.Errorf("newest first: %v", ho)
	}
}

func TestEmployeeOT_ComparePeriod(t *testing.T) {
	d := employeeOTDashboard(t)
	t.Run("default tak berubah", func(t *testing.T) {
		p, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH"}, otNow)
		if err != nil {
			t.Fatal(err)
		}
		if p.Args[8] != isoDay("2026-09-25") || p.Args[9] != isoDay("2026-09-25") {
			t.Errorf("prev args = %v", p.Args[8:10])
		}
	})
	t.Run("periode terpilih", func(t *testing.T) {
		p, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", ComparePeriod: "202607"}, otNow)
		if err != nil {
			t.Fatal(err)
		}
		if p.Args[8] != isoDay("2026-07-25") || p.Args[9] != isoDay("2026-07-25") {
			t.Errorf("prev args = %v", p.Args[8:10])
		}
	})
	t.Run("jendela sama panjang", func(t *testing.T) {
		p, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_QTR", ComparePeriod: "202606"}, isoDay("2026-12-20"))
		if err != nil {
			t.Fatal(err)
		}
		if p.Args[8] != isoDay("2026-04-25") || p.Args[9] != isoDay("2026-06-25") {
			t.Errorf("prev args = %v", p.Args[8:10])
		}
	})
	for _, c := range []struct{ name, cmp string }{
		{"format salah", "2026-07"}, {"bulan tak ada", "202613"},
		{"jendela sekarang", "202610"}, {"masa depan", "202701"},
	} {
		t.Run("tak valid "+c.name, func(t *testing.T) {
			_, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH", ComparePeriod: c.cmp}, otNow)
			if !errors.Is(err, factmetric.ErrInvalidPlan) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

// HD-26: a long preset caps the week columns, while EXCESS / WEEKS_OVER keep the whole window.
func TestEmployeeOT_PlanCapsWeekColumns(t *testing.T) {
	w, err := chartdata.ResolveEmployeeOTWindows("L12M", isoDay("2026-10-21"))
	if err != nil {
		t.Fatal(err)
	}
	cw := w.ColumnWeeks()
	if len(w.Weeks) <= 13 || len(cw) != 13 || cw[12] != w.Week {
		t.Fatalf("weeks=%d cols=%d", len(w.Weeks), len(cw))
	}
	p, err := chartdata.Plan(employeeOTDashboard(t), chartdata.ViewerFilters{PeriodPreset: "L12M"}, otNow)
	if err != nil {
		t.Fatal(err)
	}
	if p.Args[10] != w.Weeks[0] || p.Args[11] != cw[0] {
		t.Errorf("window start %v, column start %v", p.Args[10], p.Args[11])
	}
	if !strings.Contains(p.SQL, "BETWEEN $12 AND $7") || !strings.Contains(p.SQL, "BETWEEN $11 AND $7") {
		t.Error("week columns use $12, EXCESS/WEEKS_OVER use $11")
	}
}
