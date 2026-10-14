package costcalc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// --- DENIER RM ratio mode (rm-ratio-denier, migration 000570) --------------

const (
	denierFG   int64 = 900
	denierUpA  int64 = 901 // PTY / POY1
	denierUpB  int64 = 902 // POY2
	spandexGrp       = "SPD0000006"
	oilGrp           = "RMP0000087"
)

func denierProductLine(id int64, upstream int64, ratio float64) *costroute.Rm {
	return &costroute.Rm{RmID: id, SeqID: 1, RmType: costroute.RmTypeProduct, RmProductSysID: upstream, RouteRmRatio: ratio}
}

func denierGroupLine(id int64, code string, ratio float64) *costroute.Rm {
	return &costroute.Rm{RmID: id, SeqID: 1, RmType: costroute.RmTypeGroup, RmGroupCode: code, RouteRmRatio: ratio}
}

func denierRoute(rms ...*costroute.Rm) *costroute.Graph {
	return &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: denierFG, RoutingStatus: costroute.StatusComplete},
		Seqs: []*costroute.Seq{
			{SeqID: 1, HeadID: 1, ProductSysID: denierFG, RouteLevel: 1, RouteSeq: 1, Rms: rms},
		},
	}
}

// denierInput builds an ACTUAL compute input whose final cost is
// COST_RM_TOTAL. The spandex group carries distinct CostVal / cascade rates so
// a test can tell which one a line used.
func denierInput(mode string, route *costroute.Graph) ComputeInput {
	return ComputeInput{
		ProductSysID: denierFG,
		Period:       "202609",
		CalcType:     costcalcdom.CalcTypeActual,
		Route:        route,
		CAPP:         map[string]float64{cappKeySpandexDraft: 3},
		Formulas:     []Formula{finalCostFormula("COST_RM_TOTAL")},
		RMCosts: map[string]RMCostRates{
			spandexGrp + "|": {CostVal: 100, CrRate: 7, ClRate: 8},
			oilGrp + "|":     {CostVal: 999, CrRate: 5, ClRate: 5},
		},
		UpstreamCosts:  map[int64]float64{denierUpA: 40, denierUpB: 33},
		EvalCache:      evaluator.NewCache(),
		RMRatioMode:    mode,
		RMGroupDenier:  map[string]float64{spandexGrp: 40},
		UpstreamDenier: map[int64]float64{denierUpA: 70, denierUpB: 150},
	}
}

// landedTotal runs the RM_LANDED_COST aggregation the way ComputeProduct does.
func landedTotal(t *testing.T, in ComputeInput) float64 {
	t.Helper()
	total, _, _, err := aggregateRMCost(in, planRMRatios(in), resolveRMLandedCost)
	require.NoError(t, err)
	return total
}

