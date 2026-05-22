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

// --- fixture helpers -------------------------------------------------------

func buildOneStageRoute(productSysID int64, rmType, refCode string, ratio float64) *costroute.Graph {
	rm := &costroute.Rm{
		RmID:         1,
		SeqID:        1,
		RmType:       rmType,
		RouteRmRatio: ratio,
	}
	switch rmType {
	case costroute.RmTypeItem:
		rm.RmItemCode = refCode
	case costroute.RmTypeGroup:
		rm.RmGroupCode = refCode
	case costroute.RmTypeProduct:
		// refCode unused; caller wires RmProductSysID directly when needed.
	}
	return &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: productSysID, RoutingStatus: costroute.StatusComplete},
		Seqs: []*costroute.Seq{
			{SeqID: 1, HeadID: 1, ProductSysID: productSysID, RouteLevel: 1, RouteSeq: 1, Rms: []*costroute.Rm{rm}},
		},
	}
}

func buildTwoStageRoute(fgProductSysID, upstreamProductSysID int64, fgRatio float64) *costroute.Graph {
	upstreamRm := &costroute.Rm{
		RmID:           1,
		SeqID:          1,
		RmType:         costroute.RmTypeProduct,
		RmProductSysID: upstreamProductSysID,
		RouteRmRatio:   fgRatio,
	}
	return &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: fgProductSysID, RoutingStatus: costroute.StatusComplete},
		Seqs: []*costroute.Seq{
			// Level-1 FG seq consumes the upstream PRODUCT.
			{SeqID: 1, HeadID: 1, ProductSysID: fgProductSysID, RouteLevel: 1, RouteSeq: 1, Rms: []*costroute.Rm{upstreamRm}},
			// Level-2 upstream seq (its cost is sourced from UpstreamCosts, not
			// recomputed in this product's compute pass).
			{SeqID: 2, HeadID: 1, ProductSysID: upstreamProductSysID, RouteLevel: 2, RouteSeq: 1, Rms: nil},
		},
	}
}

func multiLevelOnSameProduct(productSysID int64) *costroute.Graph {
	// Two seqs both producing the same FG product but at different levels —
	// exercises the per-level aggregation in CostByLevel.
	rm1 := &costroute.Rm{RmID: 1, SeqID: 1, RmType: costroute.RmTypeItem, RmItemCode: "RM_A", RouteRmRatio: 1.0}
	rm2 := &costroute.Rm{RmID: 2, SeqID: 2, RmType: costroute.RmTypeItem, RmItemCode: "RM_B", RouteRmRatio: 2.0}
	return &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: productSysID, RoutingStatus: costroute.StatusComplete},
		Seqs: []*costroute.Seq{
			{SeqID: 1, HeadID: 1, ProductSysID: productSysID, RouteLevel: 1, RouteSeq: 1, Rms: []*costroute.Rm{rm1}},
			{SeqID: 2, HeadID: 1, ProductSysID: productSysID, RouteLevel: 2, RouteSeq: 1, Rms: []*costroute.Rm{rm2}},
		},
	}
}

func finalCostFormula(expr string) Formula {
	return Formula{
		FormulaCode:     "F_FINAL",
		FormulaName:     "Final cost",
		Expression:      expr,
		ResultParamCode: ScopeKeyFinalCost,
		InputParamCodes: []string{ScopeKeyCostRMTotal, "WASTE_PCT"},
	}
}

// --- tests -----------------------------------------------------------------

func TestComputeProduct_HappyPath_OneItem(t *testing.T) {
	in := ComputeInput{
		ProductSysID: 42,
		Period:       "202604",
		CalcType:     costcalcdom.CalcTypeActual,
		Route:        buildOneStageRoute(42, costroute.RmTypeItem, "RM001", 1.0),
		CAPP:         map[string]float64{"WASTE_PCT": 5.0},
		Formulas:     []Formula{finalCostFormula("COST_RM_TOTAL * (1 + WASTE_PCT/100)")},
		RMCosts:      map[string]float64{"RM001|": 100.0},
		EvalCache:    evaluator.NewCache(),
	}

	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.InDelta(t, 105.0, out.CostPerUnit, 1e-9)
	assert.InDelta(t, 100.0, out.TotalRMCost, 1e-9)
	assert.Len(t, out.RMCostDetail, 1)
	assert.Equal(t, "RM001", out.RMCostDetail[0].RefCode)
	assert.Len(t, out.FormulaTrace, 1)
	assert.Equal(t, ScopeKeyFinalCost, out.FormulaTrace[0].ResultParamCode)
}

