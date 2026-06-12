package dashboard_test

import (
	"testing"

	"github.com/google/uuid"

	dashboard "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
)

func TestViewConfigFor_UsesConfiguredValues(t *testing.T) {
	d, err := dashboard.NewDashboard(dashboard.NewDashboardParams{
		Code:          "TEST",
		Title:         "Test Dashboard",
		FilterType:    "MIS",
		FilterGroup1:  "EBITDA",
		PeriodGrain:   "MONTHLY",
		DefaultPeriod: "L12M",
		ChartType:     "waterfall",
		ChartConfigRaw: map[string]any{
			"x_axis_field": "group_2",
			"y_axis_field": "display_value",
			"view_configs": map[string]any{
				"waterfall": map[string]any{
					"title_template": "EBITDA Breakdown — {period}",
					"drill_enabled":  true,
					"hint":           "Click to drill",
				},
				"line": map[string]any{
					"title_template": "EBITDA Trend Over Time",
					"drill_enabled":  false,
					"hint":           "",
				},
			},
		},
		MaxDrillLevel: 3,
		CacheTTLSec:   60,
		GroupID:       uuid.New(),
		IsActive:      true,
		CreatedBy:     uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}

	wf := d.ViewConfigFor("waterfall")
	if wf.TitleTemplate != "EBITDA Breakdown — {period}" {
		t.Errorf("waterfall title: got %q", wf.TitleTemplate)
	}
	if !wf.DrillEnabled {
		t.Error("waterfall should be drillable")
	}
	if wf.Hint != "Click to drill" {
		t.Errorf("waterfall hint: got %q", wf.Hint)
	}

	ln := d.ViewConfigFor("line")
	if ln.DrillEnabled {
		t.Error("line should not be drillable (configured false)")
	}
	if ln.TitleTemplate != "EBITDA Trend Over Time" {
		t.Errorf("line title: got %q", ln.TitleTemplate)
	}

	// Fallback for a chart type NOT in view_configs.
	bar := d.ViewConfigFor("bar")
	if !bar.DrillEnabled {
		t.Error("bar should default to drillable")
	}
	if bar.TitleTemplate != "Test Dashboard" {
		t.Errorf("bar fallback title: got %q", bar.TitleTemplate)
	}

	scatter := d.ViewConfigFor("scatter")
	if scatter.DrillEnabled {
		t.Error("scatter should default to non-drillable")
	}

	heatmap := d.ViewConfigFor("heatmap")
	if heatmap.DrillEnabled {
		t.Error("heatmap should default to non-drillable")
	}

	treemap := d.ViewConfigFor("treemap")
	if !treemap.DrillEnabled {
		t.Error("treemap should default to drillable")
	}
}
