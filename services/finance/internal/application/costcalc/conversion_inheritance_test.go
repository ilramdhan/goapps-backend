package costcalc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// --- Route total conversion (conversion_inheritance.go, migration 000605) ----

// routeTotalConvFormulas mirrors the 000605 expressions plus a COST_STAGE_OUT
// sink, in topo order.
func routeTotalConvFormulas() []Formula {
	return []Formula{
		{
			FormulaCode: "F_YARN_CAP_CONV_STAGE", FormulaType: "CALCULATION",
			Expression:      "CAPTIVE_COST_QLTY_LOSS - RM_RATE + VOLUME_BUCKET_4_LOSS",
			ResultParamCode: "CAP_CONV_STAGE",
			InputParamCodes: []string{"CAPTIVE_COST_QLTY_LOSS", "RM_RATE", "VOLUME_BUCKET_4_LOSS"},
		},
		{
			FormulaCode: "F_YARN_DEL_CONV_STAGE", FormulaType: "CALCULATION",
			Expression:      "DELIVERY_COST_QLTY_LOSS - RM_RATE + VOLUME_BUCKET_4_LOSS",
			ResultParamCode: "DEL_CONV_STAGE",
			InputParamCodes: []string{"DELIVERY_COST_QLTY_LOSS", "RM_RATE", "VOLUME_BUCKET_4_LOSS"},
		},
		{
			FormulaCode: "F_YARN_TOTAL_CAP_CONV", FormulaType: "CALCULATION",
			Expression:      "CAP_CONV_STAGE + UPSTREAM_TOTAL_CAP_CONV",
			ResultParamCode: paramTotalCapConv,
			InputParamCodes: []string{"CAP_CONV_STAGE", paramUpstreamTotalCapConv},
		},
		{
			FormulaCode: "F_YARN_TOTAL_DEL_CONV", FormulaType: "CALCULATION",
			Expression:      "DEL_CONV_STAGE + UPSTREAM_TOTAL_CAP_CONV",
			ResultParamCode: "TOTAL_DEL_CONV",
			InputParamCodes: []string{"DEL_CONV_STAGE", paramUpstreamTotalCapConv},
		},
		{
			FormulaCode: "F_STAGE_OUT", FormulaType: "CALCULATION",
			Expression:      "COST_RM_TOTAL",
			ResultParamCode: ScopeKeyFinalCost,
			InputParamCodes: []string{ScopeKeyCostRMTotal},
		},
	}
}

// TestComputeProduct_RouteTotalConv_ExcelChain202608 replays the five-stage
// route POY -> PTY1574 -> PTY1632 -> TTY -> TTS of conv-cost-calc.xlsx
// (period 202608, ratio 1, VB4L 0), feeding each stage the previous stage's
// computed TOTAL_CAP_CONV as its upstream snapshot.
func TestComputeProduct_RouteTotalConv_ExcelChain202608(t *testing.T) {
	stages := []struct {
		name                 string
		capFinal, delFinal   float64
		rmRate               float64
		wantStage, wantTotal float64
	}{
		{"POY", 1.2159765781048217, 1.276370292390536, 1.03596449, 0.18001208810482178, 0.18001208810482178},
		{"PTY1574", 1.4285520125784457, 1.4801861871558941, 1.215977, 0.21257501257844555, 0.39258710068326733},
		{"PTY1632", 1.5026846897618886, 1.5026846897618886, 1.428552, 0.07413268976188858, 0.4667197904451559},
		{"TTY", 1.5741024619155437, 1.5741024619155437, 1.502685, 0.0714174619155435, 0.5381372523606994},
		{"TTS", 2.02934254833454, 2.0972158141903865, 1.574102, 0.45524054833454025, 0.9933778006952396},
	}

	var prevID int64
	var prevTotal, lastDel float64
	for i, st := range stages {
		id := int64(100 + i)
		in := ComputeInput{
			ProductSysID: id,
			Period:       "202608",
			CalcType:     costcalcdom.CalcTypeActual,
			CAPP: map[string]float64{
				"CAPTIVE_COST_QLTY_LOSS":  st.capFinal,
				"DELIVERY_COST_QLTY_LOSS": st.delFinal,
				"RM_RATE":                 st.rmRate,
				"VOLUME_BUCKET_4_LOSS":    0,
			},
			Formulas:  routeTotalConvFormulas(),
			EvalCache: evaluator.NewCache(),
		}
		if prevID == 0 {
			in.Route = buildOneStageRoute(id, costroute.RmTypeItem, "CHIP", 1.0)
			in.RMCosts = map[string]RMCostRates{"CHIP|": {CostVal: st.rmRate}}
		} else {
			in.Route = buildTwoStageRoute(id, prevID, 1.0)
			in.UpstreamCosts = map[int64]float64{prevID: st.rmRate}
			in.UpstreamSnapshots = map[int64]map[string]float64{prevID: {paramTotalCapConv: prevTotal}}
		}

		out, err := ComputeProduct(context.Background(), in)
		require.NoError(t, err, st.name)
		snap := out.ParamSnapshot
		upstream, ok := snap[paramUpstreamTotalCapConv]
		require.True(t, ok, "%s: %s must be persisted", st.name, paramUpstreamTotalCapConv)
		assert.InDelta(t, prevTotal, upstream, 1e-9, st.name)
		assert.InDelta(t, st.wantStage, snap["CAP_CONV_STAGE"], 1e-9, st.name)
		assert.InDelta(t, st.wantTotal, snap[paramTotalCapConv], 1e-9, st.name)
		// EI = EG (stage delivery conversion) + upstream route total captive.
		assert.InDelta(t, st.delFinal-st.rmRate+prevTotal, snap["TOTAL_DEL_CONV"], 1e-9, st.name)
		lastDel = snap["TOTAL_DEL_CONV"]

		prevID, prevTotal = id, snap[paramTotalCapConv]
	}
	assert.InDelta(t, 0.9933778006952396, prevTotal, 1e-9, "Excel EH TTS")
	assert.InDelta(t, 1.0612510665510861, lastDel, 1e-9, "Excel EI TTS = EG8 + EH7")
}

