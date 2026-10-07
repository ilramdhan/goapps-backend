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
	convCapExprV560 = "TOTAL_FIXEDCOST_PER_KG + CAPTIVE_PACK_COST + WASTE_LESS_MB_OPU + HEATSET_COST_PER_KG + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + SPECIAL_COST_2 + STEAM_COST_CNG + SOFTNER_COST + WASHING_COST + OIL_GAIN"
	convDelExprV560 = "TOTAL_FIXEDCOST_PER_KG + DELIVERY_PACK_COST + WASTE_LESS_MB_OPU + HEATSET_COST_PER_KG + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + SPECIAL_COST_2 + STEAM_COST_CNG + SOFTNER_COST + WASHING_COST + OIL_GAIN"
)

var convNewTermsV560 = []string{"WASTE_LESS_MB_OPU", "HEATSET_COST_PER_KG", "SPECIAL_COST_2", "STEAM_COST_CNG", "SOFTNER_COST", "WASHING_COST"}

// TestMigration000560_ExpressionsAndEdges pins the migration text to the
// expressions the compute test below models (database-free text check).
func TestMigration000560_ExpressionsAndEdges(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/postgres/000560_conv_ex_mb_add_legacy_terms.up.sql")
	require.NoError(t, err)
	sql := string(raw)
	assert.Contains(t, sql, "'"+convCapExprV560+"'")
	assert.Contains(t, sql, "'"+convDelExprV560+"'")
	for _, c := range convNewTermsV560 {
		assert.Contains(t, sql, "'"+c+"'", "edge for %s", c)
	}
}

// TestComputeProduct_ConvExMBLegacyTerms checks that the six legacy terms
// (000560) each contribute to the conversion result (legacy CSV row 92).
func TestComputeProduct_ConvExMBLegacyTerms(t *testing.T) {
	base := map[string]float64{
		"TOTAL_FIXEDCOST_PER_KG": 0.10, "CAPTIVE_PACK_COST": 0.20, "OIL_COST": 0.03,
		"INTERMINGLING": 0.04, "SPECIAL_COST_1": 0.05, "OIL_GAIN": -0.01,
	}
	inputs := []string{"TOTAL_FIXEDCOST_PER_KG", "CAPTIVE_PACK_COST", "OIL_COST", "INTERMINGLING", "SPECIAL_COST_1", "OIL_GAIN"}
	inputs = append(inputs, convNewTermsV560...)
	newVals := map[string]float64{
		"WASTE_LESS_MB_OPU": 0.5, "HEATSET_COST_PER_KG": 0.07, "SPECIAL_COST_2": 0.06,
		"STEAM_COST_CNG": 0, "SOFTNER_COST": 0.02, "WASHING_COST": 0,
	}
	run := func(extra map[string]float64) float64 {
		capp := map[string]float64{}
		for k, v := range base {
			capp[k] = v
		}
		for k, v := range extra {
			capp[k] = v
		}
		out, err := ComputeProduct(context.Background(), ComputeInput{
			ProductSysID: 7002, Period: "202604", CalcType: costcalcdom.CalcTypeActual,
			Route:     buildOneStageRoute(7002, costroute.RmTypeItem, "RM_YARN", 1.0),
			CAPP:      capp,
			Formulas:  []Formula{{FormulaCode: "F_YARN_CONV_CAP", FormulaType: "CALCULATION", Expression: convCapExprV560, ResultParamCode: "ONLY_CONV_CAP_PACK_EXCL_MB", InputParamCodes: inputs}},
			RMCosts:   map[string]RMCostRates{"RM_YARN|": {CostVal: 50.0}},
			EvalCache: evaluator.NewCache(),
		})
		require.NoError(t, err)
		return out.ParamSnapshot["ONLY_CONV_CAP_PACK_EXCL_MB"]
	}
	assert.InDelta(t, 0.10+0.20+0.03+0.04+0.05-0.01, run(nil), 1e-9, "unset new terms zero-fill")
	assert.InDelta(t, 0.41+0.5+0.07+0.06+0.02, run(newVals), 1e-9, "new terms contribute")
}
