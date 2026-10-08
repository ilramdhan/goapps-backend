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

const (
	capPackExprV564 = "IS_POY == 1 ? CAP_PACK_POY_DEFAULT : (CAPTIVE_BOX_WT > 0 ? (IS_ACTUAL == 1 ? (CAPTIVE_NO_OF_BOB * CAP_BOB_RATE_VAL + CAP_BOX_RATE_VAL) : (CAPTIVE_NO_OF_BOB * CAPTIVE_BOB_RATE + CAPTIVE_BOX_RATE)) / CAPTIVE_BOX_WT : 0)"
	delPackExprV564 = "DELIVERY_BOX_WT > 0 ? (IS_ACTUAL == 1 ? (DELIVERY_NO_OF_BOB * DELIVERY_BOB_RATE + DELIVERY_BOX_RATE) : (DELIVERY_NO_OF_BOB * DEL_BOB_RATE_MKT + DEL_BOX_RATE_MKT)) / DELIVERY_BOX_WT : 0"
)

func TestComputeProduct_PackRatesByCalcType(t *testing.T) {
	run := func(ct costcalcdom.CalculationType, oil *OilInput) map[string]float64 {
		out, err := ComputeProduct(context.Background(), ComputeInput{
			ProductSysID: 7010, Period: "202604", CalcType: ct, Oil: oil,
			Route: buildOneStageRoute(7010, costroute.RmTypeItem, "RM_YARN", 1.0),
			CAPP: map[string]float64{
				"CAPTIVE_NO_OF_BOB": 10, "CAPTIVE_BOX_WT": 20, "CAPTIVE_BOB_RATE": 0.5, "CAPTIVE_BOX_RATE": 1.0,
				"CAP_BOB_RATE_VAL": 0.3, "CAP_BOX_RATE_VAL": 0.8,
				"DELIVERY_NO_OF_BOB": 10, "DELIVERY_BOX_WT": 20, "DELIVERY_BOB_RATE": 0.4, "DELIVERY_BOX_RATE": 0.9,
				"DEL_BOB_RATE_MKT": 0.6, "DEL_BOX_RATE_MKT": 1.2,
			},
			Formulas: []Formula{
				{FormulaCode: "F_YARN_CAP_PACK_POY_DEFAULT", FormulaType: "CONSTANT", Expression: "0.0078", ResultParamCode: "CAP_PACK_POY_DEFAULT", SortOrder: 0},
				{FormulaCode: "F_YARN_CAP_PACK", FormulaType: "CALCULATION", Expression: capPackExprV564, ResultParamCode: "CAPTIVE_PACK_COST", SortOrder: 1,
					InputParamCodes: []string{"CAPTIVE_NO_OF_BOB", "CAPTIVE_BOB_RATE", "CAPTIVE_BOX_RATE", "CAPTIVE_BOX_WT", "CAP_PACK_POY_DEFAULT", "CAP_BOB_RATE_VAL", "CAP_BOX_RATE_VAL"}},
				{FormulaCode: "F_YARN_DEL_PACK", FormulaType: "CALCULATION", Expression: delPackExprV564, ResultParamCode: "DELIVERY_PACK_COST", SortOrder: 2,
					InputParamCodes: []string{"DELIVERY_NO_OF_BOB", "DELIVERY_BOB_RATE", "DELIVERY_BOX_RATE", "DELIVERY_BOX_WT", "DEL_BOB_RATE_MKT", "DEL_BOX_RATE_MKT"}},
			},
			RMCosts:   map[string]RMCostRates{"RM_YARN|": {CostVal: 50.0}, "OILG|": {CrRate: 2.0}},
			EvalCache: evaluator.NewCache(), // shared across calc types on purpose
		})
		require.NoError(t, err)
		return out.ParamSnapshot
	}
	act := run(costcalcdom.CalcTypeActual, nil)
	assert.InDelta(t, (10*0.3+0.8)/20, act["CAPTIVE_PACK_COST"], 1e-12)
	assert.InDelta(t, (10*0.4+0.9)/20, act["DELIVERY_PACK_COST"], 1e-12)
	assert.InDelta(t, 1.0, act["IS_ACTUAL"], 0)
	for _, ct := range []costcalcdom.CalculationType{costcalcdom.CalcTypeForecast, costcalcdom.CalcTypeSelling} {
		m := run(ct, nil)
		assert.InDelta(t, (10*0.5+1.0)/20, m["CAPTIVE_PACK_COST"], 1e-12, string(ct))
		assert.InDelta(t, (10*0.6+1.2)/20, m["DELIVERY_PACK_COST"], 1e-12, string(ct))
		assert.InDelta(t, 0.0, m["IS_ACTUAL"], 0)
	}
	// POY keeps the 000561 default in every calc type: the IS_POY branch wins
	// before IS_ACTUAL is consulted. Delivery is unaffected by IS_POY.
	poy := &OilInput{Class: OilClassPOY, TypeCode: "POY", GroupCode: "OILG"}
	for _, ct := range []costcalcdom.CalculationType{costcalcdom.CalcTypeActual, costcalcdom.CalcTypeForecast, costcalcdom.CalcTypeSelling} {
		m := run(ct, poy)
		assert.InDelta(t, 0.0078, m["CAPTIVE_PACK_COST"], 1e-12, "POY "+string(ct))
	}
	assert.InDelta(t, (10*0.4+0.9)/20, run(costcalcdom.CalcTypeActual, poy)["DELIVERY_PACK_COST"], 1e-12)
	assert.InDelta(t, (10*0.6+1.2)/20, run(costcalcdom.CalcTypeForecast, poy)["DELIVERY_PACK_COST"], 1e-12)
}

// TestMigration000564_ExpressionsPinned pins the migration text to the expressions modelled above.
func TestMigration000564_ExpressionsPinned(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/postgres/000564_pack_rates_by_calc_type.up.sql")
	require.NoError(t, err)
	sql := string(raw)
	assert.Contains(t, sql, "'"+capPackExprV564+"'")
	assert.Contains(t, sql, "'"+delPackExprV564+"'")
	assert.Contains(t, sql, "'DELIVERY_BOX_WT > 0 ? ((DELIVERY_NO_OF_BOB * DELIVERY_BOB_RATE) + DELIVERY_BOX_RATE) / DELIVERY_BOX_WT : 0'")
	assert.Contains(t, sql, "'DELIVERY_BOX_WT > 0 ? (DELIVERY_NO_OF_BOB * DELIVERY_BOB_RATE + DELIVERY_BOX_RATE) / DELIVERY_BOX_WT : 0'")
}
