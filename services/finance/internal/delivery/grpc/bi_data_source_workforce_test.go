package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	chartdataapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dsapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/datasource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

type distinctsStub struct{ factmetric.Repository }

func (distinctsStub) GetDistincts(_ context.Context, s factmetric.DistinctScope) (factmetric.DistinctValues, error) {
	v := factmetric.DistinctValues{Types: []string{"MIS", "HR_OT_HOURS"}}
	if s.Type != "" {
		v.Group1s, v.Group2s, v.Group3s, v.DimensionKeys = []string{"PRD"}, []string{"LEMBUR"}, []string{"1234567"}, []string{"k"}
	}
	return v, nil
}

// HD-07/HD-10: HR_* metadata and NIK (group_3) in GetFactDistincts follow the caller's permissions.
func TestGetFactDistincts_WorkforcePermissions(t *testing.T) {
	h, err := NewBIDataSourceHandler(nil, dsapp.NewGetDistinctsHandler(distinctsStub{}))
	require.NoError(t, err)
	call := func(typ string, perms, roles []string) *financev1.FactMetricDistinct {
		ctx := context.WithValue(context.Background(), AuthPermissionsKey, perms)
		ctx = context.WithValue(ctx, AuthRolesKey, roles)
		resp, err := h.GetFactDistincts(ctx, &financev1.GetFactDistinctsRequest{Type: typ})
		require.NoError(t, err)
		return resp.GetData()
	}

	d := call("HR_OT_HOURS", nil, nil)
	assert.Equal(t, []string{"MIS"}, d.GetTypes(), "nama type HR_* disembunyikan")
	assert.Empty(t, d.GetGroup_1S())
	assert.Empty(t, d.GetGroup_2S())
	assert.Empty(t, d.GetGroup_3S())
	assert.Empty(t, d.GetDimensionKeys())

	d = call("HR_OT_HOURS", []string{chartdataapp.PermWorkforceView}, nil)
	assert.Contains(t, d.GetTypes(), "HR_OT_HOURS")
	assert.NotEmpty(t, d.GetGroup_1S())
	assert.Empty(t, d.GetGroup_3S(), "NIK butuh workforceemployee.view")

	d = call("HR_OT_HOURS", []string{chartdataapp.PermWorkforceView, chartdataapp.PermWorkforceEmployee}, nil)
	assert.Equal(t, []string{"1234567"}, d.GetGroup_3S())

	d = call("HR_OT_HOURS", nil, []string{"SUPER_ADMIN"})
	assert.Equal(t, []string{"1234567"}, d.GetGroup_3S())

	d = call("MIS", nil, nil)
	assert.Equal(t, []string{"1234567"}, d.GetGroup_3S(), "type non-HR tidak dibatasi")
}
