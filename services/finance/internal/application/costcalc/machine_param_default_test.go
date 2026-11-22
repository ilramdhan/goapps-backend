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

// machineDefaultFormulas: MB_COST_MKT from its formula (= 0.5 with the CAPP
// below), consumed by the final cost.
func machineDefaultFormulas() []Formula {
	return []Formula{
		{
			FormulaCode:     "F_YARN_MB_COST",
			FormulaType:     "CALCULATION",
			Expression:      "MB_SP_DOZING * MB_RATE_MKT / 100.0",
			ResultParamCode: "MB_COST_MKT",
			InputParamCodes: []string{"MB_SP_DOZING", "MB_RATE_MKT"},
		},
		{
			FormulaCode:     "F_STAGE_OUT",
			FormulaType:     "CALCULATION",
			Expression:      "COST_RM_TOTAL + MB_COST_MKT",
			ResultParamCode: ScopeKeyFinalCost,
			InputParamCodes: []string{ScopeKeyCostRMTotal, "MB_COST_MKT"},
		},
	}
}

func runMachineDefault(t *testing.T, capp, defaults map[string]float64) *ComputeOutput {
	t.Helper()
	out, err := ComputeProduct(context.Background(), ComputeInput{
		ProductSysID:    7,
		CalcType:        costcalcdom.CalcTypeActual,
		Route:           buildOneStageRoute(7, costroute.RmTypeItem, "RM1", 1.0),
		CAPP:            capp,
		Formulas:        machineDefaultFormulas(),
		RMCosts:         map[string]RMCostRates{"RM1|": {CostVal: 1.0}},
		EvalCache:       evaluator.NewCache(),
		MachineDefaults: defaults,
	})
	require.NoError(t, err)
	return out
}

func mbCostTrace(t *testing.T, out *ComputeOutput) FormulaEvalTrace {
	t.Helper()
	for _, tr := range out.FormulaTrace {
		if tr.ResultParamCode == "MB_COST_MKT" {
			return tr
		}
	}
	t.Fatal("no MB_COST_MKT trace")
	return FormulaEvalTrace{}
}

func TestComputeProduct_MachineDefault_NoStoredValue_UsesDefault(t *testing.T) {
	out := runMachineDefault(t,
		map[string]float64{"MB_SP_DOZING": 10, "MB_RATE_MKT": 5},
		map[string]float64{"MB_COST_MKT": 0.08})
	assert.InDelta(t, 0.08, out.ParamSnapshot["MB_COST_MKT"], 1e-12)
	assert.InDelta(t, 1.08, out.CostPerUnit, 1e-12, "downstream consumer uses the default")
	assert.Equal(t, "machine default (machine initial 0.08)", mbCostTrace(t, out).Expression)
}

func TestComputeProduct_MachineDefault_StoredValueWins(t *testing.T) {
	out := runMachineDefault(t,
		map[string]float64{"MB_SP_DOZING": 10, "MB_RATE_MKT": 5, "MB_COST_MKT": 0.1},
		map[string]float64{"MB_COST_MKT": 0.08})
	assert.InDelta(t, 0.1, out.ParamSnapshot["MB_COST_MKT"], 1e-12)
	assert.InDelta(t, 1.1, out.CostPerUnit, 1e-12)
	assert.Equal(t, "machine default (product value)", mbCostTrace(t, out).Expression)
}

func TestComputeProduct_MachineDefault_NilKeepsFormula(t *testing.T) {
	// A stored value is still overwritten by the formula, exactly as before.
	out := runMachineDefault(t,
		map[string]float64{"MB_SP_DOZING": 10, "MB_RATE_MKT": 5, "MB_COST_MKT": 0.1},
		nil)
	assert.InDelta(t, 0.5, out.ParamSnapshot["MB_COST_MKT"], 1e-12)
	assert.InDelta(t, 1.5, out.CostPerUnit, 1e-12)
	assert.Equal(t, "MB_SP_DOZING * MB_RATE_MKT / 100.0", mbCostTrace(t, out).Expression)
}

func TestComputeProduct_MachineDefault_NotApplicableParamIgnored(t *testing.T) {
	// STEAM_COST_CNG has a machine default but no formula for this product (not
	// applicable): it must not enter the snapshot, and MB_COST_MKT computes normally.
	out := runMachineDefault(t,
		map[string]float64{"MB_SP_DOZING": 10, "MB_RATE_MKT": 5},
		map[string]float64{"STEAM_COST_CNG": 0.019})
	_, has := out.ParamSnapshot["STEAM_COST_CNG"]
	assert.False(t, has)
	assert.InDelta(t, 0.5, out.ParamSnapshot["MB_COST_MKT"], 1e-12)
	assert.Equal(t, "MB_SP_DOZING * MB_RATE_MKT / 100.0", mbCostTrace(t, out).Expression)
}