func TestComputeProduct_DenierRatio(t *testing.T) {
	draft33Spd := 40 / 3.3
	draft33 := (40*70 + 100*draft33Spd) / (70 + draft33Spd)

	tests := []struct {
		name       string
		route      *costroute.Graph
		mutate     func(in *ComputeInput)
		wantRM     float64
		wantLanded float64
		wantRatios []float64
		wantNotes  int
	}{
		{
			name:  "MEL: POY1 75D cap 30 + POY2 150D cap 33 -> 32",
			route: denierRoute(denierProductLine(1, denierUpA, 1), denierProductLine(2, denierUpB, 1)),
			mutate: func(in *ComputeInput) {
				in.UpstreamCosts[denierUpA] = 30
				in.UpstreamDenier[denierUpA] = 75
			},
			wantRM: 32, wantLanded: 32, wantRatios: []float64{1.0 / 3, 2.0 / 3},
		},
		{
			name:   "ACY: PTY 70D cap 40 + spandex 40D / draft 3 at cost_val 100 -> 49.6",
			route:  denierRoute(denierProductLine(1, denierUpA, 1), denierGroupLine(2, spandexGrp, 1)),
			wantRM: 49.6, wantLanded: 49.6, wantRatios: []float64{0.84, 0.16},
		},
		{
			name:   "ACY: draft 3.3",
			route:  denierRoute(denierProductLine(1, denierUpA, 1), denierGroupLine(2, spandexGrp, 1)),
			mutate: func(in *ComputeInput) { in.CAPP[cappKeySpandexDraft] = 3.3 },
			wantRM: draft33, wantLanded: draft33,
			wantRatios: []float64{70 / (70 + draft33Spd), draft33Spd / (70 + draft33Spd)},
		},
		{
			name: "ACY + non-denier group keeps route ratio 1 at its cascade unit 5",
			route: denierRoute(denierProductLine(1, denierUpA, 1), denierGroupLine(2, spandexGrp, 1),
				denierGroupLine(3, oilGrp, 1)),
			wantRM: 54.6, wantLanded: 54.6, wantRatios: []float64{0.84, 0.16, 1},
		},
		{
			name:   "PTY-only route -> ratio 1",
			route:  denierRoute(denierProductLine(1, denierUpA, 1)),
			wantRM: 40, wantLanded: 40, wantRatios: []float64{1},
		},
		{
			name:   "duplicate spandex lines, no PTY -> 0.5 / 0.5",
			route:  denierRoute(denierGroupLine(1, spandexGrp, 1), denierGroupLine(2, spandexGrp, 1)),
			wantRM: 100, wantLanded: 100, wantRatios: []float64{0.5, 0.5},
		},
		{
			name:   "missing upstream DENIER -> PTY 0 + note, spandex takes ratio 1",
			route:  denierRoute(denierProductLine(1, denierUpA, 1), denierGroupLine(2, spandexGrp, 1)),
			mutate: func(in *ComputeInput) { delete(in.UpstreamDenier, denierUpA) },
			wantRM: 100, wantLanded: 100, wantRatios: []float64{0, 1}, wantNotes: 1,
		},
		{
			name:   "missing SPANDEX_DRAFT -> spandex 0 + note",
			route:  denierRoute(denierProductLine(1, denierUpA, 1), denierGroupLine(2, spandexGrp, 1)),
			mutate: func(in *ComputeInput) { delete(in.CAPP, cappKeySpandexDraft) },
			wantRM: 40, wantLanded: 40, wantRatios: []float64{1, 0}, wantNotes: 1,
		},
		{
			name:  "all denier zero -> total 0 + notes, no error",
			route: denierRoute(denierProductLine(1, denierUpA, 1), denierGroupLine(2, spandexGrp, 1)),
			mutate: func(in *ComputeInput) {
				in.UpstreamDenier = nil
				in.CAPP[cappKeySpandexDraft] = 0
			},
			wantRM: 0, wantLanded: 0, wantRatios: []float64{0, 0}, wantNotes: 3,
		},
		{
			name: "duplicate notes are deduped",
			route: denierRoute(denierProductLine(1, denierUpA, 1), denierGroupLine(2, spandexGrp, 1),
				denierGroupLine(3, spandexGrp, 1)),
			mutate: func(in *ComputeInput) { delete(in.CAPP, cappKeySpandexDraft) },
			wantRM: 40, wantLanded: 40, wantRatios: []float64{1, 0, 0}, wantNotes: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := denierInput(RMRatioModeDenier, tc.route)
			if tc.mutate != nil {
				tc.mutate(&in)
			}
			out, err := ComputeProduct(context.Background(), in)
			require.NoError(t, err)
			assert.InDelta(t, tc.wantRM, out.TotalRMCost, 1e-9)
			assert.InDelta(t, tc.wantRM, out.CostPerUnit, 1e-9)
			assert.InDelta(t, tc.wantLanded, landedTotal(t, in), 1e-9)
			require.Len(t, out.RMCostDetail, len(tc.wantRatios))
			for i, want := range tc.wantRatios {
				assert.InDelta(t, want, out.RMCostDetail[i].Ratio, 1e-9, "line %d ratio", i)
				assert.NotEmpty(t, out.RMCostDetail[i].RatioSource)
			}
			assert.Len(t, out.Notes, tc.wantNotes, "notes: %v", out.Notes)
		})
	}
}

func TestComputeProduct_DenierRatio_DetailAnnotations(t *testing.T) {
	in := denierInput(RMRatioModeDenier, denierRoute(denierProductLine(1, denierUpA, 1),
		denierGroupLine(2, spandexGrp, 1), denierGroupLine(3, oilGrp, 1)))
	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out.RMCostDetail, 3)

	pty, spd, oil := out.RMCostDetail[0], out.RMCostDetail[1], out.RMCostDetail[2]
	assert.Equal(t, ratioSourceDenier, pty.RatioSource)
	assert.InDelta(t, 70, pty.Denier, 1e-9)
	assert.InDelta(t, 40, pty.UnitCost, 1e-9)

	assert.Equal(t, ratioSourceDenier, spd.RatioSource)
	assert.InDelta(t, 40.0/3, spd.Denier, 1e-9)
	assert.InDelta(t, 100, spd.UnitCost, 1e-9, "spandex uses cost_val, not the CR cascade")

	assert.Equal(t, ratioSourceInput, oil.RatioSource)
	assert.Zero(t, oil.Denier)
	assert.InDelta(t, 5, oil.UnitCost, 1e-9)
	assert.Nil(t, out.Notes)
}

