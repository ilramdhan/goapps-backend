package costcalc

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// mbCostExprV567 is F_YARN_MB_COST as migration 000567 writes it.
const mbCostExprV567 = "IS_SUPERBA == 1 ? SUPERBA_MB_COST : (MB_RATE_MKT * MB_SP_DOZING / 100.0)"

func superbaCompute(ct costcalcdom.CalculationType, oil *OilInput, sb *SuperbaCost) (*ComputeOutput, error) {
	return ComputeProduct(context.Background(), ComputeInput{
		ProductSysID: 8010, Period: "202604", CalcType: ct, Oil: oil, Superba: sb,
		Route:   buildOneStageRoute(8010, costroute.RmTypeItem, "RM_YARN", 1.0),
		CAPP:    map[string]float64{"MB_RATE_MKT": 50, "MB_SP_DOZING": 4, "OIL_RATE": 5, "OPU": 2},
		RMCosts: map[string]RMCostRates{"RM_YARN|": {CostVal: 10}, "OILG|": {CrRate: 2.0}},
		Formulas: []Formula{{FormulaCode: "F_YARN_MB_COST", FormulaType: "CALCULATION", Expression: mbCostExprV567,
			ResultParamCode: "MB_COST_MKT", SortOrder: 1, InputParamCodes: []string{"MB_RATE_MKT", "MB_SP_DOZING"}}},
		EvalCache: evaluator.NewCache(),
	})
}

var sbTestOil = &OilInput{Class: OilClassSuperba, TypeCode: "TCS", GroupCode: "OILG"}

func TestApplySuperbaMBCost_SuperbaUsesOldValueAllCalcTypes(t *testing.T) {
	sb := &SuperbaCost{ShadeCode: "SP1", Found: true, OldValue: 0.08, LegacySysID: 9}
	for _, ct := range []costcalcdom.CalculationType{costcalcdom.CalcTypeActual, costcalcdom.CalcTypeForecast, costcalcdom.CalcTypeSelling} {
		out, err := superbaCompute(ct, sbTestOil, sb)
		require.NoError(t, err, string(ct))
		assert.InDelta(t, 0.08, out.ParamSnapshot["MB_COST_MKT"], 1e-12, string(ct))
		assert.InDelta(t, 0.08, out.ParamSnapshot[ScopeKeySuperbaMBCost], 1e-12, "recorded in snapshot "+string(ct))
	}
}

func TestApplySuperbaMBCost_MissingBlocks(t *testing.T) {
	for name, sb := range map[string]*SuperbaCost{
		"nil":       nil,
		"not found": {ShadeCode: "NOPE", Found: false},
	} {
		_, err := superbaCompute(costcalcdom.CalcTypeForecast, sbTestOil, sb)
		require.Error(t, err, name)
		assert.ErrorIs(t, err, costcalcdom.ErrMissingSuperbaCost, name)
	}
	_, err := superbaCompute(costcalcdom.CalcTypeForecast, sbTestOil, &SuperbaCost{ShadeCode: "NOPE"})
	assert.ErrorContains(t, err, `"NOPE"`, "shade code in message")
}

func TestApplySuperbaMBCost_NonSuperbaUnchanged(t *testing.T) {
	cases := map[string]*OilInput{
		"no oil": nil,
		"PTY":    {Class: OilClassPTY, TypeCode: "PTY", GroupCode: "OILG"},
		"POY":    {Class: OilClassPOY, TypeCode: "POY", GroupCode: "OILG"},
	}
	for name, oil := range cases {
		// A stray Superba value must be ignored for non-SUPERBA products.
		out, err := superbaCompute(costcalcdom.CalcTypeForecast, oil, &SuperbaCost{Found: true, OldValue: 99})
		require.NoError(t, err, name)
		assert.InDelta(t, 50*4/100.0, out.ParamSnapshot["MB_COST_MKT"], 1e-12, name)
		_, inSnap := out.ParamSnapshot[ScopeKeySuperbaMBCost]
		assert.False(t, inSnap, name+": key kept out of snapshot")
		// mbbatch path / no lookup result: Oil nil, Superba nil, no block.
		out, err = superbaCompute(costcalcdom.CalcTypeActual, oil, nil)
		require.NoError(t, err, name)
		assert.InDelta(t, 2.0, out.ParamSnapshot["MB_COST_MKT"], 1e-12, name)
	}
}

func TestApplySuperbaMBCost_Direct(t *testing.T) {
	scope := map[string]any{}
	zf := map[string]bool{}
	require.NoError(t, applySuperbaMBCost(ComputeInput{}, scope, zf))
	assert.Equal(t, float64(0), scope[ScopeKeySuperbaMBCost])
	assert.True(t, zf[ScopeKeySuperbaMBCost])
}