// totalCapConvInput builds a minimal product consuming UPSTREAM_TOTAL_CAP_CONV
// with the given route and upstream data.
func totalCapConvInput(route *costroute.Graph, upstreamCosts map[int64]float64) ComputeInput {
	return ComputeInput{
		ProductSysID:  10,
		Period:        "202608",
		CalcType:      costcalcdom.CalcTypeActual,
		Route:         route,
		CAPP:          map[string]float64{"CAPTIVE_COST_QLTY_LOSS": 5, "RM_RATE": 4},
		Formulas:      routeTotalConvFormulas(),
		RMCosts:       map[string]RMCostRates{"CHIP|": {CostVal: 1}},
		UpstreamCosts: upstreamCosts,
		EvalCache:     evaluator.NewCache(),
	}
}

func TestApplyUpstreamTotalCapConv_Cases(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(in *ComputeInput)
		route  *costroute.Graph
		want   float64
	}{
		{
			name:  "multi-RM ratios 0.6/0.4 give weighted sum",
			route: buildMultiUpstreamRoute(10, map[int64]float64{20: 0.6, 21: 0.4}),
			mutate: func(in *ComputeInput) {
				in.UpstreamSnapshots = map[int64]map[string]float64{
					20: {paramTotalCapConv: 0.5},
					21: {paramTotalCapConv: 0.25},
				}
			},
			want: 0.6*0.5 + 0.4*0.25,
		},
		{
			name:  "MB upstream skipped",
			route: buildMultiUpstreamRoute(10, map[int64]float64{20: 0.97, 21: 0.03}),
			mutate: func(in *ComputeInput) {
				in.UpstreamSnapshots = map[int64]map[string]float64{
					20: {paramTotalCapConv: 0.5},
					21: {paramTotalCapConv: 9},
				}
				in.VBLoss = &VBLossInheritance{ProductTypeCode: "DTY", UpstreamTypeCodes: map[int64]string{20: "POY", 21: "MB"}}
			},
			want: 0.97 * 0.5,
		},
		{
			name:  "missing snapshot and missing key count as 0",
			route: buildMultiUpstreamRoute(10, map[int64]float64{20: 0.5, 21: 0.3, 22: 0.2}),
			mutate: func(in *ComputeInput) {
				in.UpstreamSnapshots = map[int64]map[string]float64{
					20: {paramTotalCapConv: 0.4},
					21: {"OTHER": 1},
				}
			},
			want: 0.5 * 0.4,
		},
		{
			name:  "nil snapshots everywhere (e.g. mbbatch) yields 0",
			route: buildTwoStageRoute(10, 20, 1.0),
			want:  0,
		},
		{
			name:  "falls back to VBLoss.UpstreamParamSnapshots",
			route: buildTwoStageRoute(10, 20, 1.05),
			mutate: func(in *ComputeInput) {
				in.VBLoss = &VBLossInheritance{
					ProductTypeCode:        "DTY",
					UpstreamParamSnapshots: map[int64]map[string]float64{20: {paramTotalCapConv: 0.2}},
				}
			},
			want: 1.05 * 0.2,
		},
		{
			name:  "POY with no upstream PRODUCT RM yields 0",
			route: buildOneStageRoute(10, costroute.RmTypeItem, "CHIP", 1.0),
			want:  0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := totalCapConvInput(tt.route, map[int64]float64{20: 1, 21: 1, 22: 1})
			if tt.mutate != nil {
				tt.mutate(&in)
			}
			out, err := ComputeProduct(context.Background(), in)
			require.NoError(t, err)
			got, ok := out.ParamSnapshot[paramUpstreamTotalCapConv]
			require.True(t, ok, "%s must be persisted", paramUpstreamTotalCapConv)
			assert.InDelta(t, tt.want, got, 1e-12)
			assert.InDelta(t, 1+tt.want, out.ParamSnapshot[paramTotalCapConv], 1e-12)
		})
	}
}

