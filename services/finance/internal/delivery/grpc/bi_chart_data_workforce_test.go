package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	chartdataapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

type workforceFactStub struct {
	factmetric.Repository
	queries int
}

func (s *workforceFactStub) QueryAggregate(context.Context, factmetric.PlannedQuery) ([]factmetric.AggRow, error) {
	s.queries++
	return nil, nil
}

// HD-10: PreviewDashboard for an HR_* type is mapped from the caller's permissions;
// without finance.bi.workforce.view the request is refused and no query runs.
func TestPreviewDashboard_WorkforcePermissionMapping(t *testing.T) {
	cfg, err := structpb.NewStruct(map[string]any{"x_axis_field": "group_1", "y_axis_field": "value"})
	require.NoError(t, err)
	req := &financev1.PreviewDashboardRequest{
		FilterType:   "HR_OT_HOURS",
		PeriodeGrain: financev1.PeriodeGrain_PERIODE_GRAIN_MONTHLY,
		ChartType:    financev1.ChartType_CHART_TYPE_BAR,
		ChartConfig:  cfg,
	}
	for _, c := range []struct {
		name      string
		perms     []string
		roles     []string
		wantQuery bool
	}{
		{"tanpa izin", []string{"finance.bi.dashboard.view"}, nil, false},
		{"izin employee saja tidak cukup", []string{chartdataapp.PermWorkforceEmployee}, nil, false},
		{"dengan workforce.view", []string{chartdataapp.PermWorkforceView}, nil, true},
		{"super admin", nil, []string{"SUPER_ADMIN"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			fact := &workforceFactStub{}
			h, err := NewBIChartDataHandler(nil, chartdataapp.NewPreviewHandler(fact))
			require.NoError(t, err)
			ctx := context.WithValue(context.Background(), AuthPermissionsKey, c.perms)
			ctx = context.WithValue(ctx, AuthRolesKey, c.roles)
			resp, _ := h.PreviewDashboard(ctx, req)
			if c.wantQuery {
				assert.Positive(t, fact.queries, "harus sampai ke query")
			} else {
				assert.Zero(t, fact.queries, "tidak boleh query DB")
				assert.Equal(t, "403", resp.GetBase().GetStatusCode())
			}
		})
	}
}

// HD-07: preview with table_mode=employee_week exposes NIK, so it needs workforceemployee.view
// on top of workforce.view; without it no query runs.
func TestPreviewDashboard_EmployeeTableNeedsEmployeePermission(t *testing.T) {
	cfg, err := structpb.NewStruct(map[string]any{"x_axis_field": "group_1", "y_axis_field": "value", "table_mode": "employee_week"})
	require.NoError(t, err)
	req := &financev1.PreviewDashboardRequest{
		FilterType:   "HR_OT_WEEK",
		PeriodeGrain: financev1.PeriodeGrain_PERIODE_GRAIN_WEEKLY,
		ChartType:    financev1.ChartType_CHART_TYPE_DATA_TABLE,
		ChartConfig:  cfg,
	}
	for _, c := range []struct {
		name      string
		perms     []string
		wantQuery bool
	}{
		{"workforce.view saja", []string{chartdataapp.PermWorkforceView}, false},
		{"workforce + employee", []string{chartdataapp.PermWorkforceView, chartdataapp.PermWorkforceEmployee}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			fact := &workforceFactStub{}
			h, err := NewBIChartDataHandler(nil, chartdataapp.NewPreviewHandler(fact))
			require.NoError(t, err)
			ctx := context.WithValue(context.Background(), AuthPermissionsKey, c.perms)
			resp, _ := h.PreviewDashboard(ctx, req)
			if c.wantQuery {
				assert.Positive(t, fact.queries)
			} else {
				assert.Zero(t, fact.queries, "tidak boleh query DB")
				assert.Equal(t, "403", resp.GetBase().GetStatusCode())
			}
		})
	}
}
