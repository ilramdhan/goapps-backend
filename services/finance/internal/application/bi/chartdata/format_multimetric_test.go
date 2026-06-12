package chartdata_test

import (
	"testing"

	"github.com/google/uuid"

	chartdata "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

func buildDeliveryDashboard(t *testing.T) *dashboarddomain.Dashboard {
	t.Helper()
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code: "DELIVERY", Title: "Delivery", FilterType: "SALES", FilterGroup1: "",
		PeriodGrain: "MONTHLY", DefaultPeriod: "L12M", ChartType: "line",
		ChartConfigRaw: map[string]any{
			"x_axis_field": "period",
			"y_axis_field": "display_value",
			"metric_filter": map[string]any{
				"include_metrics": []any{"GROSS_SALES", "MARGIN"},
			},
		},
		MaxDrillLevel: 2, CacheTTLSec: 60,
		GroupID: uuid.New(), IsActive: true, DrillEnabled: true, CreatedBy: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestShape_MultiMetric_ProducesOneSeriesPerMetric(t *testing.T) {
	d := buildDeliveryDashboard(t)
	// Rows from planMultiMetric: Category=metric_name, PeriodLabel=YYYYMM
	rows := []factmetric.AggRow{
		{Category: "GROSS_SALES", PeriodLabel: "202604", Value: 1000},
		{Category: "GROSS_SALES", PeriodLabel: "202605", Value: 1100},
		{Category: "MARGIN", PeriodLabel: "202604", Value: 200},
		{Category: "MARGIN", PeriodLabel: "202605", Value: 220},
	}
	result := chartdata.Shape(d, rows, nil, chartdata.ViewerFilters{}, chartdata.DrillContext{})

	if len(result.Series) != 2 {
		t.Fatalf("want 2 series (GROSS_SALES + MARGIN), got %d: %v",
			len(result.Series), multiMetricSeriesNames(result.Series))
	}
	if result.Series[0].Name != "Gross Sales" && result.Series[0].Name != "GROSS_SALES" {
		t.Errorf("first series name: want 'Gross Sales' or 'GROSS_SALES', got %q", result.Series[0].Name)
	}
	for _, s := range result.Series {
		if len(s.Points) != 2 {
			t.Errorf("series %q: want 2 points, got %d", s.Name, len(s.Points))
		}
	}
}

func TestShape_MultiMetric_CategoriesFromPeriodLabels(t *testing.T) {
	d := buildDeliveryDashboard(t)
	rows := []factmetric.AggRow{
		{Category: "GROSS_SALES", PeriodLabel: "202604", Value: 1000},
		{Category: "MARGIN", PeriodLabel: "202604", Value: 200},
	}
	result := chartdata.Shape(d, rows, nil, chartdata.ViewerFilters{}, chartdata.DrillContext{})

	if len(result.Series) == 0 {
		t.Fatal("expected series")
	}
	pt := result.Series[0].Points[0]
	if pt.Category != "202604" {
		t.Errorf("point category should be period label '202604', got %q", pt.Category)
	}
}

func TestShape_SingleMetric_Unchanged(t *testing.T) {
	// Dashboard without metric_filter → existing single-Series behavior.
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code: "EBITDA", Title: "EBITDA", FilterType: "MIS", FilterGroup1: "EBITDA",
		PeriodGrain: "MONTHLY", DefaultPeriod: "L12M", ChartType: "waterfall",
		ChartConfigRaw: map[string]any{"x_axis_field": "group_2", "y_axis_field": "display_value"},
		MaxDrillLevel:  3, CacheTTLSec: 60,
		GroupID: uuid.New(), IsActive: true, DrillEnabled: true, CreatedBy: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}

	rows := []factmetric.AggRow{
		{Category: "INCOME", Value: 5000, Order: 1},
		{Category: "PRODUCTION COST", Value: -3000, Order: 2},
	}
	result := chartdata.Shape(d, rows, nil, chartdata.ViewerFilters{}, chartdata.DrillContext{})

	// Single-metric: all rows go into one Series.
	if len(result.Series) != 1 {
		t.Fatalf("single-metric dashboard: want 1 series, got %d", len(result.Series))
	}
	if len(result.Series[0].Points) != 2 {
		t.Errorf("single-metric series should have 2 points")
	}
}

// multiMetricSeriesNames extracts series names for error messages.
func multiMetricSeriesNames(ss []chartdata.Series) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Name
	}
	return out
}
