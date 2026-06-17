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

// TestComputePTYMELANGEOracleReference verifies the Oracle yarn formula chain
// produces correct values for a PTY MELANGE product with standard machine inputs.
//
// Route RM: GROUP "RM_TEST" ratio=1.0, rate=1.85 → COST_RM_TOTAL=1.85.
// Computed reference values for the given CAPP inputs:
//
//	RM_NORMS       ≈ 1.00704  (1/(1-0.7%))
//	NET_PRODUCTION ≈ 4451.3   kg/day  (504 pos × 800 m/min × 92% × 1440 × 75D / 9M)
//	NET_BOB_WT     ≈ 6.40     (4.8 × sum-of-grade-fractions)
//	COST_CAP_FINAL ≈ 3.238    (RM norms + conversion + quality loss)
//	COST_DEL_FINAL ≈ 3.244    (delivery packing slightly higher)
//	VB1_DEL_COST   ≈ 3.264    (COST_DEL_FINAL + 100/5000)
//	VB2_DEL_COST   ≈ 3.254    (COST_DEL_FINAL + 100/10000)
func TestComputePTYMELANGEOracleReference(t *testing.T) {
	t.Parallel()

	const productID int64 = 1

	capp := map[string]float64{
		"YARN_DENIER":      75.0,
		"NO_OF_POSITION":   504.0,
		"MC_SPEED":         800.0,
		"MC_EFFICIENCY":    92.0,
		"NO_OF_END":        1.0,
		"AX_WT":            4.8,
		"AX_PERC":          75.0,
		"AE_PERC":          15.0,
		"A9_PERC":          5.0,
		"A_PERC":           3.0,
		"B_PERC":           1.5,
		"C_PERC":           0.5,
		"WASTE_PCT":        0.7,
		"OPU":              2.2,
		"CAP_NO_OF_BOB":    6.0,
		"DEL_NO_OF_BOB":    6.0,
		"CAP_BOB_RATE":     0.35,
		"DEL_BOB_RATE":     0.35,
		"CAP_BOX_RATE":     1.2,
		"DEL_BOX_RATE":     1.4,
		"CHANGE_OVER_KG":   100.0,
		"VOL_BUCKET_1_QTY": 5000.0,
		"VOL_BUCKET_2_QTY": 10000.0,
		"VOL_BUCKET_3_QTY": 0.0,
		"VOL_BUCKET_4_QTY": 0.0,
		"VOL_BUCKET_5_QTY": 0.0,
		"POWER_PER_DAY":    2500.0,
		"MANPOWER_PER_DAY": 1800.0,
		"OVERHEAD_PER_DAY": 900.0,
		"SPARES_PER_DAY":   400.0,
		"MB_DOZING_PCT":    0.0,
		"MB_RATE":          0.0,
		"OIL_RATE":         0.0,
		"BC_PERC":          2.0,
		"NON_STD_PERC":     3.0,
		"BC_RECOVERY_RATE": 80.0,
		"SPECIAL_COST_1":   0.0,
		"INTERMINGLE_COST": 0.0,
	}

	route := &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: productID},
		Seqs: []*costroute.Seq{{
			HeadID:       1,
			ProductSysID: productID,
			RouteLevel:   1,
			Rms: []*costroute.Rm{{
				RmType:       costroute.RmTypeGroup,
				RmGroupCode:  "RM_TEST",
				RouteRmRatio: 1.0,
			}},
		}},
	}

	in := ComputeInput{
		ProductSysID:  productID,
		Period:        "202606",
		CalcType:      costcalcdom.CalcTypeActual,
		Route:         route,
		CAPP:          capp,
		Formulas:      buildOracleYarnFormulaChain(),
		RMCosts:       map[string]float64{"RM_TEST|": 1.85},
		UpstreamCosts: map[int64]float64{},
		EvalCache:     evaluator.NewCache(),
	}

	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)
	require.NotNil(t, out)

	const tol = 0.01 // 1-cent tolerance for floating-point rounding

	snap := out.ParamSnapshot
	assert.InDelta(t, 1.00704, snap["RM_NORMS"], tol, "RM_NORMS")
	assert.InDelta(t, 4451.3, snap["NET_PRODUCTION"], 1.0, "NET_PRODUCTION (kg/day)")
	assert.InDelta(t, 6.40, snap["NET_BOB_WT"], tol, "NET_BOB_WT")
	assert.InDelta(t, 3.238, snap["COST_CAP_FINAL"], tol, "COST_CAP_FINAL")
	assert.InDelta(t, 3.244, snap["COST_DEL_FINAL"], tol, "COST_DEL_FINAL")
	assert.InDelta(t, 3.264, snap["VB1_DEL_COST"], tol, "VB1_DEL_COST")
	assert.InDelta(t, 3.254, snap["VB2_DEL_COST"], tol, "VB2_DEL_COST")
	// VB3-5: VOL_BUCKET_N_QTY = 0 → guard returns 0 → VBN_DEL_COST = COST_DEL_FINAL.
	assert.InDelta(t, snap["COST_DEL_FINAL"], snap["VB3_DEL_COST"], tol, "VB3_DEL_COST")
	assert.InDelta(t, snap["COST_DEL_FINAL"], snap["VB4_DEL_COST"], tol, "VB4_DEL_COST")
	assert.InDelta(t, snap["COST_DEL_FINAL"], snap["VB5_DEL_COST"], tol, "VB5_DEL_COST")
	// Terminal formula passes COST_DEL_FINAL → COST_STAGE_OUT → CostPerUnit.
	assert.InDelta(t, snap["COST_DEL_FINAL"], out.CostPerUnit, tol, "CostPerUnit == COST_DEL_FINAL")
}

