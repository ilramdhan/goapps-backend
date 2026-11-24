package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// spyFixThrUpdater records the commands it receives.
type spyFixThrUpdater struct {
	cmds []app.UpdateFixCostMinThresholdCommand
	err  error
}

func (s *spyFixThrUpdater) Handle(_ context.Context, cmd app.UpdateFixCostMinThresholdCommand) (*domain.CostProductMaster, error) {
	s.cmds = append(s.cmds, cmd)
	if s.err != nil {
		return nil, s.err
	}
	p := rmModeTestProduct()
	v := cmd.Threshold
	p.RestoreFixCostMinThreshold(&v)
	return p, nil
}

func f64P(v float64) *float64 { return &v }

func newFixThrHandler(t *testing.T) (*CostProductMasterHandler, *spyFixThrUpdater) {
	t.Helper()
	h, err := NewCostProductMasterHandler(&rmModeCPMRepo{p: rmModeTestProduct()}, &fakeCPTRepo{byID: map[int32]string{2: "YARN"}})
	require.NoError(t, err)
	spy := &spyFixThrUpdater{}
	h.WithFixCostMinThreshold(spy)
	return h, spy
}

func TestUpdateCostProductMaster_FixCostMinThreshold(t *testing.T) {
	tests := []struct {
		name     string
		thr      *float64
		wantCall bool
	}{
		{name: "absent leaves it unchanged", thr: nil},
		{name: "zero turns the floor off", thr: f64P(0), wantCall: true},
		{name: "value passes through", thr: f64P(0.05), wantCall: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, spy := newFixThrHandler(t)
			resp, err := h.UpdateCostProductMaster(context.Background(), &financev1.UpdateCostProductMasterRequest{
				ProductSysId: 5, ProductName: "Product", GradeCode: "AX", FixcostMinThreshold: tt.thr,
			})
			require.NoError(t, err)
			require.True(t, resp.GetBase().GetIsSuccess(), resp.GetBase().GetMessage())
			if !tt.wantCall {
				assert.Empty(t, spy.cmds)
				assert.Nil(t, resp.GetData().FixcostMinThreshold)
				return
			}
			require.Len(t, spy.cmds, 1)
			assert.Equal(t, int64(5), spy.cmds[0].ProductSysID)
			assert.InDelta(t, *tt.thr, spy.cmds[0].Threshold, 1e-12)
			require.NotNil(t, resp.GetData().FixcostMinThreshold)
			assert.InDelta(t, *tt.thr, resp.GetData().GetFixcostMinThreshold(), 1e-12)
		})
	}
}

func TestUpdateCostProductMaster_FixCostMinThresholdNilUpdater(t *testing.T) {
	h, err := NewCostProductMasterHandler(&rmModeCPMRepo{p: rmModeTestProduct()}, &fakeCPTRepo{byID: map[int32]string{2: "YARN"}})
	require.NoError(t, err)
	resp, err := h.UpdateCostProductMaster(context.Background(), &financev1.UpdateCostProductMasterRequest{
		ProductSysId: 5, ProductName: "Product", GradeCode: "AX", FixcostMinThreshold: f64P(0.05),
	})
	require.NoError(t, err)
	assert.True(t, resp.GetBase().GetIsSuccess())
}

func TestUpdateCostProductMaster_FixCostMinThresholdInvalid(t *testing.T) {
	h, spy := newFixThrHandler(t)
	spy.err = domain.ErrInvalidFixCostMinThreshold
	resp, err := h.UpdateCostProductMaster(context.Background(), &financev1.UpdateCostProductMasterRequest{
		ProductSysId: 5, ProductName: "Product", GradeCode: "AX", FixcostMinThreshold: f64P(0.5),
	})
	require.NoError(t, err)
	assert.Equal(t, "400", resp.GetBase().GetStatusCode())
}

func TestCostProductMasterToProto_FixCostMinThreshold(t *testing.T) {
	p := rmModeTestProduct()
	assert.Nil(t, costProductMasterToProto(p).FixcostMinThreshold, "nil stays unset")

	p.RestoreFixCostMinThreshold(f64P(0))
	got := costProductMasterToProto(p)
	require.NotNil(t, got.FixcostMinThreshold, "0 = off is a set value")
	assert.Zero(t, got.GetFixcostMinThreshold())

	p.RestoreFixCostMinThreshold(f64P(0.07))
	assert.InDelta(t, 0.07, costProductMasterToProto(p).GetFixcostMinThreshold(), 1e-12)
}
