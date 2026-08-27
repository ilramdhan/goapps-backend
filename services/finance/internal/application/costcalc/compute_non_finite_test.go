package costcalc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// nonFiniteInput builds a one-formula product whose single formula divides
// COST_RM_TOTAL-derived values in a way the caller controls via numerator and
// divisor CAPP params.
func nonFiniteInput(expr string, capp map[string]float64, inputs []string) ComputeInput {
	return ComputeInput{
		ProductSysID: 1,
		Route:        buildOneStageRoute(1, costroute.RmTypeItem, "X", 1.0),
		CAPP:         capp,
		Formulas: []Formula{{
			FormulaCode:     "F_FINAL",
			Expression:      expr,
			ResultParamCode: ScopeKeyFinalCost,
			InputParamCodes: inputs,
		}},
		RMCosts:   map[string]float64{"X|": 10.0},
		EvalCache: evaluator.NewCache(),
	}
}

// TestComputeProduct_NonFiniteMarkedInTrace covers all three kinds end to end:
// the cost stays 0 (unchanged behavior) and the trace entry carries the kind.
func TestComputeProduct_NonFiniteMarkedInTrace(t *testing.T) {
	cases := []struct {
		name string
		expr string
		capp map[string]float64
		want evaluator.NonFiniteKind
	}{
		{
			name: "nan_from_zero_over_zero",
			expr: "NUMER / DIVISOR",
			capp: map[string]float64{"NUMER": 0.0, "DIVISOR": 0.0},
			want: evaluator.NonFiniteNaN,
		},
		{
			name: "pos_inf_from_x_over_zero",
			expr: "NUMER / DIVISOR",
			capp: map[string]float64{"NUMER": 1.0, "DIVISOR": 0.0},
			want: evaluator.NonFinitePosInf,
		},
		{
			name: "neg_inf_from_negative_x_over_zero",
			expr: "NUMER / DIVISOR",
			capp: map[string]float64{"NUMER": -1.0, "DIVISOR": 0.0},
			want: evaluator.NonFiniteNegInf,
		},
		{
			name: "pos_inf_from_overflow",
			expr: "NUMER * 10.0",
			capp: map[string]float64{"NUMER": 1e308, "DIVISOR": 1.0},
			want: evaluator.NonFinitePosInf,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := nonFiniteInput(tc.expr, tc.capp, []string{"NUMER", "DIVISOR"})
			out, err := ComputeProduct(context.Background(), in)
			require.NoError(t, err)

			// Behavior is unchanged: still a fabricated 0, still no error.
			require.Equal(t, float64(0), out.CostPerUnit)

			require.Len(t, out.FormulaTrace, 1)
			assert.Equal(t, tc.want, out.FormulaTrace[0].NonFinite,
				"the swallowed non-finite kind must reach cpc_formula_trace")
			assert.Equal(t, float64(0), out.FormulaTrace[0].Output)
		})
	}
}

// TestComputeProduct_NonFiniteMarkerSurvivesZeroFilledDelete pins the subtle
// interaction with evalFormulaChain's `delete(zeroFilled, f.ResultParamCode)`:
// that delete promotes the fabricated 0 into ParamSnapshot, but it must not
// touch the trace entry's marker, which is the only surviving evidence.
func TestComputeProduct_NonFiniteMarkerSurvivesZeroFilledDelete(t *testing.T) {
	in := nonFiniteInput("NUMER / DIVISOR",
		map[string]float64{"NUMER": 0.0, "DIVISOR": 0.0},
		[]string{"NUMER", "DIVISOR"})

	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)

	require.Len(t, out.FormulaTrace, 1)
	require.Equal(t, evaluator.NonFiniteNaN, out.FormulaTrace[0].NonFinite)

	// Documenting the known gap, not endorsing it: the result param is present
	// in the snapshot as a plain 0, indistinguishable from a real 0. The
	// snapshot has no place to record the marker (see scopeSnapshot doc), so
	// cpc_formula_trace is where a reader must look.
	snapVal, present := out.ParamSnapshot[ScopeKeyFinalCost]
	require.True(t, present, "delete(zeroFilled, ...) still promotes the fabricated 0 — unchanged behavior")
	assert.Equal(t, float64(0), snapVal)
}

// TestFormulaEvalTrace_NonFiniteJSONRoundTrip proves the new field needs no
// schema change: FormulaTrace is marshalled with encoding/json into the
// existing cpc_formula_trace JSONB column (process_chunk.go jsonOrNil /
// mbbatch service.go), so a new struct field is serialized automatically.
// It also proves finite entries stay byte-identical thanks to omitempty.
func TestFormulaEvalTrace_NonFiniteJSONRoundTrip(t *testing.T) {
	finite := FormulaEvalTrace{FormulaCode: "F1", Output: 1.5}
	b, err := json.Marshal([]FormulaEvalTrace{finite})
	require.NoError(t, err)
	assert.NotContains(t, string(b), "non_finite",
		"omitempty keeps the wire format unchanged for finite evaluations")

	marked := FormulaEvalTrace{FormulaCode: "F1", Output: 0, NonFinite: evaluator.NonFiniteNegInf}
	b, err = json.Marshal([]FormulaEvalTrace{marked})
	require.NoError(t, err)
	assert.Contains(t, string(b), `"non_finite":"neg_inf"`)

	var back []FormulaEvalTrace
	require.NoError(t, json.Unmarshal(b, &back))
	require.Len(t, back, 1)
	assert.Equal(t, evaluator.NonFiniteNegInf, back[0].NonFinite)
}

// TestComputeProduct_FiniteResultsUnmarked is the compute-level numeric
// regression guard: an ordinary product must produce the same numbers it
// always did, with an empty marker on every trace entry.
func TestComputeProduct_FiniteResultsUnmarked(t *testing.T) {
	in := nonFiniteInput("COST_RM_TOTAL / DIVISOR",
		map[string]float64{"DIVISOR": 4.0},
		[]string{ScopeKeyCostRMTotal, "DIVISOR"})

	out, err := ComputeProduct(context.Background(), in)
	require.NoError(t, err)

	assert.InDelta(t, 2.5, out.CostPerUnit, 1e-9, "10.0 / 4.0 — unchanged by K-35")
	require.Len(t, out.FormulaTrace, 1)
	assert.Equal(t, evaluator.NonFiniteNone, out.FormulaTrace[0].NonFinite)
	assert.InDelta(t, 2.5, out.FormulaTrace[0].Output, 1e-9)
}