// buildOracleYarnFormulaChain returns the Oracle yarn formula chain in dependency order.
func buildOracleYarnFormulaChain() []Formula {
	return []Formula{
		// SEQ 0
		{FormulaCode: "F_YARN_RM_NORMS", Expression: "1.0 / (1.0 - WASTE_PCT / 100.0)", ResultParamCode: "RM_NORMS"},
		// SEQ 1a
		{FormulaCode: "F_YARN_AE_WT", Expression: "AX_WT * AE_PERC / AX_PERC", ResultParamCode: "AE_WT"},
		{FormulaCode: "F_YARN_A9_WT", Expression: "AX_WT * A9_PERC / AX_PERC", ResultParamCode: "A9_WT"},
		{FormulaCode: "F_YARN_A_WT", Expression: "AX_WT * A_PERC / AX_PERC", ResultParamCode: "A_WT"},
		{FormulaCode: "F_YARN_B_WT", Expression: "AX_WT * B_PERC / AX_PERC", ResultParamCode: "B_WT"},
		{FormulaCode: "F_YARN_C_WT", Expression: "AX_WT * C_PERC / AX_PERC", ResultParamCode: "C_WT"},
		{FormulaCode: "F_YARN_NET_BOB_WT", Expression: "AX_WT + AE_WT + A9_WT + A_WT + B_WT + C_WT", ResultParamCode: "NET_BOB_WT"},
		// SEQ 1c
		{FormulaCode: "F_YARN_OIL_COST", Expression: "OIL_RATE * OPU / 100.0", ResultParamCode: "OIL_COST"},
		{FormulaCode: "F_YARN_MB_COST", Expression: "MB_RATE * MB_DOZING_PCT / 100.0", ResultParamCode: "MB_COST"},
		{FormulaCode: "F_YARN_RP_DOZING", Expression: "MB_DOZING_PCT > 0 ? MB_DOZING_PCT : 0", ResultParamCode: "RP_DOZING"},
		// SEQ 2
		{FormulaCode: "F_YARN_NET_PROD", Expression: "NO_OF_POSITION * MC_SPEED * (MC_EFFICIENCY / 100.0) * 1440.0 * YARN_DENIER / 9000000.0", ResultParamCode: "NET_PRODUCTION"},
		{FormulaCode: "F_YARN_POWER_KG", Expression: "NET_PRODUCTION > 0 ? POWER_PER_DAY / NET_PRODUCTION : 0", ResultParamCode: "POWER_PER_KG"},
		{FormulaCode: "F_YARN_MANPOWER_KG", Expression: "NET_PRODUCTION > 0 ? MANPOWER_PER_DAY / NET_PRODUCTION : 0", ResultParamCode: "MANPOWER_PER_KG"},
		{FormulaCode: "F_YARN_OVERHEAD_KG", Expression: "NET_PRODUCTION > 0 ? OVERHEAD_PER_DAY * NO_OF_END / NET_PRODUCTION : 0", ResultParamCode: "OVERHEAD_PER_KG"},
		{FormulaCode: "F_YARN_SPARES_KG", Expression: "NET_PRODUCTION > 0 ? SPARES_PER_DAY / NET_PRODUCTION : 0", ResultParamCode: "SPARES_PER_KG"},
		{FormulaCode: "F_YARN_TOTAL_FIXED", Expression: "POWER_PER_KG + MANPOWER_PER_KG + OVERHEAD_PER_KG + SPARES_PER_KG", ResultParamCode: "TOTAL_FIXED_COST"},
		// SEQ 3
		{FormulaCode: "F_YARN_CAP_BOX_WT", Expression: "CAP_NO_OF_BOB * NET_BOB_WT * RM_NORMS", ResultParamCode: "CAP_BOX_WT"},
		{FormulaCode: "F_YARN_DEL_BOX_WT", Expression: "DEL_NO_OF_BOB * NET_BOB_WT * RM_NORMS", ResultParamCode: "DEL_BOX_WT"},
		// SEQ 4
		{FormulaCode: "F_YARN_CAP_PACK", Expression: "CAP_BOX_WT > 0 ? (CAP_NO_OF_BOB * CAP_BOB_RATE + CAP_BOX_RATE) / CAP_BOX_WT : 0", ResultParamCode: "CAP_PACK_COST"},
		{FormulaCode: "F_YARN_DEL_PACK", Expression: "DEL_BOX_WT > 0 ? (DEL_NO_OF_BOB * DEL_BOB_RATE + DEL_BOX_RATE) / DEL_BOX_WT : 0", ResultParamCode: "DEL_PACK_COST"},
		// SEQ 5
		{FormulaCode: "F_YARN_CONV_CAP", Expression: "TOTAL_FIXED_COST + CAP_PACK_COST + OIL_COST + INTERMINGLE_COST + SPECIAL_COST_1", ResultParamCode: "CONV_CAP_EX_MB"},
		{FormulaCode: "F_YARN_CONV_DEL", Expression: "TOTAL_FIXED_COST + DEL_PACK_COST + OIL_COST + INTERMINGLE_COST + SPECIAL_COST_1", ResultParamCode: "CONV_DEL_EX_MB"},
		// SEQ 6: COST_RM_TOTAL is injected by aggregateRMCost before formula eval.
		{FormulaCode: "F_YARN_CAP_PRE_QL", Expression: "RM_NORMS * COST_RM_TOTAL + CONV_CAP_EX_MB", ResultParamCode: "CAP_COST_PRE_QL"},
		{FormulaCode: "F_YARN_DEL_PRE_QL", Expression: "RM_NORMS * COST_RM_TOTAL + CONV_DEL_EX_MB", ResultParamCode: "DEL_COST_PRE_QL"},
		// SEQ 7
		{FormulaCode: "F_YARN_NON_STD_LOSS", Expression: "CAP_COST_PRE_QL * (NON_STD_PERC / 100.0) * (1.0 - BC_RECOVERY_RATE / 100.0)", ResultParamCode: "NON_STD_LOSS"},
		{FormulaCode: "F_YARN_BC_LOSS_CAP", Expression: "CAP_COST_PRE_QL * (BC_PERC / 100.0) * (1.0 - BC_RECOVERY_RATE / 100.0)", ResultParamCode: "BC_LOSS_CAP"},
		{FormulaCode: "F_YARN_BC_LOSS_DEL", Expression: "DEL_COST_PRE_QL * (BC_PERC / 100.0) * (1.0 - BC_RECOVERY_RATE / 100.0)", ResultParamCode: "BC_LOSS_DEL"},
		// SEQ 8
		{FormulaCode: "F_YARN_QLOSS_CAP", Expression: "BC_LOSS_CAP + NON_STD_LOSS", ResultParamCode: "QLOSS_CAP"},
		{FormulaCode: "F_YARN_QLOSS_DEL", Expression: "BC_LOSS_DEL + NON_STD_LOSS", ResultParamCode: "QLOSS_DEL"},
		// SEQ 9
		{FormulaCode: "F_YARN_CAP_FINAL", Expression: "CAP_COST_PRE_QL + QLOSS_CAP", ResultParamCode: "COST_CAP_FINAL"},
		{FormulaCode: "F_YARN_DEL_FINAL", Expression: "DEL_COST_PRE_QL + QLOSS_DEL", ResultParamCode: "COST_DEL_FINAL"},
		// SEQ 10
		{FormulaCode: "F_YARN_VB1_LOSS", Expression: "VOL_BUCKET_1_QTY > 0 ? CHANGE_OVER_KG / VOL_BUCKET_1_QTY : 0", ResultParamCode: "VB1_LOSS"},
		{FormulaCode: "F_YARN_VB2_LOSS", Expression: "VOL_BUCKET_2_QTY > 0 ? CHANGE_OVER_KG / VOL_BUCKET_2_QTY : 0", ResultParamCode: "VB2_LOSS"},
		{FormulaCode: "F_YARN_VB3_LOSS", Expression: "VOL_BUCKET_3_QTY > 0 ? CHANGE_OVER_KG / VOL_BUCKET_3_QTY : 0", ResultParamCode: "VB3_LOSS"},
		{FormulaCode: "F_YARN_VB4_LOSS", Expression: "VOL_BUCKET_4_QTY > 0 ? CHANGE_OVER_KG / VOL_BUCKET_4_QTY : 0", ResultParamCode: "VB4_LOSS"},
		{FormulaCode: "F_YARN_VB5_LOSS", Expression: "VOL_BUCKET_5_QTY > 0 ? CHANGE_OVER_KG / VOL_BUCKET_5_QTY : 0", ResultParamCode: "VB5_LOSS"},
		{FormulaCode: "F_YARN_VB1_DEL", Expression: "COST_DEL_FINAL + VB1_LOSS", ResultParamCode: "VB1_DEL_COST"},
		{FormulaCode: "F_YARN_VB2_DEL", Expression: "COST_DEL_FINAL + VB2_LOSS", ResultParamCode: "VB2_DEL_COST"},
		{FormulaCode: "F_YARN_VB3_DEL", Expression: "COST_DEL_FINAL + VB3_LOSS", ResultParamCode: "VB3_DEL_COST"},
		{FormulaCode: "F_YARN_VB4_DEL", Expression: "COST_DEL_FINAL + VB4_LOSS", ResultParamCode: "VB4_DEL_COST"},
		{FormulaCode: "F_YARN_VB5_DEL", Expression: "COST_DEL_FINAL + VB5_LOSS", ResultParamCode: "VB5_DEL_COST"},
		// Terminal: COST_STAGE_OUT = engine ScopeKeyFinalCost.
		{FormulaCode: "F_YARN_STAGE_OUT", Expression: "COST_DEL_FINAL", ResultParamCode: "COST_STAGE_OUT"},
	}
}
