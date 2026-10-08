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

func TestMissingConstantInputs(t *testing.T) {
	fs := []Formula{
		{ResultParamCode: "CAPTIVE_PACK_COST", InputParamCodes: []string{"CAP_PACK_POY_DEFAULT", "CAPTIVE_BOX_WT", "BOB"}},
		{ResultParamCode: "BOB", InputParamCodes: []string{"CAPTIVE_BOX_WT"}},
	}
	// BOB is produced; CAPTIVE_BOX_WT is attached; only the default is missing.
	got := missingConstantInputs(fs, map[string]bool{"CAPTIVE_BOX_WT": true})
	assert.Equal(t, []string{"CAP_PACK_POY_DEFAULT"}, got)

	// CAPP wins: attached default is not re-added.
	got = missingConstantInputs(fs, map[string]bool{"CAP_PACK_POY_DEFAULT": true, "CAPTIVE_BOX_WT": true})
	assert.Empty(t, got)
}

// TestComputeProduct_AutoLoadedConstant models the loader output for a POY product
// whose CAPP lacks CAP_PACK_POY_DEFAULT: the CONSTANT is appended after the consumer
// and topoSortFormulas must order it first; the unreferenced CONSTANT is absent.
func TestComputeProduct_AutoLoadedConstant(t *testing.T) {
	loaded := []Formula{
		{
			FormulaCode: "F_YARN_CAP_PACK", FormulaType: "CALCULATION", Expression: capPackExprV561,
			ResultParamCode: "CAPTIVE_PACK_COST", SortOrder: 0,
			InputParamCodes: []string{"CAPTIVE_NO_OF_BOB", "CAPTIVE_BOB_RATE", "CAPTIVE_BOX_RATE", "CAPTIVE_BOX_WT", "CAP_PACK_POY_DEFAULT"},
		},
	}
	attached := map[string]bool{"CAPTIVE_NO_OF_BOB": true, "CAPTIVE_BOB_RATE": true, "CAPTIVE_BOX_RATE": true, "CAPTIVE_BOX_WT": true}
	missing := missingConstantInputs(loaded, attached)
	require.Equal(t, []string{"CAP_PACK_POY_DEFAULT"}, missing)

	available := map[string]Formula{
		"CAP_PACK_POY_DEFAULT": {FormulaCode: "F_YARN_CAP_PACK_POY_DEFAULT", FormulaType: "CONSTANT", Expression: "0.0078", ResultParamCode: "CAP_PACK_POY_DEFAULT"},
		"UNREFERENCED":         {FormulaCode: "F_UNREF", FormulaType: "CONSTANT", Expression: "9", ResultParamCode: "UNREFERENCED"},
	}
	for _, c := range missing {
		loaded = append(loaded, available[c])
	}
	sorted, err := topoSortFormulas(loaded)
	require.NoError(t, err)
	require.Len(t, sorted, 2)
	assert.Equal(t, "CAP_PACK_POY_DEFAULT", sorted[0].ResultParamCode)

	run := func(oil *OilInput) float64 {
		out, err := ComputeProduct(context.Background(), ComputeInput{
			ProductSysID: 7010, Period: "202604", CalcType: costcalcdom.CalcTypeActual,
			Route: buildOneStageRoute(7010, costroute.RmTypeItem, "RM_YARN", 1.0),
			Oil:   oil,
			CAPP: map[string]float64{
				"CAPTIVE_NO_OF_BOB": 10, "CAPTIVE_BOB_RATE": 0.5, "CAPTIVE_BOX_RATE": 1.0, "CAPTIVE_BOX_WT": 20,
			},
			Formulas:  sorted,
			RMCosts:   map[string]RMCostRates{"RM_YARN|": {CostVal: 50.0}, "OILG|": {CrRate: 2.0}},
			EvalCache: evaluator.NewCache(),
		})
		require.NoError(t, err)
		return out.ParamSnapshot["CAPTIVE_PACK_COST"]
	}
	assert.InDelta(t, 0.0078, run(&OilInput{Class: OilClassPOY, TypeCode: "POY", GroupCode: "OILG"}), 1e-12)
	assert.InDelta(t, (10*0.5+1.0)/20.0, run(&OilInput{Class: OilClassPTY, TypeCode: "PTY", GroupCode: "OILG"}), 1e-12, "non-POY unaffected")
}
