package costcalc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
)

// =============================================================================
// Chunk wiring of the oil rate (oil-cost-rm-group P3-T6). Database-free:
// oilChunkLoader extends mbGuardLoader (a route per product, non-empty spin
// pool) with an oil context, the oil formula chain and a recording LoadRMCosts.
// =============================================================================

// Product ids of the oil chunk fixture.
const (
	oilChunkPTY     int64 = 501 // PTY, oil row present      -> SUCCESS
	oilChunkPTYMiss int64 = 502 // PTY, group without a row  -> BLOCKED
	oilChunkDTY     int64 = 503 // no oil class              -> SUCCESS, untouched
)

type oilChunkLoader struct {
	mbGuardLoader
	oil      map[int64]*OilInput
	oilErr   error
	rmRates  map[string]RMCostRates
	gotCodes []string
}

func (f *oilChunkLoader) LoadOilContext(_ context.Context, _ []int64) (map[int64]*OilInput, error) {
	return f.oil, f.oilErr
}

func (f *oilChunkLoader) LoadRMCosts(_ context.Context, codes []string, _, _ string) (map[string]RMCostRates, error) {
	f.gotCodes = append([]string(nil), codes...)
	return f.rmRates, nil
}

func (f *oilChunkLoader) LoadFormulas(_ context.Context, ids []int64) (map[int64][]Formula, error) {
	out := make(map[int64][]Formula, len(ids))
	for _, id := range ids {
		out[id] = oilFormulas(oilGainPOYDefault)
	}
	return out, nil
}

func (f *oilChunkLoader) LoadCAPP(_ context.Context, ids []int64) (map[int64]map[string]float64, error) {
	out := make(map[int64]map[string]float64, len(ids))
	for _, id := range ids {
		out[id] = map[string]float64{"OPU": 2.2, "WASTE_PERC": 0.7, "OIL_RATE": 5}
	}
	return out, nil
}

func newOilChunkLoader() *oilChunkLoader {
	return &oilChunkLoader{
		oil: map[int64]*OilInput{
			oilChunkPTY: {
				Class: OilClassPTY, TypeCode: "PTY", GroupCode: oilGroupConing,
				DefaultGroup: oilGroupConing, Allowed: map[string]bool{oilGroupConing: true, "OILNOROW": true},
			},
			oilChunkPTYMiss: {
				Class: OilClassPTY, TypeCode: "PTY", GroupCode: "OILNOROW",
				DefaultGroup: oilGroupConing, Allowed: map[string]bool{oilGroupConing: true, "OILNOROW": true},
			},
		},
		rmRates: map[string]RMCostRates{oilGroupConing + "|": {SrRate: 2.2869}},
	}
}

func TestProcessChunk_OilRate_CodesBlockAndNonOil(t *testing.T) {
	t.Parallel()
	loader := newOilChunkLoader()
	prodRepo := newRecordingProductRepo()
	resRepo := &recordingResultRepo{}
	svc := NewService(nil, &nopChunkRepo{}, prodRepo, resRepo, nil, loader, evaluator.NewCache(), nil, nil)

	out, err := svc.ProcessChunk(context.Background(),
		mbGuardInput([]int64{oilChunkPTY, oilChunkPTYMiss, oilChunkDTY}))
	require.NoError(t, err)

	// (a) oil group codes ride the LoadRMCosts query, once each.
	assert.ElementsMatch(t, []string{oilGroupConing, "OILNOROW"}, loader.gotCodes,
		"stored OIL_NAME and default codes must be added to the RM cost code list (deduped)")

	// (b) the product whose oil group has no cst_rm_cost row is BLOCKED.
	assert.Equal(t, blockReasonMissingRMCost, prodRepo.blocked[oilChunkPTYMiss])
	assert.NotContains(t, resRepo.upserted, oilChunkPTYMiss)

	// (c) the PTY with a row and the non-oil DTY in the same chunk succeed.
	assert.True(t, prodRepo.success[oilChunkPTY])
	assert.True(t, prodRepo.success[oilChunkDTY])
	assert.ElementsMatch(t, []int64{oilChunkPTY, oilChunkDTY}, resRepo.upserted)
	assert.Empty(t, prodRepo.failed)

	assert.Equal(t, 2, out.Success)
	assert.Equal(t, 1, out.Blocked)
	assert.Equal(t, 0, out.Failed)
}

func TestBulkLoad_OilContextStoredPerProduct(t *testing.T) {
	t.Parallel()
	loader := newOilChunkLoader()
	svc := NewService(nil, nil, nil, nil, nil, loader, nil, nil, nil)

	bundle, err := svc.bulkLoad(context.Background(), mbGuardInput([]int64{oilChunkPTY, oilChunkDTY}))
	require.NoError(t, err)
	require.NotNil(t, bundle.oil[oilChunkPTY])
	assert.Equal(t, OilClassPTY, bundle.oil[oilChunkPTY].Class)
	assert.Nil(t, bundle.oil[oilChunkDTY], "non-oil products carry no oil context")
}

func TestBulkLoad_OilContextError_AbortsChunk(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("oil query failed")
	loader := newOilChunkLoader()
	loader.oilErr = sentinel
	svc := NewService(nil, nil, nil, nil, nil, loader, nil, nil, nil)

	_, err := svc.bulkLoad(context.Background(), mbGuardInput([]int64{oilChunkPTY}))
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.Contains(t, err.Error(), "load oil context")
}