func TestApplyUpstreamTotalCapConv_NotConsumed_NoScopeWrite(t *testing.T) {
	in := totalCapConvInput(buildTwoStageRoute(10, 20, 1.0), map[int64]float64{20: 1})
	in.Formulas = []Formula{finalCostFormula("COST_RM_TOTAL")}
	in.UpstreamSnapshots = map[int64]map[string]float64{20: {paramTotalCapConv: 0.7}}

	scope, zeroFilled := buildInitialScope(in)
	applyUpstreamTotalCapConv(in, nil, scope, zeroFilled)
	_, inScope := scope[paramUpstreamTotalCapConv]
	assert.False(t, inScope)

	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	_, inSnap := out.ParamSnapshot[paramUpstreamTotalCapConv]
	assert.False(t, inSnap, "products that do not consume the param get no new snapshot key")
}

func TestApplyUpstreamTotalCapConv_CalculatedGuardNotTripped(t *testing.T) {
	in := totalCapConvInput(buildTwoStageRoute(10, 20, 1.0), map[int64]float64{20: 1})
	in.UpstreamSnapshots = map[int64]map[string]float64{20: {paramTotalCapConv: 0.3}}
	in.CalculatedParams = map[string]bool{
		paramUpstreamTotalCapConv: true, "CAP_CONV_STAGE": true, "DEL_CONV_STAGE": true,
		paramTotalCapConv: true, "TOTAL_DEL_CONV": true,
	}
	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	assert.InDelta(t, 0.3, out.ParamSnapshot[paramUpstreamTotalCapConv], 1e-12)
}

func TestApplyUpstreamTotalCapConv_DenierModeUsesPlanRatio(t *testing.T) {
	// A DENIER-mode PRODUCT line uses its denier-derived ratio for RM_RATE;
	// the upstream conversion must be weighted by the same ratio.
	rm := &costroute.Rm{RmID: 1, SeqID: 1, RmType: costroute.RmTypeProduct, RmProductSysID: 20, RouteRmRatio: 1.0}
	plan := &rmRatioPlan{lines: map[*costroute.Rm]denierLine{rm: {denier: 75, ratio: 0.8}}}
	route := &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: 10, RoutingStatus: costroute.StatusComplete},
		Seqs: []*costroute.Seq{{SeqID: 1, HeadID: 1, ProductSysID: 10, RouteLevel: 1, RouteSeq: 1, Rms: []*costroute.Rm{rm}}},
	}
	in := totalCapConvInput(route, map[int64]float64{20: 1})
	in.UpstreamSnapshots = map[int64]map[string]float64{20: {paramTotalCapConv: 0.5}}

	scope := map[string]any{}
	zeroFilled := map[string]bool{paramUpstreamTotalCapConv: true}
	applyUpstreamTotalCapConv(in, plan, scope, zeroFilled)
	assert.InDelta(t, 0.4, scope[paramUpstreamTotalCapConv], 1e-12)
	assert.False(t, zeroFilled[paramUpstreamTotalCapConv])

	applyUpstreamTotalCapConv(in, nil, scope, zeroFilled)
	assert.InDelta(t, 0.5, scope[paramUpstreamTotalCapConv], 1e-12, "INPUT mode uses the route ratio")
}
