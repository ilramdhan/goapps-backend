package chartdata_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	chartdata "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
)

// newDashboard builds a valid Dashboard for planner tests.
func newDashboard(t *testing.T, chartType, filterGroup1 string, maxDrill int) *dashboarddomain.Dashboard {
	t.Helper()
	cfgRaw := map[string]any{"x_axis_field": "group_2", "y_axis_field": "display_value"}
	if chartType == "line" || chartType == "mixed" {
		cfgRaw = map[string]any{"x_axis_field": "period", "y_axis_field": "display_value"}
		if chartType == "mixed" {
			cfgRaw["series_defs"] = []any{map[string]any{"name": "v", "type": "bar", "field": "value"}}
		}
	}
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code:           "TEST_DASH",
		Title:          "Test",
		FilterType:     "MIS",
		FilterGroup1:   filterGroup1,
		PeriodGrain:    "MONTHLY",
		DefaultPeriod:  "L12M",
		ChartType:      chartType,
		ChartConfigRaw: cfgRaw,
		CompareModes:   []string{"MoM", "YoY"},
		MaxDrillLevel:  maxDrill,
		CacheTTLSec:    60,
		GroupID:        uuid.New(),
		IsActive:       true,
		CreatedBy:      uuid.New(),
	})
	if err != nil {
		t.Fatalf("construct dashboard: %v", err)
	}
	return d
}

func TestPlan_DrillLevels_SelectCorrectSource(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		group1     string
		drill      []string
		wantSource string
	}{
		// No pre-filter: depth 0 → g1 MV, 1 → g2 MV, 2 → raw fact.
		{"depth0 no-prefilter → g1", "", nil, "mv_bi_metric_g1"},
		{"depth1 no-prefilter → g2", "", []string{"EBITDA"}, "mv_bi_metric_g2"},
		{"depth2 no-prefilter → fact", "", []string{"EBITDA", "INCOME"}, "bi_fact_metric"},
		// Pre-filtered (filter_group_1 set): depth0 already shows g2 breakdown.
		{"depth0 prefiltered → g2", "EBITDA", nil, "mv_bi_metric_g2"},
		{"depth1 prefiltered → fact", "EBITDA", []string{"INCOME"}, "bi_fact_metric"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newDashboard(t, "waterfall", tc.group1, 3)
			plan, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "L12M", DrillPath: tc.drill}, now)
			if err != nil {
				t.Fatalf("Plan error: %v", err)
			}
			if plan.TargetTable != tc.wantSource {
				t.Errorf("source: want %q, got %q", tc.wantSource, plan.TargetTable)
			}
		})
	}
}

// HD-07: HR employee level (group_3 = NIK) is labelled "NIK — Nama" via bi_hr_employee; other types keep group_3.
func TestPlan_EmployeeLevel_LabelsNIKWithName(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	mk := func(filterType string) *dashboarddomain.Dashboard {
		d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
			Code: "TEST_EMP", Title: "Test", FilterType: filterType, PeriodGrain: "MONTHLY", DefaultPeriod: "L12M",
			ChartType: "waterfall", ChartConfigRaw: map[string]any{"x_axis_field": "group_2", "y_axis_field": "display_value"},
			MaxDrillLevel: 3, CacheTTLSec: 60, GroupID: uuid.New(), IsActive: true, CreatedBy: uuid.New(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	f := chartdata.ViewerFilters{PeriodPreset: "L12M", DrillPath: []string{"Spinning", "Normal"}}

	hr, err := chartdata.Plan(mk("HR_OT_HOURS"), f, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hr.SQL, "bi_hr_employee") || !strings.Contains(hr.SQL, "emp_name") {
		t.Errorf("HR level 3 must label NIK with name, got SQL:\n%s", hr.SQL)
	}
	other, err := chartdata.Plan(mk("MIS"), f, now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(other.SQL, "bi_hr_employee") {
		t.Errorf("non-HR level 3 must not join employees, got SQL:\n%s", other.SQL)
	}
}

func TestPlan_DrillTooDeep(t *testing.T) {
	now := time.Now().UTC()
	d := newDashboard(t, "waterfall", "", 1)
	_, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "L12M", DrillPath: []string{"A", "B"}}, now)
	if err == nil {
		t.Fatal("expected ErrDrillTooDeep for depth 2 > max 1")
	}
}

func TestPlan_TrendCompare_EmitsSelfJoin(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	d := newDashboard(t, "line", "", 1)
	plan, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "L12M", Compare: "YoY"}, now)
	if err != nil {
		t.Fatal(err)
	}
	// YoY → 12-month shift self-join must appear, and prev_value must be populated from prev CTE.
	if !strings.Contains(plan.SQL, "INTERVAL '12 months'") {
		t.Errorf("expected 12-month interval in compare SQL, got:\n%s", plan.SQL)
	}
	if !strings.Contains(plan.SQL, "WITH cur AS") || !strings.Contains(plan.SQL, "prev AS") {
		t.Errorf("expected cur/prev CTEs, got:\n%s", plan.SQL)
	}
	if strings.Contains(plan.SQL, "0::numeric AS prev_value") {
		t.Errorf("compare query must NOT hardcode prev_value=0, got:\n%s", plan.SQL)
	}
}

