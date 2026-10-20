package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// rmModeCPMRepo serves one active product from GetBySysID.
type rmModeCPMRepo struct {
	fakeCPMRepo
	p *domain.CostProductMaster
}

func (r *rmModeCPMRepo) GetBySysID(_ context.Context, _ int64) (*domain.CostProductMaster, error) {
	return r.p, nil
}

// spyRMModeUpdater records the commands it receives.
type spyRMModeUpdater struct {
	cmds []app.UpdateRMRatioModeCommand
	err  error
}

func (s *spyRMModeUpdater) Handle(_ context.Context, cmd app.UpdateRMRatioModeCommand) (*domain.CostProductMaster, error) {
	s.cmds = append(s.cmds, cmd)
	if s.err != nil {
		return nil, s.err
	}
	p := rmModeTestProduct()
	p.RestoreRMRatioMode(cmd.Mode, "DENIER")
	return p, nil
}

func rmModeTestProduct() *domain.CostProductMaster {
	at := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	return domain.Reconstruct(5, "P5", 2, "Product", "", "AX", "", "", "", "", nil, "",
		true, at, "seed", at, "seed", "", "", "", "", "", false)
}

func newRMModeHandler(t *testing.T) (*CostProductMasterHandler, *spyRMModeUpdater) {
	t.Helper()
	h, err := NewCostProductMasterHandler(&rmModeCPMRepo{p: rmModeTestProduct()}, &fakeCPTRepo{byID: map[int32]string{2: "YARN"}})
	require.NoError(t, err)
	spy := &spyRMModeUpdater{}
	h.WithRMRatioMode(spy)
	return h, spy
}

func strP(v string) *string { return &v }

func TestUpdateCostProductMaster_RMRatioModeOverride(t *testing.T) {
	tests := []struct {
		name     string
		mode     *string
		wantCall bool
		wantMode string
	}{
		{name: "absent leaves the mode unchanged", mode: nil},
		{name: "empty clears", mode: strP(""), wantCall: true, wantMode: ""},
		{name: "DENIER sets", mode: strP("DENIER"), wantCall: true, wantMode: "DENIER"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, spy := newRMModeHandler(t)
			resp, err := h.UpdateCostProductMaster(context.Background(), &financev1.UpdateCostProductMasterRequest{
				ProductSysId: 5, ProductName: "Product", GradeCode: "AX", RmRatioModeOverride: tt.mode,
			})
			require.NoError(t, err)
			require.True(t, resp.GetBase().GetIsSuccess(), resp.GetBase().GetMessage())
			if !tt.wantCall {
				assert.Empty(t, spy.cmds)
				return
			}
			require.Len(t, spy.cmds, 1)
			assert.Equal(t, int64(5), spy.cmds[0].ProductSysID)
			assert.Equal(t, tt.wantMode, spy.cmds[0].Mode)
			assert.Equal(t, tt.wantMode, resp.GetData().GetRmRatioModeOverride())
			assert.Equal(t, "DENIER", resp.GetData().GetEffectiveRmRatioMode())
		})
	}
}

func TestUpdateCostProductMaster_RMRatioModeInvalid(t *testing.T) {
	h, spy := newRMModeHandler(t)
	resp, err := h.UpdateCostProductMaster(context.Background(), &financev1.UpdateCostProductMasterRequest{
		ProductSysId: 5, ProductName: "Product", GradeCode: "AX", RmRatioModeOverride: strP("X"),
	})
	require.NoError(t, err)
	assert.False(t, resp.GetBase().GetIsSuccess(), "buf.validate rejects values outside \"\", INPUT, DENIER")
	assert.Empty(t, spy.cmds)

	spy.err = domain.ErrInvalidRMRatioModeOverride
	resp, err = h.UpdateCostProductMaster(context.Background(), &financev1.UpdateCostProductMasterRequest{
		ProductSysId: 5, ProductName: "Product", GradeCode: "AX", RmRatioModeOverride: strP("INPUT"),
	})
	require.NoError(t, err)
	assert.Equal(t, "400", resp.GetBase().GetStatusCode())
}

func TestCostProductMasterToProto_RMRatioMode(t *testing.T) {
	p := rmModeTestProduct()
	p.RestoreRMRatioMode("", "INPUT")
	got := costProductMasterToProto(p)
	assert.Empty(t, got.GetRmRatioModeOverride())
	assert.Equal(t, "INPUT", got.GetEffectiveRmRatioMode())

	p.RestoreRMRatioMode("DENIER", "DENIER")
	got = costProductMasterToProto(p)
	assert.Equal(t, "DENIER", got.GetRmRatioModeOverride())
	assert.Equal(t, "DENIER", got.GetEffectiveRmRatioMode())
}
