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

func conversionBaseFormulas() []Formula {
	return []Formula{
		{FormulaCode: "F_CAP_QL", Expression: "COST_RM_TOTAL", ResultParamCode: "CAPTIVE_COST_QLTY_LOSS", InputParamCodes: []string{ScopeKeyCostRMTotal}},
		{FormulaCode: "F_DEL_QL", Expression: "CAPTIVE_COST_QLTY_LOSS * 1.5", ResultParamCode: "DELIVERY_COST_QLTY_LOSS", InputParamCodes: []string{"CAPTIVE_COST_QLTY_LOSS"}},
		{FormulaCode: "F_HELPER", Expression: "COST_RM_TOTAL", ResultParamCode: "OUT_HELPER", InputParamCodes: []string{ScopeKeyCostRMTotal}},
	}
}

func conversionInfoFormulas() []Formula {
	return []Formula{
		{FormulaCode: "F_YARN_CAP_CONV_STAGE", Expression: "CAPTIVE_COST_QLTY_LOSS - RM_RATE + VOLUME_BUCKET_4_LOSS", ResultParamCode: "CAP_CONV_STAGE", InputParamCodes: []string{"CAPTIVE_COST_QLTY_LOSS", "RM_RATE", "VOLUME_BUCKET_4_LOSS"}},
		{FormulaCode: "F_YARN_DEL_CONV_STAGE", Expression: "DELIVERY_COST_QLTY_LOSS - RM_RATE + VOLUME_BUCKET_4_LOSS", ResultParamCode: "DEL_CONV_STAGE", InputParamCodes: []string{"DELIVERY_COST_QLTY_LOSS", "RM_RATE", "VOLUME_BUCKET_4_LOSS"}},
		{FormulaCode: "F_YARN_TOTAL_CAP_CONV", Expression: "CAP_CONV_STAGE + UPSTREAM_TOTAL_CAP_CONV", ResultParamCode: "TOTAL_CAP_CONV", InputParamCodes: []string{"CAP_CONV_STAGE", "UPSTREAM_TOTAL_CAP_CONV"}},
		{FormulaCode: "F_YARN_TOTAL_DEL_CONV", Expression: "DEL_CONV_STAGE + UPSTREAM_TOTAL_CAP_CONV", ResultParamCode: "TOTAL_DEL_CONV", InputParamCodes: []string{"DEL_CONV_STAGE", "UPSTREAM_TOTAL_CAP_CONV"}},
	}
}

func conversionInput(formulas []Formula) ComputeInput {
	return ComputeInput{
		ProductSysID: 1,
		Route:        buildOneStageRoute(1, costroute.RmTypeItem, "X", 1.0),
		CAPP:         map[string]float64{"RM_RATE": 4, "VOLUME_BUCKET_4_LOSS": 1, "UPSTREAM_TOTAL_CAP_CONV": 2},
		Formulas:     formulas,
		RMCosts:      map[string]RMCostRates{"X|": {CostVal: 10.0}},
		EvalCache:    evaluator.NewCache(),
	}
}

func TestResolveFinalCost_InformationalConversionFormulasInvisible(t *testing.T) {
	base := conversionBaseFormulas()
	withConv := append(append([]Formula{}, base...), conversionInfoFormulas()...)

	outBase, err := ComputeProduct(context.Background(), conversionInput(base))
	require.NoError(t, err)
	// deepest base terminal = DELIVERY_COST_QLTY_LOSS = 10 * 1.5
	require.InDelta(t, 15.0, outBase.CostPerUnit, 1e-9)

	outConv, err := ComputeProduct(context.Background(), conversionInput(withConv))
	require.NoError(t, err)
	assert.InDelta(t, outBase.CostPerUnit, outConv.CostPerUnit, 1e-9, "conversion formulas must not change inferred final cost")
	assert.Contains(t, outConv.ParamSnapshot, "TOTAL_DEL_CONV") // still computed
}

func TestResolveFinalCost_OnlyConversionFormulas_MissingFinalCost(t *testing.T) {
	in := conversionInput(conversionInfoFormulas())
	in.CAPP["CAPTIVE_COST_QLTY_LOSS"] = 1
	in.CAPP["DELIVERY_COST_QLTY_LOSS"] = 1
	_, err := resolveFinalCost(in, map[string]any{}, 10)
	require.ErrorIs(t, err, costcalcdom.ErrMissingFinalCostFormula)
}
