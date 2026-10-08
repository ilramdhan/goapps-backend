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
	capPackExprV561 = "IS_POY == 1 ? CAP_PACK_POY_DEFAULT : (CAPTIVE_BOX_WT > 0 ? (CAPTIVE_NO_OF_BOB * CAPTIVE_BOB_RATE + CAPTIVE_BOX_RATE) / CAPTIVE_BOX_WT : 0)"
	capPackOldExpr  = "CAPTIVE_BOX_WT > 0 ? (CAPTIVE_NO_OF_BOB * CAPTIVE_BOB_RATE + CAPTIVE_BOX_RATE) / CAPTIVE_BOX_WT : 0"
)

// TestMigration000561_ExpressionsAndEdges pins the migration text to the
// expressions the compute test below models (database-free text check).
func TestMigration000561_ExpressionsAndEdges(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/postgres/000561_cap_pack_poy_default.up.sql")
	require.NoError(t, err)
	sql := string(raw)
	assert.Contains(t, sql, "'"+capPackExprV561+"'")
	assert.Contains(t, sql, "expression = '"+capPackOldExpr+"'")
	assert.Contains(t, sql, "'F_YARN_CAP_PACK_POY_DEFAULT', 'Cap-Pack Cost POY Default', 'CONSTANT', '0.0078'")
	assert.Contains(t, sql, "'CAP_PACK_POY_DEFAULT'")

	down, err := os.ReadFile("../../../migrations/postgres/000561_cap_pack_poy_default.down.sql")
	require.NoError(t, err)
	assert.Contains(t, string(down), "SET expression  = '"+capPackOldExpr+"'")
}

func TestComputeProduct_CapPackPOYDefault(t *testing.T) {
	run := func(oil *OilInput) float64 {
		out, err := ComputeProduct(context.Background(), ComputeInput{
			ProductSysID: 7003, Period: "202604", CalcType: costcalcdom.CalcTypeActual,
			Route: buildOneStageRoute(7003, costroute.RmTypeItem, "RM_YARN", 1.0),
			Oil:   oil,
			CAPP: map[string]float64{
				"CAPTIVE_NO_OF_BOB": 10, "CAPTIVE_BOB_RATE": 0.5, "CAPTIVE_BOX_RATE": 1.0, "CAPTIVE_BOX_WT": 20,
			},
			Formulas: []Formula{
				{FormulaCode: "F_YARN_CAP_PACK_POY_DEFAULT", FormulaType: "CONSTANT", Expression: "0.0078", ResultParamCode: "CAP_PACK_POY_DEFAULT", SortOrder: 0},
				{
					FormulaCode: "F_YARN_CAP_PACK", FormulaType: "CALCULATION", Expression: capPackExprV561,
					ResultParamCode: "CAPTIVE_PACK_COST", SortOrder: 1,
					InputParamCodes: []string{"CAPTIVE_NO_OF_BOB", "CAPTIVE_BOB_RATE", "CAPTIVE_BOX_RATE", "CAPTIVE_BOX_WT", "CAP_PACK_POY_DEFAULT"},
				},
			},
			RMCosts:   map[string]RMCostRates{"RM_YARN|": {CostVal: 50.0}, "OILG|": {CrRate: 2.0}},
			EvalCache: evaluator.NewCache(),
		})
		require.NoError(t, err)
		return out.ParamSnapshot["CAPTIVE_PACK_COST"]
	}
	assert.InDelta(t, 0.0078, run(&OilInput{Class: OilClassPOY, TypeCode: "POY", GroupCode: "OILG"}), 1e-12, "POY uses the default regardless of box inputs")
	assert.InDelta(t, (10*0.5+1.0)/20.0, run(&OilInput{Class: OilClassPTY, TypeCode: "PTY", GroupCode: "OILG"}), 1e-12, "PTY keeps the formula")
	assert.InDelta(t, (10*0.5+1.0)/20.0, run(nil), 1e-12, "no oil class keeps the formula")
}