func TestPlan_TrendCompare_MoMShift(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	d := newDashboard(t, "line", "", 1)
	plan, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "L12M", Compare: "MoM"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "INTERVAL '1 months'") {
		t.Errorf("MoM expected 1-month shift, got:\n%s", plan.SQL)
	}
}

func TestPlan_TrendNoCompare_HardcodesPrevZero(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	d := newDashboard(t, "line", "", 1)
	plan, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "L12M", Compare: "NONE"}, now)
	if err != nil {
		t.Fatal(err)
	}
	// "NONE" is not a recognised shift → plain trend query with prev_value=0.
	if !strings.Contains(plan.SQL, "0::numeric AS prev_value") {
		t.Errorf("no-compare trend should hardcode prev_value=0, got:\n%s", plan.SQL)
	}
	if strings.Contains(plan.SQL, "WITH cur AS") {
		t.Errorf("no-compare trend must not emit CTE self-join, got:\n%s", plan.SQL)
	}
}

func TestPlan_Categorical_NoCompareOverlay(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	// Waterfall is categorical (x=group_2); compare overlay should NOT apply even if requested.
	d := newDashboard(t, "waterfall", "EBITDA", 3)
	plan, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "L12M", Compare: "YoY"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan.SQL, "WITH cur AS") {
		t.Errorf("categorical chart must not emit compare CTE, got:\n%s", plan.SQL)
	}
	// Args[0] must be the type filter.
	if len(plan.Args) == 0 || plan.Args[0] != "MIS" {
		t.Errorf("expected first arg MIS, got %v", plan.Args)
	}
}

func TestPlan_ParameterizedArgsOrder(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	d := newDashboard(t, "waterfall", "EBITDA", 3)
	plan, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "L12M"}, now)
	if err != nil {
		t.Fatal(err)
	}
	// args: type, group_1, grain, scenario, from, to
	if len(plan.Args) != 6 {
		t.Fatalf("expected 6 args, got %d: %v", len(plan.Args), plan.Args)
	}
	if plan.Args[0] != "MIS" || plan.Args[1] != "EBITDA" || plan.Args[2] != "MONTHLY" || plan.Args[3] != "ACTUAL" {
		t.Errorf("unexpected arg order: %v", plan.Args)
	}
}

// PR #236 review: the trend label stays YYYYMM for every grain except WEEKLY (IYYY-"W"IW); the
// viewer reads 6-digit labels as periods, so DAILY/QUARTERLY/YEARLY dashboards must not change.
func TestPlan_TrendLabel_YYYYMMExceptWeekly(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	cfg := map[string]any{"x_axis_field": "period", "y_axis_field": "value"}
	for grain, want := range map[string]string{
		"DAILY":     "TO_CHAR(periode_date, 'YYYYMM')::text AS periode_label",
		"MONTHLY":   "TO_CHAR(periode_date, 'YYYYMM')::text AS periode_label",
		"QUARTERLY": "TO_CHAR(periode_date, 'YYYYMM')::text AS periode_label",
		"YEARLY":    "TO_CHAR(periode_date, 'YYYYMM')::text AS periode_label",
		"WEEKLY":    `TO_CHAR(periode_date, 'IYYY-"W"IW')::text AS periode_label`,
	} {
		t.Run(grain, func(t *testing.T) {
			plan, err := chartdata.Plan(newPivotDashboard(t, "line", grain, "", cfg), chartdata.ViewerFilters{PeriodPreset: "L12M"}, now)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(plan.SQL, want) {
				t.Errorf("want %q in SQL:\n%s", want, plan.SQL)
			}
		})
	}
}

// PR #236 review: WEEKLY periode_date is a Monday, so the compare overlay shifts by whole weeks
// (7/91/364 days, as compareRangeWeekly), never by months (a Monday minus N months is rarely a Monday).
func TestPlan_TrendCompare_WeeklyShiftsWeeks(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	cfg := map[string]any{"x_axis_field": "period", "y_axis_field": "value"}
	d := newPivotDashboard(t, "line", "WEEKLY", "", cfg)
	for mode, want := range map[string]string{"MoM": "INTERVAL '7 days'", "QoQ": "INTERVAL '91 days'", "YoY": "INTERVAL '364 days'"} {
		t.Run(mode, func(t *testing.T) {
			plan, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "L12M", Compare: mode}, now)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(plan.SQL, want) || strings.Contains(plan.SQL, "months'") {
				t.Errorf("%s: want %q and no month shift, got:\n%s", mode, want, plan.SQL)
			}
		})
	}
}
