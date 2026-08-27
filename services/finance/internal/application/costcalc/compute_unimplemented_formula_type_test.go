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

// xsectionExpression is the expression stored for F_MB_LDR_XSECTION by migration
// 000480. None of XSECTION_OP / LDR_SOURCE / XSECTION_FACTOR is ever placed in the
// costcalc scope by any code — that is exactly what makes the expr-lang path unsafe.
const xsectionExpression = "XSECTION_OP == 1 ? LDR_SOURCE * XSECTION_FACTOR : LDR_SOURCE / XSECTION_FACTOR"

// TestEvaluator_XSectionExpression_SilentZero documents WHY the K-22 guard has to
// exist, by asserting the evaluator's actual unguarded behavior on the real stored
// F_MB_LDR_XSECTION expression. Two sub-cases, both honest:
//
//   - Bare scope: expr-lang's AllowUndefinedVariables makes the expression COMPILE,
//     and it then fails at run time on `<nil> / <nil>`. Loud — but only by luck, and
//     only because nothing pre-fills the scope.
//   - Zero-filled scope: this is what the costcalc pipeline actually builds —
//     buildInitialScope pre-fills every InputParamCode and the result param with
//     float64(0) precisely so expr never sees nil. With those zeros, XSECTION_OP == 1
//     is false, the division branch runs 0/0 = NaN, and Evaluator.Run deliberately
//     maps NaN to (0, nil). A plausible-looking 0 cost, no error at all.
//
// The second case is the silent-failure path the guard closes. If either assertion
// starts failing, the guard's rationale changed and the guard should be revisited.
func TestEvaluator_XSectionExpression_SilentZero(t *testing.T) {
	ev, err := evaluator.Compile("F_MB_LDR_XSECTION", xsectionExpression)
	require.NoError(t, err, "AllowUndefinedVariables lets this compile even with no vars in scope")

	t.Run("bare scope errors on nil arithmetic", func(t *testing.T) {
		_, runErr := ev.Run(map[string]any{})
		require.Error(t, runErr, "nil operands happen to blow up — not a guard, just luck")
	})

	t.Run("zero-filled scope yields 0 with no error", func(t *testing.T) {
		// Exactly the shape buildInitialScope produces for a formula whose inputs
		// nothing supplies: every referenced param defaulted to float64(0).
		scope := map[string]any{
			"XSECTION_OP":     float64(0),
			"LDR_SOURCE":      float64(0),
			"XSECTION_FACTOR": float64(0),
		}
		got, runErr := ev.Run(scope)
		require.NoError(t, runErr, "Run maps the resulting NaN to (0, nil) — a silent wrong answer")
		assert.InDelta(t, 0.0, got, 1e-9, "this plausible-looking 0 is the bug the K-22 guard prevents")
	})
}

// TestEvalSingleFormulaStep_MBXSectionLookup_ReturnsError is the guard itself: an
// MB_XSECTION_LOOKUP formula must produce an actionable error, never (0, nil).
func TestEvalSingleFormulaStep_MBXSectionLookup_ReturnsError(t *testing.T) {
	f := Formula{
		FormulaCode:     "F_MB_LDR_XSECTION",
		FormulaType:     FormulaTypeMBXSectionLookup,
		Expression:      xsectionExpression,
		ResultParamCode: "MB_LDR_XSECTION",
	}

	trace, err := evalSingleFormulaStep(
		context.Background(), evaluator.NewCache(), map[string]any{},
		0, f, 4242, nil, costcalcdom.CalcTypeActual,
	)

	require.Error(t, err, "MB_XSECTION_LOOKUP must NOT silently evaluate to 0")
	require.ErrorIs(t, err, ErrFormulaTypeNotImplemented)
	assert.Zero(t, trace.Output)
	// The message must be actionable: formula code, type, product, and the reason.
	assert.Contains(t, err.Error(), "F_MB_LDR_XSECTION")
	assert.Contains(t, err.Error(), "MB_XSECTION_LOOKUP")
	assert.Contains(t, err.Error(), "4242")
	assert.Contains(t, err.Error(), "mst_mb_cross_section_factor")
}

// TestEvalSingleFormulaStep_UnimplementedLookupTypes_ReturnError covers every entry
// of the denylist, so adding a value to formulaTypesNeedingGoImpl is automatically
// covered here too.
func TestEvalSingleFormulaStep_UnimplementedLookupTypes_ReturnError(t *testing.T) {
	require.NotEmpty(t, formulaTypesNeedingGoImpl)
	for ftype := range formulaTypesNeedingGoImpl {
		t.Run(ftype, func(t *testing.T) {
			f := Formula{
				FormulaCode:     "F_TEST_" + ftype,
				FormulaType:     ftype,
				Expression:      xsectionExpression,
				ResultParamCode: "SOME_PARAM",
			}
			_, err := evalSingleFormulaStep(
				context.Background(), evaluator.NewCache(), map[string]any{},
				0, f, 7, nil, costcalcdom.CalcTypeActual,
			)
			require.ErrorIs(t, err, ErrFormulaTypeNotImplemented)
			assert.Contains(t, err.Error(), ftype)
		})
	}
}

