package chartdata_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	chartdata "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

func newPivotDashboard(t *testing.T, chartType, grain, filterGroup1 string, cfg map[string]any) *dashboarddomain.Dashboard {
	t.Helper()
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code: "HR_PIVOT", Title: "Pivot", FilterType: "HR_OT_HOURS", FilterGroup1: filterGroup1,
		PeriodGrain: grain, DefaultPeriod: "L12M", ChartType: chartType, ChartConfigRaw: cfg,
		MaxDrillLevel: 3, CacheTTLSec: 60, GroupID: uuid.New(), IsActive: true, CreatedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("construct dashboard: %v", err)
	}
	return d
}

func stackedCfg() map[string]any {
	return map[string]any{"x_axis_field": "group_1", "y_axis_field": "value", "stack_by_field": "group_2"}
}

func heatmapCfg() map[string]any {
	return map[string]any{"x_axis_field": "group_1", "y_axis_field": "period", "value_field": "value", "filter_group_2": "OVER_LIMIT"}
}

func TestIsPivot_Conditions(t *testing.T) {
	tests := []struct {
		name      string
		chartType string
		cfg       map[string]any
		group1    string
		f         chartdata.ViewerFilters
		want      bool
	}{
		{"stacked top level", "stacked_bar", stackedCfg(), "", chartdata.ViewerFilters{}, true},
		{"heatmap top level", "heatmap", heatmapCfg(), "", chartdata.ViewerFilters{}, true},
		{"drilled keeps drill path", "stacked_bar", stackedCfg(), "", chartdata.ViewerFilters{DrillPath: []string{"PRD"}}, false},
		{"force trend", "stacked_bar", stackedCfg(), "", chartdata.ViewerFilters{ForceTrend: true}, false},
		{"pinned group_1", "stacked_bar", stackedCfg(), "PRD", chartdata.ViewerFilters{}, false},
		{"bar is 1D", "bar", map[string]any{"x_axis_field": "group_1", "y_axis_field": "value"}, "", chartdata.ViewerFilters{}, false},
		{"group_3 never pivots (HD-07)", "stacked_bar",
			map[string]any{"x_axis_field": "group_1", "y_axis_field": "value", "stack_by_field": "group_3"}, "", chartdata.ViewerFilters{}, false},
		{"same axis", "stacked_bar",
			map[string]any{"x_axis_field": "group_2", "y_axis_field": "value", "stack_by_field": "group_2"}, "", chartdata.ViewerFilters{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newPivotDashboard(t, tc.chartType, "MONTHLY", tc.group1, tc.cfg)
			if got := chartdata.IsPivot(d, tc.f); got != tc.want {
				t.Errorf("IsPivot = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlan_Pivot_StackedBar(t *testing.T) {
	d := newPivotDashboard(t, "stacked_bar", "MONTHLY", "", stackedCfg())
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	plan, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "L12M", Group1Filter: []string{"PRD"}}, now)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.TargetTable != "mv_bi_metric_g2" {
		t.Errorf("source = %q", plan.TargetTable)
	}
	for _, want := range []string{"COALESCE(group_2, '')::text AS category", "COALESCE(group_1, '')::text AS periode_label", "group_1 IN ("} {
		if !strings.Contains(plan.SQL, want) {
			t.Errorf("SQL missing %q:\n%s", want, plan.SQL)
		}
	}
	if strings.Contains(plan.SQL, "group_3") {
		t.Errorf("pivot must not touch group_3:\n%s", plan.SQL)
	}
}

func TestPlan_Pivot_HeatmapWeeklyPinsGroup2(t *testing.T) {
	d := newPivotDashboard(t, "heatmap", "WEEKLY", "", heatmapCfg())
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	plan, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "L12M"}, now)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !strings.Contains(plan.SQL, `TO_CHAR(periode_date, 'IYYY-"W"IW')::text AS category`) {
		t.Errorf("week label missing:\n%s", plan.SQL)
	}
	found := false
	for _, a := range plan.Args {
		if a == "OVER_LIMIT" {
			found = true
		}
	}
	if !strings.Contains(plan.SQL, "group_2 = $") || !found {
		t.Errorf("filter_group_2 not applied: %s %v", plan.SQL, plan.Args)
	}
}

func TestShape_Pivot_FillsMissingCellsSorted(t *testing.T) {
	d := newPivotDashboard(t, "stacked_bar", "MONTHLY", "", stackedCfg())
	rows := []factmetric.AggRow{
		{Category: "Off", PeriodLabel: "QA", Value: 2},
		{Category: "Normal", PeriodLabel: "PRD", Value: 5},
		{Category: "Normal", PeriodLabel: "QA", Value: 1},
	}
	res := chartdata.Shape(d, rows, nil, chartdata.ViewerFilters{}, chartdata.DrillContext{})
	if got := strings.Join(res.Categories, ","); got != "PRD,QA" {
		t.Fatalf("categories = %s", got)
	}
	if len(res.Series) != 2 || res.Series[0].Name != "Normal" || res.Series[1].Name != "Off" {
		t.Fatalf("series = %+v", res.Series)
	}
	off := res.Series[1].Points
	if off[0].Category != "PRD" || off[0].Value != 0 || off[1].Value != 2 {
		t.Errorf("Off points = %+v", off)
	}
}

func TestShape_Pivot_HourBucketsSortNumerically(t *testing.T) {
	d := newPivotDashboard(t, "stacked_bar", "WEEKLY", "", map[string]any{"x_axis_field": "group_2", "y_axis_field": "value", "stack_by_field": "group_1"})
	rows := []factmetric.AggRow{
		{Category: "QA", PeriodLabel: ">18", Value: 1},
		{Category: "QA", PeriodLabel: "14-18", Value: 1},
		{Category: "QA", PeriodLabel: "7-14", Value: 1},
		{Category: "QA", PeriodLabel: "0-7", Value: 1},
	}
	res := chartdata.Shape(d, rows, nil, chartdata.ViewerFilters{}, chartdata.DrillContext{})
	if got := strings.Join(res.Categories, ","); got != "0-7,7-14,14-18,>18" {
		t.Errorf("categories = %s", got)
	}
}

func TestShape_Pivot_WeeklyZeroFillsWindowWeeks(t *testing.T) {
	d := newPivotDashboard(t, "heatmap", "WEEKLY", "", map[string]any{
		"x_axis_field": "period", "y_axis_field": "group_1", "value_field": "value", "filter_group_2": "OVER_LIMIT",
	})
	f := chartdata.ViewerFilters{
		PeriodPreset: "CUSTOM",
		PeriodFrom:   time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC),
		PeriodTo:     time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC),
	}
	rows := []factmetric.AggRow{{Category: "QA", PeriodLabel: "2026-W41", Value: 2}}
	res := chartdata.Shape(d, rows, nil, f, chartdata.DrillContext{})
	if got := strings.Join(res.Categories, ","); got != "2026-W40,2026-W41,2026-W42,2026-W43" {
		t.Fatalf("categories = %s", got)
	}
	pts := res.Series[0].Points
	if pts[0].Value != 0 || pts[1].Value != 2 || pts[3].Value != 0 {
		t.Errorf("points = %+v", pts)
	}
}
