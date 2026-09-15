package costcalc

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// This file is a MEASUREMENT test, not a lock test. It answers one question:
// does wiring OIL_GAIN into the conversion formulas as an input move the
// terminal-formula selection made by findTerminalFormula (compute.go:800-873)?
//
// Why the question is not obvious. The terminal formula is never configured
// explicitly; it is derived from the set of InputParamCodes (compute.go:812-818)
// and the longest formula-ancestor chain (compute.go:830-847), with FormulaCode
// ASC as the tie-break (compute.go:866). Today F_YARN_OIL_GAIN is a terminal
// SINK: its result param OIL_GAIN is consumed by no formula at all
// (migrations/postgres/000408_seed_oracle_formulas.up.sql:28 defines it with the
// literal expression '0', and no mst_formula_param row anywhere names OIL_GAIN).
// Consuming it removes one candidate from the terminal set, so the alphabetical
// tie-break is evaluated over a DIFFERENT set — and F_YARN_OIL_GAIN sorts before
// F_YARN_VB1_DEL..F_YARN_VB5_DEL.
//
// The fixture below is database-free: it calls ComputeProduct directly, exactly
// like compute_test.go / compute_non_finite_test.go.

// --- fixture ---------------------------------------------------------------

// yarnTerminalCAPP holds the non-formula (leaf) params of the fixture chain.
// Every key here is an input of some formula below but is produced by no
// formula, so it must arrive via CAPP rather than be zero-filled — that keeps
// the arithmetic non-degenerate and makes an accidental change in the chosen
// terminal show up as a numeric difference too.
func yarnTerminalCAPP() map[string]float64 {
	return map[string]float64{
		// Leaves of F_YARN_CONV_CAP / F_YARN_CONV_DEL
		// (000408_seed_oracle_formulas.up.sql:39,40; params :159,:161).
		"TOTAL_FIXEDCOST_PER_KG": 12.5,
		"CAPTIVE_PACK_COST":      3.25,
		"DELIVERY_PACK_COST":     4.75,
		"OIL_COST":               1.125,
		"INTERMINGLING":          0.5,
		"SPECIAL_COST_1":         2.0,
		// Leaves of F_YARN_CAP_PRE_QL / F_YARN_DEL_PRE_QL
		// (000408:41,42; params :163,:165).
		"RM_NORMS":       1.05,
		"RM_LANDED_COST": 100.0,
		// Leaves of F_YARN_CAP_FINAL / F_YARN_DEL_FINAL
		// (000408:48,49; params :179,:181). The QLOSS producers are deliberately
		// left out of the fixture — the point under test is terminal selection,
		// not the quality-loss sub-chain.
		"QLTY_LOSS_CAPTIVE_COST":  7.0,
		"QLTY_LOSS_DELIVERY_COST": 8.0,
		// Leaves of F_YARN_VB1..5_DEL (000408:55-59; params :190-194).
		"VOLUME_BUCKET_1_LOSS": 1.1,
		"VOLUME_BUCKET_2_LOSS": 2.2,
		"VOLUME_BUCKET_3_LOSS": 3.3,
		"VOLUME_BUCKET_4_LOSS": 4.4,
		"VOLUME_BUCKET_5_LOSS": 5.5,
	}
}