func TestComputeProduct_TwoStage_PRODUCTUpstream(t *testing.T) {
	in := ComputeInput{
		ProductSysID:  10,
		Period:        "202604",
		CalcType:      costcalcdom.CalcTypeActual,
		Route:         buildTwoStageRoute(10, 20, 2.0),
		CAPP:          map[string]float64{"WASTE_PCT": 0.0},
		Formulas:      []Formula{finalCostFormula("COST_RM_TOTAL * (1 + WASTE_PCT/100)")},
		UpstreamCosts: map[int64]float64{20: 50.0},
		EvalCache:     evaluator.NewCache(),
	}

	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	assert.InDelta(t, 100.0, out.TotalRMCost, 1e-9) // 50 * 2.0
	assert.InDelta(t, 100.0, out.CostPerUnit, 1e-9)
	assert.Equal(t, "product:20", out.RMCostDetail[0].RefCode)
}

func TestComputeProduct_MissingCAPP_SurfacesAsFormulaErr(t *testing.T) {
	// CAPP omits WASTE_PCT; evaluator treats undefined vars as nil → arithmetic
	// fails with non-numeric output, which our evaluator wraps as a run error.
	in := ComputeInput{
		ProductSysID: 1,
		Route:        buildOneStageRoute(1, costroute.RmTypeItem, "X", 1.0),
		CAPP:         map[string]float64{},
		Formulas:     []Formula{finalCostFormula("COST_RM_TOTAL * (1 + WASTE_PCT/100)")},
		RMCosts:      map[string]float64{"X|": 10.0},
		EvalCache:    evaluator.NewCache(),
	}
	_, err := ComputeProduct(context.Background(), in)
	require.Error(t, err)
	require.ErrorIs(t, err, costcalcdom.ErrFormulaEval)
}

func TestComputeProduct_MissingRMCost(t *testing.T) {
	in := ComputeInput{
		ProductSysID: 1,
		Route:        buildOneStageRoute(1, costroute.RmTypeItem, "RM_MISSING", 1.0),
		CAPP:         map[string]float64{},
		Formulas:     []Formula{finalCostFormula("COST_RM_TOTAL")},
		RMCosts:      map[string]float64{},
		EvalCache:    evaluator.NewCache(),
	}
	_, err := ComputeProduct(context.Background(), in)
	require.Error(t, err)
	require.ErrorIs(t, err, costcalcdom.ErrMissingRMCost)
}

func TestComputeProduct_MissingUpstream(t *testing.T) {
	in := ComputeInput{
		ProductSysID:  10,
		Route:         buildTwoStageRoute(10, 20, 1.0),
		Formulas:      []Formula{finalCostFormula("COST_RM_TOTAL")},
		UpstreamCosts: map[int64]float64{}, // empty → upstream 20 missing
		EvalCache:     evaluator.NewCache(),
	}
	_, err := ComputeProduct(context.Background(), in)
	require.Error(t, err)
	require.ErrorIs(t, err, costcalcdom.ErrMissingUpstreamCost)
}

func TestComputeProduct_DivByZero_ReturnsFormulaError(t *testing.T) {
	in := ComputeInput{
		ProductSysID: 1,
		Route:        buildOneStageRoute(1, costroute.RmTypeItem, "X", 1.0),
		CAPP:         map[string]float64{"DIVISOR": 0.0},
		Formulas: []Formula{{
			FormulaCode:     "F_FINAL",
			Expression:      "COST_RM_TOTAL / DIVISOR",
			ResultParamCode: ScopeKeyFinalCost,
			InputParamCodes: []string{ScopeKeyCostRMTotal, "DIVISOR"},
		}},
		RMCosts:   map[string]float64{"X|": 10.0},
		EvalCache: evaluator.NewCache(),
	}
	_, err := ComputeProduct(context.Background(), in)
	require.Error(t, err)
	require.ErrorIs(t, err, costcalcdom.ErrFormulaEval)
}

