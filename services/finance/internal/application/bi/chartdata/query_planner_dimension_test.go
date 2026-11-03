package chartdata_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chartdata "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

func buildDimensionDashboard(t *testing.T, factType string) *dashboarddomain.Dashboard {
	t.Helper()
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code: "PP_TEST", Title: "PP", FilterType: factType, PeriodGrain: "MONTHLY",
		DefaultPeriod: "THIS_MONTH", ChartType: "bar",
		ChartConfigRaw: map[string]any{
			"x_axis_field":   "group_1",
			"y_axis_field":   "value",
			"category_field": "dimension_key",
			"metric_filter":  map[string]any{"include_metrics": toAnySlice([]string{"PACK_TOTAL"})},
		},
		MaxDrillLevel: 2, CacheTTLSec: 60, GroupID: uuid.New(), IsActive: true, CreatedBy: uuid.New(),
	})
	require.NoError(t, err)
	return d
}

func TestPlan_DimensionKey_GroupsByDimensionWithStagingLabel(t *testing.T) {
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		factType string
		contains []string
	}{
		{"PP_PACK_PRODUCT", []string{"GROUP BY dimension_key", "DISTINCT ON (merge_no)", "prod_descr"}},
		{"PP_PACK_MACHINE", []string{"DISTINCT ON (mc_no)", "mach_order_rep"}},
		{"PP_PACK_POLY", []string{"GROUP BY dimension_key"}},
	}
	for _, tc := range tests {
		t.Run(tc.factType, func(t *testing.T) {
			plan, err := chartdata.Plan(buildDimensionDashboard(t, tc.factType), chartdata.ViewerFilters{PeriodPreset: "THIS_MONTH"}, now)
			require.NoError(t, err)
			assert.Equal(t, tc.factType, plan.Args[0])
			assert.Contains(t, plan.SQL, "metric_name = 'PACK_TOTAL'")
			assert.Contains(t, plan.SQL, "dimension_key <> ''")
			for _, c := range tc.contains {
				assert.Contains(t, plan.SQL, c)
			}
			assert.False(t, strings.Contains(plan.SQL, "UNION ALL"))
		})
	}
}

func TestShape_DimensionKey_KeepsDimensionLabelsAsCategories(t *testing.T) {
	d := buildDimensionDashboard(t, "PP_PACK_MACHINE")
	rows := []factmetric.AggRow{{Category: "M01", Value: 10}, {Category: "M02", Value: 5}}
	res := chartdata.Shape(d, rows, nil, chartdata.ViewerFilters{}, chartdata.DrillContext{})
	require.Len(t, res.Series, 1)
	assert.Equal(t, []string{"M01", "M02"}, res.Categories)
}

func TestPlan_DimensionKey_RejectsUnsafeMetricName(t *testing.T) {
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code: "PP_BAD", Title: "PP", FilterType: "PP_PACK_POLY", PeriodGrain: "MONTHLY",
		DefaultPeriod: "THIS_MONTH", ChartType: "bar",
		ChartConfigRaw: map[string]any{
			"x_axis_field": "group_1", "y_axis_field": "value", "category_field": "dimension_key",
			"metric_filter": map[string]any{"include_metrics": toAnySlice([]string{"X' OR 1=1 --"})},
		},
		MaxDrillLevel: 2, CacheTTLSec: 60, GroupID: uuid.New(), IsActive: true, CreatedBy: uuid.New(),
	})
	require.NoError(t, err)
	_, err = chartdata.Plan(d, chartdata.ViewerFilters{}, time.Now())
	require.Error(t, err)
}

func TestPlan_DimensionKey_RequiresMetricFilter(t *testing.T) {
	d, err := dashboarddomain.NewDashboard(dashboarddomain.NewDashboardParams{
		Code: "PP_NOMETRIC", Title: "PP", FilterType: "PP_PACK_POLY", PeriodGrain: "MONTHLY",
		DefaultPeriod: "THIS_MONTH", ChartType: "bar",
		ChartConfigRaw: map[string]any{
			"x_axis_field": "group_1", "y_axis_field": "value", "category_field": "dimension_key",
		},
		MaxDrillLevel: 2, CacheTTLSec: 60, GroupID: uuid.New(), IsActive: true, CreatedBy: uuid.New(),
	})
	require.NoError(t, err)
	_, err = chartdata.Plan(d, chartdata.ViewerFilters{}, time.Now())
	require.ErrorIs(t, err, factmetric.ErrInvalidPlan)
}
