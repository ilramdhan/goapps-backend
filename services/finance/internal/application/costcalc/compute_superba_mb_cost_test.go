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

// mbCostExprV567 is F_YARN_MB_COST as migration 000567 writes it.
const mbCostExprV567 = "IS_SUPERBA == 1 ? SUPERBA_MB_COST : (MB_RATE_MKT * MB_SP_DOZING / 100.0)"

func superbaCompute(ct costcalcdom.CalculationType, oil *OilInput, sb *SuperbaCost) (*ComputeOutput, error) {
	return ComputeProduct(context.Background(), ComputeInput{
		ProductSysID: 8010, Period: "202604", CalcType: ct, Oil: oil, Superba: sb,
		Route:    buildOneStageRoute(8010, costroute.RmTypeItem, "RM_YARN", 1.0),
		CAPP:     map[string]float64{"MB_RATE_MKT": 50, "MB_SP_DOZING": 4, "OIL_RATE": 5, "OPU": 2},
		RMCosts:  map[string]RMCostRates{"RM_YARN|": {CostVal: 10}, "OILG|": {CrRate: 2.0}},
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
