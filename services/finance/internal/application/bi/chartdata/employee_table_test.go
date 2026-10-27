package chartdata_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	chartdata "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

func employeeTableDashboardCfg() map[string]any {
	return map[string]any{"x_axis_field": "group_1", "y_axis_field": "value", "table_mode": "employee_week"}
}

func TestEmployeeTable_GatedAsEmployeeLevel(t *testing.T) {
	d := newPivotDashboard(t, "data_table", "WEEKLY", "", employeeTableDashboardCfg())
	if !chartdata.RequestsEmployeeLevel(d, chartdata.ViewerFilters{}) {
		t.Fatal("employee table at top level must require workforceemployee.view")
	}
}

func TestEmployeeTable_PlanReadsFactTable(t *testing.T) {
	d := newPivotDashboard(t, "data_table", "WEEKLY", "", employeeTableDashboardCfg())
	now := time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC)
	p, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "LAST_WEEK"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.TargetTable != "bi_fact_metric" || len(p.Args) != 5 || !strings.Contains(p.SQL, "bi_hr_employee") {
		t.Errorf("unexpected plan: %s %v", p.TargetTable, p.Args)
	}
	dp, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "LAST_WEEK", DrillPath: []string{"N1"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if dp.Args[3] != "N1" || !strings.Contains(dp.SQL, "'DAILY'") {
		t.Errorf("drill plan wrong: %v", dp.Args)
	}
}

func TestShapeEmployeeTable_OrdersByHours(t *testing.T) {
	sep := "\x1f"
	rows := []factmetric.AggRow{
		{Category: "N1" + sep + "PRD" + sep + "Ani", PeriodLabel: "HOURS", Value: 15},
		{Category: "N1" + sep + "PRD" + sep + "Ani", PeriodLabel: "EXCESS", Value: 1},
		{Category: "N1" + sep + "PRD" + sep + "Ani", PeriodLabel: "CHRONIC", Value: 3},
		{Category: "N2" + sep + "QA" + sep + "Budi", PeriodLabel: "HOURS", Value: 18},
		{Category: "N2" + sep + "QA" + sep + "Budi", PeriodLabel: "EXCESS", Value: 4},
	}
	series, cats := chartdata.ShapeEmployeeTableForTest(rows)
	if len(cats) != 2 || cats[0] != "N2" {
		t.Fatalf("cats = %v", cats)
	}
	if len(series) != 3 || series[2].Points[1].Value != 3 || series[0].Points[0].Meta["name"] != "Budi" {
		t.Errorf("series = %+v", series)
	}
}

func TestEmployeeTable_RejectsTooDeepDrill(t *testing.T) {
	d := newPivotDashboard(t, "data_table", "WEEKLY", "", employeeTableDashboardCfg())
	now := time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC)
	_, err := chartdata.Plan(d, chartdata.ViewerFilters{PeriodPreset: "LAST_WEEK", DrillPath: []string{"a", "b", "c", "d"}}, now)
	if !errors.Is(err, factmetric.ErrDrillTooDeep) {
		t.Fatalf("want ErrDrillTooDeep, got %v", err)
	}
}