// yarnTerminalFormulas builds the yarn formula chain in topological order
// (the loader pre-sorts; see Formula.SortOrder in formula.go).
//
// withOilGainAsInput = false reproduces TODAY's shape: OIL_GAIN is produced but
// never consumed. withOilGainAsInput = true is the proposed shape: OIL_GAIN is
// appended to both conversion expressions and declared as their 6th input.
//
// Every expression and result param is copied verbatim from
// migrations/postgres/000408_seed_oracle_formulas.up.sql at the cited line.
func yarnTerminalFormulas(withOilGainAsInput bool) []Formula {
	convCapExpr := "TOTAL_FIXEDCOST_PER_KG + CAPTIVE_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1"
	convDelExpr := "TOTAL_FIXEDCOST_PER_KG + DELIVERY_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1"
	convCapInputs := []string{"TOTAL_FIXEDCOST_PER_KG", "CAPTIVE_PACK_COST", "OIL_COST", "INTERMINGLING", "SPECIAL_COST_1"}
	convDelInputs := []string{"TOTAL_FIXEDCOST_PER_KG", "DELIVERY_PACK_COST", "OIL_COST", "INTERMINGLING", "SPECIAL_COST_1"}
	if withOilGainAsInput {
		convCapExpr += " + OIL_GAIN"
		convDelExpr += " + OIL_GAIN"
		convCapInputs = append(convCapInputs, "OIL_GAIN")
		convDelInputs = append(convDelInputs, "OIL_GAIN")
	}

	return []Formula{
		// 000408:28 — literal '0', result param OIL_GAIN, is_active TRUE.
		{
			FormulaCode:     "F_YARN_OIL_GAIN",
			FormulaName:     "Oil Gain",
			FormulaType:     "CALCULATION",
			Expression:      "0",
			ResultParamCode: "OIL_GAIN",
			InputParamCodes: nil,
			SortOrder:       1,
		},
		// 000408:39 — formula_param at 000408:159.
		{
			FormulaCode:     "F_YARN_CONV_CAP",
			FormulaName:     "Conversion Captive",
			FormulaType:     "CALCULATION",
			Expression:      convCapExpr,
			ResultParamCode: "ONLY_CONV_CAP_PACK_EXCL_MB",
			InputParamCodes: convCapInputs,
			SortOrder:       2,
		},
		// 000408:40 — formula_param at 000408:161.
		{
			FormulaCode:     "F_YARN_CONV_DEL",
			FormulaName:     "Conversion Delivery",
			FormulaType:     "CALCULATION",
			Expression:      convDelExpr,
			ResultParamCode: "ONLY_CONV_DEL_PACK_EXCL_MB",
			InputParamCodes: convDelInputs,
			SortOrder:       3,
		},
		// 000408:41 — formula_param at 000408:163.
		{
			FormulaCode:     "F_YARN_CAP_PRE_QL",
			FormulaName:     "Captive Cost Before Quality Loss",
			FormulaType:     "CALCULATION",
			Expression:      "RM_NORMS * RM_LANDED_COST + ONLY_CONV_CAP_PACK_EXCL_MB",
			ResultParamCode: "CAPTIVE_COST_BEFORE_QLOSS",
			InputParamCodes: []string{"RM_NORMS", "RM_LANDED_COST", "ONLY_CONV_CAP_PACK_EXCL_MB"},
			SortOrder:       4,
		},
		// 000408:42 — formula_param at 000408:165.
		{
			FormulaCode:     "F_YARN_DEL_PRE_QL",
			FormulaName:     "Delivery Cost Before Quality Loss",
			FormulaType:     "CALCULATION",
			Expression:      "RM_NORMS * RM_LANDED_COST + ONLY_CONV_DEL_PACK_EXCL_MB",
			ResultParamCode: "DELIVERY_COST_BEFORE_QLOSS",
			InputParamCodes: []string{"RM_NORMS", "RM_LANDED_COST", "ONLY_CONV_DEL_PACK_EXCL_MB"},
			SortOrder:       5,
		},
		// 000408:48 — formula_param at 000408:179.
		{
			FormulaCode:     "F_YARN_CAP_FINAL",
			FormulaName:     "Captive Final Cost",
			FormulaType:     "CALCULATION",
			Expression:      "CAPTIVE_COST_BEFORE_QLOSS + QLTY_LOSS_CAPTIVE_COST",
			ResultParamCode: "CAPTIVE_COST_QLTY_LOSS",
			InputParamCodes: []string{"CAPTIVE_COST_BEFORE_QLOSS", "QLTY_LOSS_CAPTIVE_COST"},
			SortOrder:       6,
		},
		// 000408:49 — formula_param at 000408:181.
		{
			FormulaCode:     "F_YARN_DEL_FINAL",
			FormulaName:     "Delivery Final Cost",
			FormulaType:     "CALCULATION",
			Expression:      "DELIVERY_COST_BEFORE_QLOSS + QLTY_LOSS_DELIVERY_COST",
			ResultParamCode: "DELIVERY_COST_QLTY_LOSS",
			InputParamCodes: []string{"DELIVERY_COST_BEFORE_QLOSS", "QLTY_LOSS_DELIVERY_COST"},
			SortOrder:       7,
		},
		// 000408:55-59 — formula_param at 000408:190-194. These five are the
		// sibling terminals that make the tie-break observable at all.
		vbDelFormula(1, 8),
		vbDelFormula(2, 9),
		vbDelFormula(3, 10),
		vbDelFormula(4, 11),
		vbDelFormula(5, 12),
	}
}

// vbDelFormula builds F_YARN_VB{n}_DEL, all five of which share one shape
// (000408:55-59; formula_param 000408:190-194).
func vbDelFormula(n, sortOrder int) Formula {
	idx := string(rune('0' + n))
	return Formula{
		FormulaCode:     "F_YARN_VB" + idx + "_DEL",
		FormulaName:     "VB" + idx + " Delivery Cost",
		FormulaType:     "CALCULATION",
		Expression:      "DELIVERY_COST_QLTY_LOSS + VOLUME_BUCKET_" + idx + "_LOSS",
		ResultParamCode: "VOLUME_BUCKET_" + idx + "_DEL_COST",
		InputParamCodes: []string{"DELIVERY_COST_QLTY_LOSS", "VOLUME_BUCKET_" + idx + "_LOSS"},
		SortOrder:       sortOrder,
	}
}