// TestComputeProduct_MBXSectionLookup_FailsWholeProduct proves the guard propagates
// out of ComputeProduct rather than being swallowed mid-chain.
//
// This is also the bite-proof case for the WHOLE pipeline, not just the evaluator:
// InputParamCodes is populated exactly as the loader would populate it from
// mst_formula_param, so buildInitialScope zero-fills XSECTION_OP / LDR_SOURCE /
// XSECTION_FACTOR with float64(0) — no nils left to blow up on. With the guard
// removed, this input returns CostPerUnit == 0 and a NIL error (verified
// 2026-08-22 by temporarily disabling the guard). With the guard in place it must
// return ErrFormulaTypeNotImplemented.
func TestComputeProduct_MBXSectionLookup_FailsWholeProduct(t *testing.T) {
	in := ComputeInput{
		ProductSysID: 99,
		CalcType:     costcalcdom.CalcTypeActual,
		Route:        buildOneStageRoute(99, costroute.RmTypeItem, "RM001", 0),
		Formulas: []Formula{{
			FormulaCode:     "F_MB_LDR_XSECTION",
			FormulaType:     FormulaTypeMBXSectionLookup,
			Expression:      xsectionExpression,
			ResultParamCode: ScopeKeyFinalCost,
			InputParamCodes: []string{"XSECTION_OP", "LDR_SOURCE", "XSECTION_FACTOR"},
		}},
		RMCosts:   map[string]float64{"RM001|": 0},
		EvalCache: evaluator.NewCache(),
	}

	out, err := ComputeProduct(context.Background(), in)
	require.Error(t, err, "without the guard this returns (CostPerUnit=0, nil) — the silent wrong answer")
	require.ErrorIs(t, err, ErrFormulaTypeNotImplemented)
	assert.Nil(t, out)
}

// TestEvalSingleFormulaStep_WorkingTypes_Unchanged is the regression fence: the
// guard must bite ONLY types that fall through to expr-lang incorrectly. Types
// that work today must behave exactly as before.
func TestEvalSingleFormulaStep_WorkingTypes_Unchanged(t *testing.T) {
	ctx := context.Background()
	cache := evaluator.NewCache()

	t.Run("SNAPSHOT", func(t *testing.T) {
		scope := map[string]any{"SOME_COST": 12.5}
		f := Formula{
			FormulaCode: "F_SNAP", FormulaType: "SNAPSHOT",
			Expression: "snapshot(SOME_COST) at process start", ResultParamCode: "SOME_COST_SNAP",
			InputParamCodes: []string{"SOME_COST"},
		}
		tr, err := evalSingleFormulaStep(ctx, cache, scope, 0, f, 1, nil, costcalcdom.CalcTypeActual)
		require.NoError(t, err)
		assert.InDelta(t, 12.5, tr.Output, 1e-9)
	})

	t.Run("RM_LOOKUP", func(t *testing.T) {
		f := Formula{FormulaCode: "F_RM", FormulaType: FormulaTypeRMLookup, ResultParamCode: "COST_RM"}
		tr, err := evalSingleFormulaStep(ctx, cache, map[string]any{}, 33.25, f, 1, nil, costcalcdom.CalcTypeActual)
		require.NoError(t, err)
		assert.InDelta(t, 33.25, tr.Output, 1e-9)
	})

	t.Run("MB_COST_LOOKUP", func(t *testing.T) {
		f := Formula{FormulaCode: "F_MB", FormulaType: FormulaTypeMBCostLookup, ResultParamCode: "MB_COST"}
		mb := map[string]float64{string(costcalcdom.CalcTypeActual): 8.75}
		tr, err := evalSingleFormulaStep(ctx, cache, map[string]any{}, 0, f, 1, mb, costcalcdom.CalcTypeActual)
		require.NoError(t, err)
		assert.InDelta(t, 8.75, tr.Output, 1e-9)
	})

	t.Run("CALCULATION", func(t *testing.T) {
		scope := map[string]any{"A": 2.0, "B": 3.0}
		f := Formula{
			FormulaCode: "F_CALC", FormulaType: "CALCULATION", Expression: "A + B",
			ResultParamCode: "C", InputParamCodes: []string{"A", "B"},
		}
		tr, err := evalSingleFormulaStep(ctx, cache, scope, 0, f, 1, nil, costcalcdom.CalcTypeActual)
		require.NoError(t, err)
		assert.InDelta(t, 5.0, tr.Output, 1e-9)
	})

	t.Run("CONDITIONAL", func(t *testing.T) {
		scope := map[string]any{"BATCH_WEIGHT": 4.0, "HEATSET_COST_PER_BATCH": 8.0}
		f := Formula{
			FormulaCode: "F_COND", FormulaType: "CONDITIONAL",
			Expression:      "BATCH_WEIGHT > 0 ? HEATSET_COST_PER_BATCH / BATCH_WEIGHT : 0",
			ResultParamCode: "HEATSET_COST_PER_KG",
			InputParamCodes: []string{"BATCH_WEIGHT", "HEATSET_COST_PER_BATCH"},
		}
		tr, err := evalSingleFormulaStep(ctx, cache, scope, 0, f, 1, nil, costcalcdom.CalcTypeActual)
		require.NoError(t, err)
		assert.InDelta(t, 2.0, tr.Output, 1e-9)
	})

	t.Run("CONSTANT", func(t *testing.T) {
		f := Formula{
			FormulaCode: "F_CONST", FormulaType: "CONSTANT", Expression: "0.024",
			ResultParamCode: "FORWARDING_COST",
		}
		tr, err := evalSingleFormulaStep(ctx, cache, map[string]any{}, 0, f, 1, nil, costcalcdom.CalcTypeActual)
		require.NoError(t, err)
		assert.InDelta(t, 0.024, tr.Output, 1e-9)
	})
}