// TestComputeProduct_DenierRatio_MasterDenierWinsOverSnapshot proves the
// upstream DENIER comes from the product master (UpstreamDenier) only: a
// conflicting snapshot value is ignored, and a snapshot alone is no fallback.
func TestComputeProduct_DenierRatio_MasterDenierWinsOverSnapshot(t *testing.T) {
	route := denierRoute(denierProductLine(1, denierUpA, 1), denierGroupLine(2, spandexGrp, 1))

	in := denierInput(RMRatioModeDenier, route)
	in.UpstreamSnapshots = map[int64]map[string]float64{denierUpA: {"DENIER": 999}}
	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	assert.InDelta(t, 49.6, out.TotalRMCost, 1e-9)
	assert.InDelta(t, 70, out.RMCostDetail[0].Denier, 1e-9)

	noMaster := denierInput(RMRatioModeDenier, route)
	noMaster.UpstreamDenier = nil
	noMaster.UpstreamSnapshots = map[int64]map[string]float64{denierUpA: {"DENIER": 70}}
	out, err = ComputeProduct(context.Background(), noMaster)
	require.NoError(t, err)
	assert.InDelta(t, 100, out.TotalRMCost, 1e-9, "snapshot DENIER is not a fallback")
	assert.Len(t, out.Notes, 1)
}

func TestComputeProduct_DenierRatio_MissingGroupCostRowBlocks(t *testing.T) {
	in := denierInput(RMRatioModeDenier, denierRoute(denierProductLine(1, denierUpA, 1), denierGroupLine(2, spandexGrp, 1)))
	delete(in.RMCosts, spandexGrp+"|")
	_, err := ComputeProduct(context.Background(), in)
	require.Error(t, err)
	assert.ErrorIs(t, err, costcalcdom.ErrMissingRMCost)
}

// TestComputeProduct_InputModeUnchanged is the regression guard: the same ACY
// route under "" / INPUT mode is the legacy Σ(unit × route ratio) with the GROUP
// cascades, no notes and RMCostDetail JSON without the new keys.
func TestComputeProduct_InputModeUnchanged(t *testing.T) {
	for _, mode := range []string{"", RMRatioModeInput} {
		t.Run("mode="+mode, func(t *testing.T) {
			in := denierInput(mode, denierRoute(denierProductLine(1, denierUpA, 1),
				denierGroupLine(2, spandexGrp, 1), denierGroupLine(3, oilGrp, 0.5)))
			out, err := ComputeProduct(context.Background(), in)
			require.NoError(t, err)
			// 40×1 + CR 7×1 + CR 5×0.5
			assert.InDelta(t, 49.5, out.TotalRMCost, 1e-9)
			// 40×1 + CL 8×1 + CL 5×0.5
			assert.InDelta(t, 50.5, landedTotal(t, in), 1e-9)
			assert.Nil(t, out.Notes)

			b, err := json.Marshal(out.RMCostDetail)
			require.NoError(t, err)
			assert.NotContains(t, string(b), "denier")
			assert.NotContains(t, string(b), "ratio_source")

			legacy := in
			legacy.RMRatioMode, legacy.RMGroupDenier, legacy.UpstreamDenier = "", nil, nil
			legacyOut, err := ComputeProduct(context.Background(), legacy)
			require.NoError(t, err)
			assert.Equal(t, legacyOut.RMCostDetail, out.RMCostDetail)
			assert.Equal(t, legacyOut.InputHash, out.InputHash)
		})
	}
}

func TestBuildCalculationLog_Notes(t *testing.T) {
	var doc map[string]any
	require.NoError(t, json.Unmarshal(buildCalculationLog(&ComputeOutput{}, "202609"), &doc))
	assert.NotContains(t, doc, "notes")

	doc = nil
	require.NoError(t, json.Unmarshal(buildCalculationLog(&ComputeOutput{Notes: []string{"n1"}}, "202609"), &doc))
	assert.Equal(t, []any{"n1"}, doc["notes"])
}

func TestCollectGroupCodes(t *testing.T) {
	routes := map[int64]*costroute.Graph{
		1: denierRoute(denierGroupLine(1, spandexGrp, 1), denierProductLine(2, denierUpA, 1), denierGroupLine(3, spandexGrp, 1)),
		2: denierRoute(denierGroupLine(4, oilGrp, 1), &costroute.Rm{RmType: costroute.RmTypeItem, RmItemCode: "ITEM1"}),
		3: nil,
	}
	assert.ElementsMatch(t, []string{spandexGrp, oilGrp}, collectGroupCodes(routes))
}