func TestComputeProduct_NoFinalCostKey_Errors(t *testing.T) {
	in := ComputeInput{
		ProductSysID: 1,
		Route:        buildOneStageRoute(1, costroute.RmTypeItem, "X", 1.0),
		Formulas: []Formula{{
			FormulaCode:     "F_SOMETHING_ELSE",
			Expression:      "COST_RM_TOTAL",
			ResultParamCode: "SOMETHING_ELSE",
			InputParamCodes: []string{ScopeKeyCostRMTotal},
		}},
		RMCosts:   map[string]float64{"X|": 10.0},
		EvalCache: evaluator.NewCache(),
	}
	_, err := ComputeProduct(context.Background(), in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ScopeKeyFinalCost)
}

func TestComputeProduct_SnapshotIncludesInputs(t *testing.T) {
	in := ComputeInput{
		ProductSysID: 1,
		Route:        buildOneStageRoute(1, costroute.RmTypeItem, "X", 1.0),
		CAPP:         map[string]float64{"WASTE_PCT": 5.0, "OTHER": 99.0},
		Formulas:     []Formula{finalCostFormula("COST_RM_TOTAL * (1 + WASTE_PCT/100)")},
		RMCosts:      map[string]float64{"X|": 100.0},
		EvalCache:    evaluator.NewCache(),
	}
	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)

	snap := out.ParamSnapshot
	assert.Contains(t, snap, "WASTE_PCT")
	assert.Contains(t, snap, "OTHER")
	assert.Contains(t, snap, ScopeKeyCostRMTotal)
	assert.Contains(t, snap, ScopeKeyFinalCost)
	assert.InDelta(t, 5.0, snap["WASTE_PCT"], 1e-9)
	assert.InDelta(t, 99.0, snap["OTHER"], 1e-9)
	assert.InDelta(t, 100.0, snap[ScopeKeyCostRMTotal], 1e-9)
}

func TestComputeProduct_InputHashIsStable(t *testing.T) {
	mk := func() ComputeInput {
		return ComputeInput{
			ProductSysID: 7,
			Period:       "202604",
			CalcType:     costcalcdom.CalcTypeActual,
			Route:        buildOneStageRoute(7, costroute.RmTypeItem, "RM", 1.0),
			CAPP:         map[string]float64{"A": 1.0, "B": 2.0},
			Formulas:     []Formula{finalCostFormula("COST_RM_TOTAL")},
			RMCosts:      map[string]float64{"RM|": 50.0},
			EvalCache:    evaluator.NewCache(),
		}
	}
	out1, err := ComputeProduct(context.Background(), mk())
	require.NoError(t, err)
	out2, err := ComputeProduct(context.Background(), mk())
	require.NoError(t, err)
	assert.Equal(t, out1.InputHash, out2.InputHash)
	assert.NotEmpty(t, out1.InputHash)
}

func TestComputeProduct_CostByLevel_Aggregates(t *testing.T) {
	in := ComputeInput{
		ProductSysID: 99,
		Route:        multiLevelOnSameProduct(99),
		CAPP:         map[string]float64{},
		Formulas:     []Formula{finalCostFormula("COST_RM_TOTAL")},
		RMCosts: map[string]float64{
			"RM_A|": 10.0, // contribution = 10 * 1.0 = 10 at level 1
			"RM_B|": 20.0, // contribution = 20 * 2.0 = 40 at level 2
		},
		EvalCache: evaluator.NewCache(),
	}
	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out.CostByLevel, 2)
	assert.Equal(t, int32(1), out.CostByLevel[0].Level)
	assert.InDelta(t, 10.0, out.CostByLevel[0].RMCost, 1e-9)
	assert.Equal(t, int32(2), out.CostByLevel[1].Level)
	assert.InDelta(t, 40.0, out.CostByLevel[1].RMCost, 1e-9)
	assert.InDelta(t, 50.0, out.TotalRMCost, 1e-9)
}
