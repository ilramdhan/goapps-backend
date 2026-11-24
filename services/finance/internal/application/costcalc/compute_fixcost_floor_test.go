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

// fixCostFloorInput: TOTAL_FIXEDCOST_PER_KG = FIX_IN, then the final cost
// reads it downstream (COST_RM_TOTAL + TOTAL_FIXEDCOST_PER_KG).
func fixCostFloorInput(fixed float64, threshold *float64) ComputeInput {
	return ComputeInput{
		ProductSysID: 7,
		Period:       "202611",
		CalcType:     costcalcdom.CalcTypeActual,
		Route:        buildOneStageRoute(7, costroute.RmTypeItem, "RM001", 1.0),
		CAPP:         map[string]float64{"FIX_IN": fixed},
		Formulas: []Formula{
			{
				FormulaCode:     "F_FIX",
				Expression:      "FIX_IN",
				ResultParamCode: ScopeKeyTotalFixedCostPerKg,
				InputParamCodes: []string{"FIX_IN"},
			},
			{
				FormulaCode:     "F_FINAL",
				Expression:      "COST_RM_TOTAL + TOTAL_FIXEDCOST_PER_KG",
				ResultParamCode: ScopeKeyFinalCost,
				InputParamCodes: []string{ScopeKeyCostRMTotal, ScopeKeyTotalFixedCostPerKg},
			},
		},
		RMCosts:             map[string]RMCostRates{"RM001|": {CostVal: 1.0}},
		EvalCache:           evaluator.NewCache(),
		FixCostMinThreshold: threshold,
	}
}

func TestComputeProduct_FixCostFloor(t *testing.T) {
	ptr := func(v float64) *float64 { return &v }
	cases := []struct {
		name          string
		fixed         float64
		threshold     *float64
		wantFixed     float64
		wantShortfall *float64 // nil = key absent
		wantFloorStep bool
	}{
		{"below floors", 0.02, ptr(0.05), 0.05, ptr(0.03), true},
		{"equal unchanged", 0.05, ptr(0.05), 0.05, ptr(0), false},
		{"above unchanged", 0.08, ptr(0.05), 0.08, ptr(0), false},
		{"zero threshold off", 0.02, ptr(0), 0.02, ptr(0), false},
		{"nil threshold legacy", 0.02, nil, 0.02, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := ComputeProduct(context.Background(), fixCostFloorInput(tc.fixed, tc.threshold))
			require.NoError(t, err)
			snap := out.ParamSnapshot

			assert.InDelta(t, tc.wantFixed, snap[ScopeKeyTotalFixedCostPerKg], 1e-12)
			// Downstream formula reads the (floored) value: 1.0 RM + fixed.
			assert.InDelta(t, 1.0+tc.wantFixed, snap[ScopeKeyFinalCost], 1e-12)

			got, ok := snap[ScopeKeyFixedCostShortfall]
			if tc.wantShortfall == nil {
				assert.False(t, ok, "shortfall key must be absent without a threshold")
			} else {
				require.True(t, ok)
				assert.InDelta(t, *tc.wantShortfall, got, 1e-12)
			}

			floorSteps := 0
			for _, tr := range out.FormulaTrace {
				if tr.FormulaCode == fixCostFloorTraceCode {
					floorSteps++
					assert.InDelta(t, tc.wantFixed, tr.Output, 1e-12)
					assert.InDelta(t, tc.fixed, tr.Inputs[ScopeKeyTotalFixedCostPerKg], 1e-12)
				}
			}
			assert.Equal(t, tc.wantFloorStep, floorSteps == 1)
		})
	}
}
