package costcalc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// --- RM denier override (rm-denier-override, migration 000578) -------------

func TestComputeProduct_DenierOverride_DetailSource(t *testing.T) {
	in := denierInput(RMRatioModeDenier, denierRoute(denierProductLine(1, denierUpA, 1),
		denierGroupLine(2, spandexGrp, 1), denierGroupLine(3, oilGrp, 1)))
	in.RMDenierOverride = costroute.DenierOverrides{Group: map[string]float64{spandexGrp: 30}}

	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out.RMCostDetail, 3)
	pty, spd, oil := out.RMCostDetail[0], out.RMCostDetail[1], out.RMCostDetail[2]

	assert.Equal(t, denierSourceMaster, pty.DenierSource)
	assert.InDelta(t, 70, pty.Denier, 1e-9)
	assert.InDelta(t, 70.0/100, pty.Ratio, 1e-9)

	assert.Equal(t, denierSourceOverride, spd.DenierSource)
	assert.Equal(t, ratioSourceDenier, spd.RatioSource, "ratio_source stays DENIER")
	assert.InDelta(t, 30, spd.Denier, 1e-9, "override is not divided by SPANDEX_DRAFT")
	assert.InDelta(t, 30.0/100, spd.Ratio, 1e-9)

	assert.Empty(t, oil.DenierSource, "non-denier line has no denier source")
	assert.InDelta(t, 40*0.7+100*0.3+5*1, out.TotalRMCost, 1e-9, "oil INPUT line keeps 5 x ratio 1")
}

func TestComputeProduct_DenierOverride_ProductLine(t *testing.T) {
	in := denierInput(RMRatioModeDenier, denierRoute(denierProductLine(1, denierUpA, 1), denierProductLine(2, denierUpB, 1)))
	in.RMDenierOverride = costroute.DenierOverrides{Product: map[int64]float64{denierUpB: 30}}
	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	assert.Equal(t, denierSourceMaster, out.RMCostDetail[0].DenierSource)
	assert.Equal(t, denierSourceOverride, out.RMCostDetail[1].DenierSource)
	assert.InDelta(t, 40*70.0/100+33*30.0/100, out.TotalRMCost, 1e-9)
}

func TestComputeProduct_DenierOverride_NilEqualsEmpty(t *testing.T) {
	route := func() *costroute.Graph {
		return denierRoute(denierProductLine(1, denierUpA, 1), denierGroupLine(2, spandexGrp, 1), denierGroupLine(3, oilGrp, 1))
	}
	base, err := ComputeProduct(context.Background(), denierInput(RMRatioModeDenier, route()))
	require.NoError(t, err)
	in := denierInput(RMRatioModeDenier, route())
	in.RMDenierOverride = costroute.DenierOverrides{Product: map[int64]float64{}, Group: map[string]float64{}}
	got, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	assert.Equal(t, base.RMCostDetail, got.RMCostDetail)
	assert.Equal(t, base.InputHash, got.InputHash)
	assert.InDelta(t, base.TotalRMCost, got.TotalRMCost, 0)
}

func TestComputeProduct_DenierOverride_InputModeIgnored(t *testing.T) {
	route := func() *costroute.Graph {
		return denierRoute(denierProductLine(1, denierUpA, 0.5), denierGroupLine(2, spandexGrp, 0.5))
	}
	base, err := ComputeProduct(context.Background(), denierInput(RMRatioModeInput, route()))
	require.NoError(t, err)
	in := denierInput(RMRatioModeInput, route())
	in.RMDenierOverride = costroute.DenierOverrides{
		Product: map[int64]float64{denierUpA: 1},
		Group:   map[string]float64{spandexGrp: 1},
	}
	got, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	assert.Equal(t, base.RMCostDetail, got.RMCostDetail)
	assert.InDelta(t, base.TotalRMCost, got.TotalRMCost, 0)
	for _, d := range got.RMCostDetail {
		assert.Empty(t, d.DenierSource)
	}
}

