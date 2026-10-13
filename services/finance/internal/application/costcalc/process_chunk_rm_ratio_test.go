package costcalc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// rmRatioChunkLoader extends mbGuardLoader with routes that reference GROUP
// RMs and recording DENIER-mode loaders, to prove bulkLoad wires the two
// per-chunk queries (one call each) into the bundle.
type rmRatioChunkLoader struct {
	mbGuardLoader
	gotModeIDs    []int64
	gotGroupCodes []string
	modeCalls     int
	denierCalls   int
	denierErr     error
}

func (f *rmRatioChunkLoader) LoadRoutesByProducts(_ context.Context, ids []int64) (map[int64]*costroute.Graph, error) {
	out := map[int64]*costroute.Graph{}
	for _, id := range ids {
		out[id] = &costroute.Graph{
			Head: &costroute.Head{HeadID: id * 10},
			Seqs: []*costroute.Seq{{ProductSysID: id, RouteLevel: 1, Rms: []*costroute.Rm{
				{RmType: costroute.RmTypeGroup, RmGroupCode: "SPD1", RouteRmRatio: 1},
				{RmType: costroute.RmTypeProduct, RmProductSysID: 7, RouteRmRatio: 1},
			}}},
		}
	}
	return out, nil
}

func (f *rmRatioChunkLoader) LoadProductRMRatioModes(_ context.Context, ids []int64) (map[int64]string, error) {
	f.modeCalls++
	f.gotModeIDs = ids
	return map[int64]string{ids[0]: RMRatioModeDenier}, nil
}

func (f *rmRatioChunkLoader) LoadRMGroupDeniers(_ context.Context, codes []string) (map[string]float64, error) {
	f.denierCalls++
	f.gotGroupCodes = codes
	if f.denierErr != nil {
		return nil, f.denierErr
	}
	return map[string]float64{"SPD1": 40}, nil
}

func TestBulkLoad_LoadsRMRatioInputsOncePerChunk(t *testing.T) {
	loader := &rmRatioChunkLoader{}
	svc := NewService(nil, nil, nil, nil, nil, loader, nil, nil, nil)

	got, err := svc.bulkLoad(context.Background(), mbGuardInput([]int64{11, 12}))
	require.NoError(t, err)
	assert.Equal(t, 1, loader.modeCalls)
	assert.Equal(t, 1, loader.denierCalls)
	assert.Equal(t, []int64{11, 12}, loader.gotModeIDs)
	assert.Equal(t, []string{"SPD1"}, loader.gotGroupCodes, "group codes are deduped across routes")
	assert.Equal(t, RMRatioModeDenier, got.rmRatioModes[11])
	assert.Empty(t, got.rmRatioModes[12])
	assert.InDelta(t, 40, got.rmGroupDeniers["SPD1"], 1e-9)
}

func TestBulkLoad_RMGroupDenierErrorAbortsChunk(t *testing.T) {
	sentinel := errors.New("denier query failed")
	loader := &rmRatioChunkLoader{denierErr: sentinel}
	svc := NewService(nil, nil, nil, nil, nil, loader, nil, nil, nil)

	_, err := svc.bulkLoad(context.Background(), mbGuardInput([]int64{11}))
	require.ErrorIs(t, err, sentinel)
	assert.Contains(t, err.Error(), "load RM group deniers")
}
