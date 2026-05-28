package chartdata_test

import (
	"testing"

	"github.com/google/uuid"

	chartdata "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
)

// drillDashboard builds a waterfall dashboard for drill-context tests.
func drillDashboard(t *testing.T, filterGroup1 string, maxDrill int) *dashboarddomain.Dashboard {
	t.Helper()
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code:           "DRILL",
		Title:          "Drill",
		FilterType:     "MIS",
		FilterGroup1:   filterGroup1,
		PeriodGrain:    "MONTHLY",
		DefaultPeriod:  "L12M",
		ChartType:      "waterfall",
		ChartConfigRaw: map[string]any{"x_axis_field": "group_2", "y_axis_field": "display_value"},
		DrillEnabled:   true,
		MaxDrillLevel:  maxDrill,
		CacheTTLSec:    60,
		GroupID:        uuid.New(),
		IsActive:       true,
		CreatedBy:      uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// The fact hierarchy is fixed at 3 group levels. A dashboard that pre-filters group_1 (e.g.
// EBITDA) only exposes 2 of them to the viewer (group_2→group_3), so the deepest group_3 view
// must NOT be drillable — even when max_drill_level is configured to 3.
func TestBuildDrillContext_PreFilteredGroup1_StopsAtGroup3(t *testing.T) {
	d := drillDashboard(t, "EBITDA", 3)

	// Depth 0: showing group_2 → drillable into group_3.
	top := chartdata.BuildDrillContext(d, chartdata.ViewerFilters{})
	if !top.CanDrill || top.NextField != "group_3" {
		t.Errorf("group_2 view should drill into group_3, got canDrill=%v next=%q", top.CanDrill, top.NextField)
	}

	// Depth 1: showing group_3 (the deepest level) → NOT drillable, no next field.
	deep := chartdata.BuildDrillContext(d, chartdata.ViewerFilters{DrillPath: []string{"PRODUCTION COST"}})
	if deep.CanDrill {
		t.Errorf("group_3 view must not be drillable when group_1 is pre-filtered, got canDrill=true")
	}
	if deep.NextField != "" {
		t.Errorf("group_3 view should have no next field, got %q", deep.NextField)
	}
}

// Without a group_1 pre-filter the viewer drills group_1→group_2→group_3 (2 transitions); the
// group_3 view (depth 2) is the deepest and must not be drillable.
func TestBuildDrillContext_NoPreFilter_StopsAtGroup3(t *testing.T) {
	d := drillDashboard(t, "", 3)

	cases := []struct {
		name      string
		path      []string
		wantDrill bool
		wantNext  string
	}{
		{"group_1 view", nil, true, "group_2"},
		{"group_2 view", []string{"EBITDA"}, true, "group_3"},
		{"group_3 view", []string{"EBITDA", "PRODUCTION COST"}, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := chartdata.BuildDrillContext(d, chartdata.ViewerFilters{DrillPath: tc.path})
			if got.CanDrill != tc.wantDrill || got.NextField != tc.wantNext {
				t.Errorf("canDrill=%v next=%q, want %v / %q", got.CanDrill, got.NextField, tc.wantDrill, tc.wantNext)
			}
		})
	}
}