// TestMigration000567_MatchesFixture keeps mbCostExprV567 linked to the text
// migration 000567 writes and guards on (parsed as text, never executed).
func TestMigration000567_MatchesFixture(t *testing.T) {
	const dir = "../../../migrations/postgres/000567_superba_mb_cost_formula."
	rawUp, err := os.ReadFile(dir + "up.sql")
	require.NoError(t, err)
	rawDown, err := os.ReadFile(dir + "down.sql")
	require.NoError(t, err)
	up, down := stripSQLComments(string(rawUp)), stripSQLComments(string(rawDown))

	const orig = "MB_RATE_MKT * MB_SP_DOZING / 100.0"
	assert.Contains(t, up, "SET expression = '"+mbCostExprV567+"'")
	assert.Contains(t, up, "AND f.expression = '"+orig+"'", "guarded on the exact 000408 text")
	assert.Contains(t, up, "'superba_mb_cost_000567'")
	assert.Contains(t, up, "RAISE NOTICE")
	assert.NotContains(t, up, "RAISE EXCEPTION", "0 rows must warn, not fail")
	assert.NotContains(t, up, "INSERT INTO formula_param", "reserved keys need no edge")
	assert.Contains(t, down, "SET expression = '"+orig+"'")
	assert.Contains(t, down, "f.updated_by = 'superba_mb_cost_000567'")
	assert.Contains(t, down, "f.expression = '"+mbCostExprV567+"'")
}

// mbCostExprV568 is F_YARN_MB_COST as migration 000568 writes it (prod's waste-adjusted arm).
const mbCostExprV568 = "IS_SUPERBA == 1 ? SUPERBA_MB_COST : (((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0)"

func TestMigration000568_MatchesFixture(t *testing.T) {
	const dir = "../../../migrations/postgres/000568_superba_mb_cost_formula_prod_text."
	rawUp, err := os.ReadFile(dir + "up.sql")
	require.NoError(t, err)
	rawDown, err := os.ReadFile(dir + "down.sql")
	require.NoError(t, err)
	up, down := stripSQLComments(string(rawUp)), stripSQLComments(string(rawDown))

	const prod = "((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0"
	assert.Contains(t, up, "v_target  CONSTANT TEXT := '"+mbCostExprV568+"'")
	assert.Contains(t, up, "f.expression = '"+prod+"'", "case A: prod web-edited text")
	assert.Contains(t, up, "f.expression = 'IS_SUPERBA == 1 ? SUPERBA_MB_COST : (MB_RATE_MKT * MB_SP_DOZING / 100.0)'", "case B: 000567 output")
	assert.Contains(t, up, "f.updated_by = 'superba_mb_cost_000567'")
	assert.Contains(t, up, "'superba_mb_cost_000568'")
	assert.Contains(t, up, "p.param_code = 'MB_COST_MKT'")
	assert.Contains(t, up, "bak_000568_formula")
	assert.Contains(t, up, "INSERT INTO formula_param")
	assert.Contains(t, up, "NOT EXISTS (SELECT 1 FROM formula_param fp")
	assert.Contains(t, up, "RAISE NOTICE")
	assert.NotContains(t, up, "RAISE EXCEPTION", "0 rows must warn, not fail")
	assert.Contains(t, down, "f.updated_by = 'superba_mb_cost_000568'")
	assert.Contains(t, down, "f.expression = '"+mbCostExprV568+"'")
	assert.Contains(t, down, "b.edge_inserted")
	assert.Contains(t, down, "DROP TABLE IF EXISTS bak_000568_formula")
}

func TestApplySuperbaMBCost_ProdWasteAdjustedArm(t *testing.T) {
	run := func(oil *OilInput, sb *SuperbaCost) *ComputeOutput {
		out, err := ComputeProduct(context.Background(), ComputeInput{
			ProductSysID: 8011, Period: "202604", CalcType: costcalcdom.CalcTypeForecast, Oil: oil, Superba: sb,
			Route:   buildOneStageRoute(8011, costroute.RmTypeItem, "RM_YARN", 1.0),
			CAPP:    map[string]float64{"MB_RATE_MKT": 50, "MB_SP_DOZING": 4, "WASTE_PERC": 0.7, "OIL_RATE": 5, "OPU": 2},
			RMCosts: map[string]RMCostRates{"RM_YARN|": {CostVal: 10}, "OILG|": {CrRate: 2.0}},
			Formulas: []Formula{{FormulaCode: "F_YARN_MB_COST", FormulaType: "CALCULATION", Expression: mbCostExprV568,
				ResultParamCode: "MB_COST_MKT", SortOrder: 1, InputParamCodes: []string{"MB_RATE_MKT", "MB_SP_DOZING", "WASTE_PERC"}}},
			EvalCache: evaluator.NewCache(),
		})
		require.NoError(t, err)
		return out
	}
	sb := &SuperbaCost{ShadeCode: "SP1", Found: true, OldValue: 0.08, LegacySysID: 9}
	assert.InDelta(t, 0.08, run(sbTestOil, sb).ParamSnapshot["MB_COST_MKT"], 1e-12, "SUPERBA -> SUPERBA_MB_COST")
	want := (1 + 0.7) * 4 * 50 / 100.0
	assert.InDelta(t, want, run(&OilInput{Class: OilClassPTY, TypeCode: "PTY", GroupCode: "OILG"}, nil).ParamSnapshot["MB_COST_MKT"], 1e-9, "non-SUPERBA waste-adjusted")
}