// legacyInputHash is inputHash as it was before 000578 (no override entries).
func legacyInputHash(in ComputeInput, totalRM float64) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "p:%d|period:%s|type:%s|rm:%.6f|cappN:%d|fN:%d",
		in.ProductSysID, in.Period, in.CalcType, totalRM, len(in.CAPP), len(in.Formulas))
	keys := make([]string, 0, len(in.CAPP))
	for k := range in.CAPP {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "|capp:%s=%.6f", k, in.CAPP[k])
	}
	for _, f := range in.Formulas {
		_, _ = fmt.Fprintf(h, "|f:%s=%s", f.FormulaCode, f.Expression)
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func TestInputHash_RMDenierOverride(t *testing.T) {
	in := denierInput(RMRatioModeDenier, denierRoute(denierProductLine(1, denierUpA, 1)))
	plain := inputHash(in, 12.5)
	assert.Equal(t, legacyInputHash(in, 12.5), plain, "hash unchanged without overrides")

	in.RMDenierOverride = costroute.DenierOverrides{Product: map[int64]float64{}, Group: map[string]float64{}}
	assert.Equal(t, plain, inputHash(in, 12.5), "empty override maps do not change the hash")

	in.RMDenierOverride = costroute.DenierOverrides{
		Product: map[int64]float64{denierUpB: 30, denierUpA: 20},
		Group:   map[string]float64{spandexGrp: 10, oilGrp: 5},
	}
	withOvr := inputHash(in, 12.5)
	assert.NotEqual(t, plain, withOvr, "overrides change the hash")
	for range 5 {
		assert.Equal(t, withOvr, inputHash(in, 12.5), "hash is deterministic (sorted)")
	}
	in.RMDenierOverride.Group[spandexGrp] = 11
	assert.NotEqual(t, withOvr, inputHash(in, 12.5), "override value is part of the hash")
}

// ovrChunkLoader records LoadRMDenierOverrides calls on top of the DENIER
// chunk loader (product 11 = DENIER, 12 = INPUT).
type ovrChunkLoader struct {
	rmRatioChunkLoader
	ovrCalls int
	gotOvr   []int64
	ovrErr   error
	noDenier bool
}

func (f *ovrChunkLoader) LoadProductRMRatioModes(ctx context.Context, ids []int64) (map[int64]string, error) {
	if f.noDenier {
		return map[int64]string{}, nil
	}
	return f.rmRatioChunkLoader.LoadProductRMRatioModes(ctx, ids)
}

func (f *ovrChunkLoader) LoadRMDenierOverrides(_ context.Context, ids []int64) (map[int64]costroute.DenierOverrides, error) {
	f.ovrCalls++
	f.gotOvr = ids
	if f.ovrErr != nil {
		return nil, f.ovrErr
	}
	return map[int64]costroute.DenierOverrides{11: {Group: map[string]float64{"SPD1": 12}}}, nil
}

func TestBulkLoad_LoadsRMDenierOverridesForDenierProducts(t *testing.T) {
	loader := &ovrChunkLoader{}
	svc := NewService(nil, nil, nil, nil, nil, loader, nil, nil, nil)
	got, err := svc.bulkLoad(context.Background(), mbGuardInput([]int64{11, 12}))
	require.NoError(t, err)
	assert.Equal(t, 1, loader.ovrCalls)
	assert.Equal(t, []int64{11}, loader.gotOvr, "only DENIER-mode products are queried")
	assert.InDelta(t, 12, got.rmDenierOverrides[11].Group["SPD1"], 1e-9)
}

func TestBulkLoad_SkipsRMDenierOverridesWithoutDenierProducts(t *testing.T) {
	loader := &ovrChunkLoader{noDenier: true}
	svc := NewService(nil, nil, nil, nil, nil, loader, nil, nil, nil)
	got, err := svc.bulkLoad(context.Background(), mbGuardInput([]int64{11, 12}))
	require.NoError(t, err)
	assert.Zero(t, loader.ovrCalls)
	assert.Nil(t, got.rmDenierOverrides)
}

func TestBulkLoad_RMDenierOverrideErrorAbortsChunk(t *testing.T) {
	sentinel := errors.New("override query failed")
	loader := &ovrChunkLoader{ovrErr: sentinel}
	svc := NewService(nil, nil, nil, nil, nil, loader, nil, nil, nil)
	_, err := svc.bulkLoad(context.Background(), mbGuardInput([]int64{11}))
	require.ErrorIs(t, err, sentinel)
	assert.Contains(t, err.Error(), "load RM denier overrides")
}
