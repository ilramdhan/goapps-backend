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

// Products whose CAPP holds only the oil params backfilled by migration 000525
// load only the three oil formulas. Without COST_STAGE_OUT the inferred DAG sink
// is an oil param, so the guard must fail the product instead of reporting the
// oil value (typically 0) as its cost per unit.
func TestComputeProduct_FinalCostResolution(t *testing.T) {
	t.Parallel()

	base := oilCase{oil: ptyOil(oilGroupConing), opu: 2.2, wastePerc: 0.7,
		rmCosts: map[string]RMCostRates{oilGroupConing + "|": {CrRate: 2.2869}}}
	oilOnly := oilInput(base)
	oilOnly.Formulas = oilFormulas(oilGainPOYDefault)

	withSink := oilInput(base) // oil chain + F_YARN_STAGE_OUT = COST_RM_TOTAL

	noFormulas := oilInput(base)
	noFormulas.Formulas = nil

	soleTerminal := ComputeInput{
		ProductSysID: 1,
		Route:        buildOneStageRoute(1, costroute.RmTypeItem, "X", 1.0),
		Formulas: []Formula{{
			FormulaCode:     "F_SOMETHING_ELSE",
			Expression:      ScopeKeyCostRMTotal,
			ResultParamCode: "SOMETHING_ELSE",
			InputParamCodes: []string{ScopeKeyCostRMTotal},
		}},
		RMCosts:   map[string]RMCostRates{"X|": {CostVal: 10.0}},
		EvalCache: evaluator.NewCache(),
	}

	// A real (non-oil) chain deeper than the oil chain: the inferred sink is a
	// cost param, so path (b) still applies even though oil formulas are loaded.
	deepNonOil := oilInput(base)
	deepNonOil.Formulas = append(oilFormulas(oilGainPOYDefault),
		Formula{FormulaCode: "F_A", Expression: ScopeKeyCostRMTotal, ResultParamCode: "A_COST", InputParamCodes: []string{ScopeKeyCostRMTotal}},
		Formula{FormulaCode: "F_B", Expression: "A_COST * 2", ResultParamCode: "B_COST", InputParamCodes: []string{"A_COST"}},
		Formula{FormulaCode: "F_C", Expression: "B_COST + 1", ResultParamCode: "C_COST", InputParamCodes: []string{"B_COST"}},
	)

	tests := []struct {
		name     string
		in       ComputeInput
		wantErr  error
		wantMsg  []string
		wantCost float64
	}{
		{
			name:    "oil-only formula set fails",
			in:      oilOnly,
			wantErr: costcalcdom.ErrMissingFinalCostFormula,
			wantMsg: []string{"product 9101", "only 3 auxiliary formulas", "F_YARN_OIL_COST", "complete CAPP"},
		},
		{name: "no formulas returns total RM", in: noFormulas, wantCost: 10},
		{name: "COST_STAGE_OUT present is unchanged", in: withSink, wantCost: 10},
		{name: "legit sole terminal is unchanged", in: soleTerminal, wantCost: 10},
		{name: "deeper non-oil sink wins over oil sinks", in: deepNonOil, wantCost: 21},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, err := ComputeProduct(context.Background(), tt.in)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.NotErrorIs(t, err, costcalcdom.ErrFormulaEval,
					"must not map to BLOCKED/FORMULA_ERROR; the product must end FAILED")
				for _, m := range tt.wantMsg {
					assert.Contains(t, err.Error(), m)
				}
				assert.Nil(t, out)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, out)
			assert.InDelta(t, tt.wantCost, out.CostPerUnit, 1e-9)
		})
	}
}

// TestRecordComputeError_MissingFinalCostFormula_MarksFailed pins the status
// mapping: the sentinel falls through to the default arm (cjp_status FAILED,
// cjp_error_message = err.Error()), not to any BLOCKED reason.
func TestRecordComputeError_MissingFinalCostFormula_MarksFailed(t *testing.T) {
	t.Parallel()
	prodRepo := newRecordingProductRepo()
	svc := NewService(nil, &nopChunkRepo{}, prodRepo, &recordingResultRepo{}, nil,
		&mbGuardLoader{}, evaluator.NewCache(), nil, nil)

	in := oilInput(oilCase{oil: ptyOil(oilGroupConing), opu: 2.2, wastePerc: 0.7,
		rmCosts: map[string]RMCostRates{oilGroupConing + "|": {CrRate: 2.2869}}})
	in.Formulas = oilFormulas(oilGainPOYDefault)
	_, err := ComputeProduct(context.Background(), in)
	require.ErrorIs(t, err, costcalcdom.ErrMissingFinalCostFormula)

	outcome := svc.recordComputeError(context.Background(), mbGuardInput([]int64{9101}), 9101, err)
	assert.Equal(t, productOutcomeFailed, outcome)
	assert.Empty(t, prodRepo.blocked)
	assert.Equal(t, err.Error(), prodRepo.failed[9101])
}
