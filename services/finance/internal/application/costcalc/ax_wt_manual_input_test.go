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

// axWtFromMarketingFormula is the 000408 F_YARN_AX_WT_FROM_MKT row (SQL
// quoting removed). 000534 deactivates it; the engine guard must still keep
// AX_WT on the CAPP value if it is ever active again.
func axWtFromMarketingFormula() Formula {
	return Formula{
		FormulaCode:     "F_YARN_AX_WT_FROM_MKT",
		FormulaType:     "FROM_MARKETING",
		Expression:      "marketing_result(product, \"AX_WT\", period)",
		ResultParamCode: "AX_WT",
	}
}

// TestComputeProduct_AXWT_ManualInputBeatsSellingSnapshot: CAPP AX_WT=10,
// SELLING snapshot AX_WT=7. AX_WT must stay 10 in every calc type, and the
// grade weights AE..C must be computed from 10.
func TestComputeProduct_AXWT_ManualInputBeatsSellingSnapshot(t *testing.T) {
	capp := map[string]float64{
		"AX_WT": 10, "AX_PERC": 80,
		"AE_PERC": 4, "A9_PERC": 2, "A_PERC": 8, "B_PERC": 4, "C_PERC": 2,
	}
	// AX_WT * <G>_PERC / AX_PERC with AX_WT=10 (no tx_weight rules -> fallback).
	want := map[string]float64{"AE": 0.5, "A9": 0.25, "A": 1.0, "B": 0.5, "C": 0.25}

	formulas := []Formula{axWtFromMarketingFormula()}
	for _, g := range txWeightGrades {
		formulas = append(formulas, Formula{
			FormulaCode:     "F_YARN_" + g + "_WT",
			FormulaType:     "CALCULATION",
			Expression:      txWeightExpression(g),
			ResultParamCode: g + "_WT",
			InputParamCodes: []string{"AX_WT", g + "_PERC", "AX_PERC"},
		})
	}

	for _, calcType := range []costcalcdom.CalculationType{
		costcalcdom.CalcTypeActual, costcalcdom.CalcTypeForecast, costcalcdom.CalcTypeSelling,
	} {
		t.Run(string(calcType), func(t *testing.T) {
			in := ComputeInput{
				ProductSysID:    77,
				Period:          "202609",
				CalcType:        calcType,
				Route:           buildOneStageRoute(77, costroute.RmTypeItem, "RM_X", 1.0),
				CAPP:            capp,
				Formulas:        formulas,
				RMCosts:         map[string]RMCostRates{"RM_X|": {CostVal: 100}},
				EvalCache:       evaluator.NewCache(),
				SellingSnapshot: map[string]float64{"AX_WT": 7, "CAPTIVE_NO_OF_BOB": 4.8},
			}
			out, err := ComputeProduct(context.Background(), in)
			require.NoError(t, err)
			assert.InDelta(t, 10.0, out.ParamSnapshot["AX_WT"], 1e-9, "AX_WT is the manual CAPP input, not the snapshot")
			for _, g := range txWeightGrades {
				assert.InDelta(t, want[g], out.ParamSnapshot[g+"_WT"], 1e-9, "%s_WT computed from AX_WT=10", g)
			}
		})
	}
}

// TestInjectMarketingResult_ManualInputOnly checks the built-in directly:
// AX_WT ignores the snapshot, other FROM_MARKETING params still use it.
func TestInjectMarketingResult_ManualInputOnly(t *testing.T) {
	scope := map[string]any{"AX_WT": 10.0, "CAPTIVE_NO_OF_BOB": 3.0}
	injectMarketingResult(scope, map[string]float64{"AX_WT": 7, "CAPTIVE_NO_OF_BOB": 4.8})
	fn, ok := scope["marketing_result"].(func(...any) (any, error))
	require.True(t, ok)

	got, err := fn(1, "AX_WT", "202609")
	require.NoError(t, err)
	assert.InDelta(t, 10.0, got, 1e-12, "AX_WT -> CAPP value")

	got, err = fn(1, "CAPTIVE_NO_OF_BOB", "202609")
	require.NoError(t, err)
	assert.InDelta(t, 4.8, got, 1e-12, "other params -> snapshot value")

	delete(scope, "AX_WT")
	got, err = fn(1, "AX_WT", "202609")
	require.NoError(t, err)
	assert.InDelta(t, 0.0, got, 1e-12, "AX_WT without CAPP -> 0, never the snapshot")
}

func readMigrationPair(t *testing.T, base string) (up, down string) {
	t.Helper()
	upRaw, err := os.ReadFile("../../../migrations/postgres/" + base + ".up.sql")
	require.NoError(t, err)
	downRaw, err := os.ReadFile("../../../migrations/postgres/" + base + ".down.sql")
	require.NoError(t, err)
	return stripSQLComments(string(upRaw)), stripSQLComments(string(downRaw))
}

// TestMigration000534_DeactivatesAXWTFromMarketing asserts 000534 deactivates
// exactly the 000408 row (guarded), backs it up and restores it on down.
func TestMigration000534_DeactivatesAXWTFromMarketing(t *testing.T) {
	up, down := readMigrationPair(t, "000534_deactivate_ax_wt_from_marketing_formula")

	assert.Contains(t, up, "UPDATE mst_formula")
	assert.Contains(t, up, "SET is_active  = FALSE")
	assert.Contains(t, up, "formula_code = 'F_YARN_AX_WT_FROM_MKT'")
	assert.Contains(t, up, "AND is_active = TRUE")
	assert.Contains(t, up, "AND formula_type = 'FROM_MARKETING'")
	assert.Contains(t, up, "AND expression = 'marketing_result(product,''AX_WT'',period)'")
	assert.Contains(t, up, "updated_by = 'ax_wt_input_000534'")
	assert.Contains(t, up, "INSERT INTO bak_ax_wt_formula_000534")
	assert.Contains(t, up, "ON CONFLICT (formula_code) DO NOTHING")
	assert.Contains(t, up, "RAISE NOTICE")
	assert.NotContains(t, up, "deleted_at = NOW()", "deactivate, not soft-delete")
	assert.NotContains(t, up, "CAPTIVE_NO_OF_BOB", "other FROM_MARKETING formulas untouched")

	assert.Contains(t, down, "SET is_active  = b.is_active")
	assert.Contains(t, down, "FROM bak_ax_wt_formula_000534 b")
	assert.Contains(t, down, "AND f.updated_by = 'ax_wt_input_000534'")
	assert.Contains(t, down, "DROP TABLE IF EXISTS bak_ax_wt_formula_000534")
	for _, s := range []string{up, down} {
		assert.Contains(t, s, "BEGIN;")
		assert.Contains(t, s, "COMMIT;")
	}
}