// yarnTerminalInput wires the formula set onto a minimal one-stage route.
// Note there is deliberately NO COST_STAGE_OUT anywhere in the chain: that is
// what forces ComputeProduct down the resolveFinalCost branch
// (compute.go:211-220) and therefore through findTerminalFormula.
func yarnTerminalInput(withOilGainAsInput bool) ComputeInput {
	return ComputeInput{
		ProductSysID: 7001,
		Period:       "202604",
		CalcType:     costcalcdom.CalcTypeActual,
		Route:        buildOneStageRoute(7001, costroute.RmTypeItem, "RM_YARN", 1.0),
		CAPP:         yarnTerminalCAPP(),
		Formulas:     yarnTerminalFormulas(withOilGainAsInput),
		RMCosts:      map[string]float64{"RM_YARN|": 50.0},
		EvalCache:    evaluator.NewCache(),
	}
}

// --- tests -----------------------------------------------------------------

// TestFindTerminalFormula_OilGainSinkStatus documents the premise the main test
// rests on: OIL_GAIN is a terminal today and stops being one once consumed.
// If this ever fails, the main assertion below is testing nothing.
func TestFindTerminalFormula_OilGainSinkStatus(t *testing.T) {
	before := terminalCandidates(yarnTerminalFormulas(false))
	after := terminalCandidates(yarnTerminalFormulas(true))

	assert.Contains(t, before, "F_YARN_OIL_GAIN",
		"today OIL_GAIN is consumed by no formula, so it must be a terminal sink")
	assert.NotContains(t, after, "F_YARN_OIL_GAIN",
		"once OIL_GAIN is an input of the conversion formulas it must leave the terminal set")
	assert.Len(t, after, len(before)-1,
		"exactly one candidate must disappear — the alphabetical tie-break now runs over a different set")
}

// terminalCandidates mirrors the terminal-detection step of findTerminalFormula
// (compute.go:812-851) so the test can name the candidate set, which the
// production function does not expose.
func terminalCandidates(formulas []Formula) []string {
	allInputs := make(map[string]bool, len(formulas)*2)
	for _, f := range formulas {
		for _, inp := range f.InputParamCodes {
			allInputs[inp] = true
		}
	}
	var out []string
	for _, f := range formulas {
		if !allInputs[f.ResultParamCode] {
			out = append(out, f.FormulaCode)
		}
	}
	return out
}

// TestComputeProduct_OilGainAsInput_DoesNotMoveTerminalFormula is the gate.
//
// Assertion (a): CostPerUnit is identical to within 1e-12 before and after.
// Assertion (b): the formula findTerminalFormula selects is the SAME before and
// after.
//
// If (b) fails, do NOT relax it and do NOT change compute.go — the failure is
// the finding: it means the terminal formula is sensitive to OIL_GAIN's wiring
// and migration 000510 cannot proceed as drafted.
func TestComputeProduct_OilGainAsInput_DoesNotMoveTerminalFormula(t *testing.T) {
	beforeFormulas := yarnTerminalFormulas(false)
	afterFormulas := yarnTerminalFormulas(true)

	termBefore, errBefore := findTerminalFormula(beforeFormulas)
	require.NoError(t, errBefore)
	require.NotNil(t, termBefore)

	termAfter, errAfter := findTerminalFormula(afterFormulas)
	require.NoError(t, errAfter)
	require.NotNil(t, termAfter)

	outBefore, err := ComputeProduct(context.Background(), yarnTerminalInput(false))
	require.NoError(t, err)
	require.NotNil(t, outBefore)

	outAfter, err := ComputeProduct(context.Background(), yarnTerminalInput(true))
	require.NoError(t, err)
	require.NotNil(t, outAfter)

	// (b) terminal identity — reported first because (a) is only meaningful
	// while the same formula is being read out of scope on both sides.
	assert.Equal(t, termBefore.FormulaCode, termAfter.FormulaCode,
		"terminal formula moved: before=%s (result %s) after=%s (result %s)",
		termBefore.FormulaCode, termBefore.ResultParamCode,
		termAfter.FormulaCode, termAfter.ResultParamCode)
	assert.Equal(t, termBefore.ResultParamCode, termAfter.ResultParamCode,
		"terminal result param moved")

	// (a) numeric identity. OIL_GAIN evaluates to the literal 0 (000408:28), so
	// appending '+ OIL_GAIN' must be arithmetically inert.
	assert.LessOrEqual(t, math.Abs(outBefore.CostPerUnit-outAfter.CostPerUnit), 1e-12,
		"cost per unit changed: before=%.17g after=%.17g", outBefore.CostPerUnit, outAfter.CostPerUnit)

	// The computed value must be a real number, not the degenerate 0 that a
	// mis-built fixture would silently produce on both sides.
	require.NotZero(t, outBefore.CostPerUnit, "fixture produced no cost — it would make (a) vacuous")

	// OIL_GAIN is still evaluated in both passes; only its consumption changes.
	assert.InDelta(t, 0.0, outBefore.ParamSnapshot["OIL_GAIN"], 1e-12)
	assert.InDelta(t, 0.0, outAfter.ParamSnapshot["OIL_GAIN"], 1e-12)
}
