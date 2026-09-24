package costcalc

import (
	"context"
	"math"
	"os"
	"strings"
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
		// 000524 — new CONSTANT formula feeding the POY arm of F_YARN_OIL_GAIN.
		// It is consumed by F_YARN_OIL_GAIN, so it never joins the terminal set.
		{
			FormulaCode:     "F_YARN_OIL_GAIN_POY_DEFAULT",
			FormulaName:     "Oil Gain POY Default",
			FormulaType:     "CONSTANT",
			Expression:      oilGainPOYDefault,
			ResultParamCode: "OIL_GAIN_POY_DEFAULT",
			SortOrder:       0,
		},
		// 000408:28 rewritten by 000524 — by-product-type expression with the
		// OPU / OIL_RATE / OIL_GAIN_POY_DEFAULT input edges 000524 adds. With no
		// Oil context every IS_* flag is 0, so it still evaluates to 0.
		{
			FormulaCode:     "F_YARN_OIL_GAIN",
			FormulaName:     "Oil Gain",
			FormulaType:     "CALCULATION",
			Expression:      oilGainExpr,
			ResultParamCode: "OIL_GAIN",
			InputParamCodes: []string{"OPU", "OIL_RATE", "OIL_GAIN_POY_DEFAULT"},
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
		RMCosts:      map[string]RMCostRates{"RM_YARN|": {CostVal: 50.0}},
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

	// (a) numeric identity. With no Oil context OIL_GAIN evaluates to 0 (every
	// IS_* flag is 0 under the 000524 expression), so appending '+ OIL_GAIN'
	// must be arithmetically inert.
	assert.LessOrEqual(t, math.Abs(outBefore.CostPerUnit-outAfter.CostPerUnit), 1e-12,
		"cost per unit changed: before=%.17g after=%.17g", outBefore.CostPerUnit, outAfter.CostPerUnit)

	// The computed value must be a real number, not the degenerate 0 that a
	// mis-built fixture would silently produce on both sides.
	require.NotZero(t, outBefore.CostPerUnit, "fixture produced no cost — it would make (a) vacuous")

	// OIL_GAIN is still evaluated in both passes; only its consumption changes.
	assert.InDelta(t, 0.0, outBefore.ParamSnapshot["OIL_GAIN"], 1e-12)
	assert.InDelta(t, 0.0, outAfter.ParamSnapshot["OIL_GAIN"], 1e-12)
}

// TestMigration000510_MatchesFixtureExpressions keeps the fixture above honest.
//
// Everything the two tests above prove is only meaningful if the "after" shape
// they model is the shape migration 000510 actually writes. Nothing else in the
// build links the two, so a later edit to the migration — a different operator,
// a renamed param, a dropped formula_param edge — would leave both tests green
// while the database diverges from what they cleared.
//
// This test closes that gap by reading the migration and asserting the exact
// post-change expression text and both input edges are present. It is still
// database-free: the file is parsed as text, never executed.
func TestMigration000510_MatchesFixtureExpressions(t *testing.T) {
	const migration = "../../../migrations/postgres/000510_wire_oil_gain_into_conversion_cost.up.sql"

	raw, err := os.ReadFile(migration)
	require.NoError(t, err, "migration 000510 must exist — the tests above describe its effect")
	sql := string(raw)

	// The fixture builds the post-change expressions by appending " + OIL_GAIN"
	// to the verbatim 000408 text, so deriving the expected strings from the
	// fixture (rather than retyping them) is what makes this a real link.
	after := yarnTerminalFormulas(true)
	byCode := make(map[string]Formula, len(after))
	for _, f := range after {
		byCode[f.FormulaCode] = f
	}

	for _, code := range []string{"F_YARN_CONV_CAP", "F_YARN_CONV_DEL"} {
		f, ok := byCode[code]
		require.True(t, ok, "fixture must define %s", code)

		assert.Contains(t, sql, "'"+f.Expression+"'",
			"migration 000510 must set %s to the expression the fixture models; "+
				"if the migration changed deliberately, update yarnTerminalFormulas to match",
			code)
		assert.Contains(t, sql, code,
			"migration 000510 must name %s", code)
	}

	// The expression alone is not enough: formula_param is what populates
	// InputParamCodes and therefore what removes OIL_GAIN from the terminal set
	// (compute.go:812-818). Assert the edge insert targets both formulas.
	assert.Contains(t, sql, "INSERT INTO formula_param",
		"migration 000510 must declare OIL_GAIN as an input edge, not only as expression text")
	assert.Contains(t, sql, "'OIL_GAIN'",
		"migration 000510 must reference the OIL_GAIN param")

	// Sign guard. OIL_GAIN is stored already-negative (see the migration header
	// and data-examples/import-file-csv/param_value_import/
	// product_parameters_1.csv:101, OIL_GAIN = -0.0495), so the term must be
	// ADDED. A '- OIL_GAIN' would invert the correction: it would raise
	// conversion cost by the oil gain instead of lowering it.
	//
	// Comment lines are stripped first — the migration header discusses the
	// rejected '- OIL_GAIN' form in prose, and only executable SQL should be
	// judged here.
	assert.NotContains(t, stripSQLComments(sql), "- OIL_GAIN",
		"OIL_GAIN values are stored negative, so the term must be added, not subtracted")
}

// stripSQLComments removes whole-line and trailing "--" comments so a guard can
// assert on executable SQL without matching the file's own prose.
func stripSQLComments(sql string) string {
	lines := strings.Split(sql, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// TestComputeProduct_PTYOilGain_TerminalUnchanged runs the 000510 "after"
// shape for a PTY product whose OIL_GAIN is genuinely non-zero under 000524.
// The terminal formula must be the same one the no-oil pass selects, and the
// cost must move by exactly OIL_GAIN (it is stored negative and ADDED into
// ONLY_CONV_DEL_PACK_EXCL_MB, which reaches every VB*_DEL terminal once).
func TestComputeProduct_PTYOilGain_TerminalUnchanged(t *testing.T) {
	noOil := yarnTerminalInput(true)
	noOil.CAPP["OPU"] = 2.2
	outNoOil, err := ComputeProduct(context.Background(), noOil)
	require.NoError(t, err)

	pty := yarnTerminalInput(true)
	pty.CAPP["OPU"] = 2.2
	pty.RMCosts["202006101|"] = RMCostRates{CrRate: 2.2869}
	pty.Oil = &OilInput{
		Class: OilClassPTY, TypeCode: "PTY", GroupCode: "202006101",
		DefaultGroup: "202006101", Allowed: map[string]bool{"202006101": true},
	}
	outPTY, err := ComputeProduct(context.Background(), pty)
	require.NoError(t, err)

	term, err := findTerminalFormula(pty.Formulas)
	require.NoError(t, err)
	termNoOil, err := findTerminalFormula(noOil.Formulas)
	require.NoError(t, err)
	assert.Equal(t, termNoOil.FormulaCode, term.FormulaCode, "oil wiring must not move the terminal formula")

	gain := outPTY.ParamSnapshot["OIL_GAIN"]
	assert.InDelta(t, -0.0503118, gain, 1e-9, "PTY OIL_GAIN = -(OPU*OIL_RATE)/100")
	assert.InDelta(t, outNoOil.CostPerUnit+gain, outPTY.CostPerUnit, 1e-9,
		"cost per unit must move by exactly OIL_GAIN")
}
