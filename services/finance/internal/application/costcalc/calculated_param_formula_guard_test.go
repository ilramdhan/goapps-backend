package costcalc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// D-02. Migration 000408 seeds F_YARN_NON_STD_BC_SP (-> NON_STD_BC_SP) and
// F_YARN_ADD_NON_STD_BC_LOSS (-> ADD_NON_STD_BC_LOSS) with is_active = FALSE.
// loadPerProductFormulas filters is_active = TRUE, so activating the consumer
// before the producer leaves the produced param absent from both CAPP and the
// formula set — buildInitialScope then writes a synthetic 0 and the consumed
// term silently vanishes from the cost. These tests pin the refusal.

func TestRejectCalculatedParamsWithoutFormula_ConsumedButUnproduced(t *testing.T) {
	// ADD_NON_STD_BC_LOSS is CALCULATED and consumed, but the only formula present
	// produces something else — exactly the "83 activated before 82" ordering.
	formulas := []Formula{{
		FormulaCode:     "F_YARN_ADD_NON_STD_BC_LOSS",
		FormulaType:     "CALCULATION",
		Expression:      "NON_STD_BC_SP * -1.0",
		ResultParamCode: "ADD_NON_STD_BC_LOSS",
		InputParamCodes: []string{"NON_STD_BC_SP"},
	}}
	calculated := map[string]bool{"NON_STD_BC_SP": true, "ADD_NON_STD_BC_LOSS": true}
	zeroFilled := map[string]bool{"NON_STD_BC_SP": true}

	err := rejectCalculatedParamsWithoutFormula(42, calculated, formulas, zeroFilled)
	if err == nil {
		t.Fatal("expected refusal for CALCULATED param NON_STD_BC_SP with no active formula, got nil")
	}
	if !errors.Is(err, ErrCalculatedParamNoFormula) {
		t.Fatalf("expected ErrCalculatedParamNoFormula, got %v", err)
	}
	if !strings.Contains(err.Error(), "NON_STD_BC_SP") {
		t.Fatalf("error must name the offending param_code, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "ADD_NON_STD_BC_LOSS") {
		t.Fatalf("ADD_NON_STD_BC_LOSS IS produced by an active formula; it must not be reported: %q", err.Error())
	}
}

func TestRejectCalculatedParamsWithoutFormula_ProducedIsAccepted(t *testing.T) {
	formulas := []Formula{
		{FormulaCode: "F_YARN_NON_STD_BC_SP", ResultParamCode: "NON_STD_BC_SP"},
		{FormulaCode: "F_YARN_ADD_NON_STD_BC_LOSS", ResultParamCode: "ADD_NON_STD_BC_LOSS",
			InputParamCodes: []string{"NON_STD_BC_SP"}},
	}
	calculated := map[string]bool{"NON_STD_BC_SP": true, "ADD_NON_STD_BC_LOSS": true}
	// Both are formula outputs, so both were zero-filled as result placeholders.
	zeroFilled := map[string]bool{"NON_STD_BC_SP": true, "ADD_NON_STD_BC_LOSS": true}

	if err := rejectCalculatedParamsWithoutFormula(42, calculated, formulas, zeroFilled); err != nil {
		t.Fatalf("both params are produced by active formulas; expected nil, got %v", err)
	}
}

func TestRejectCalculatedParamsWithoutFormula_UnreferencedIsAccepted(t *testing.T) {
	// R_AX is CALCULATED with no active formula, but nothing reads it: it never
	// entered scope, so it cannot corrupt a number. Must not block the calculation.
	formulas := []Formula{{FormulaCode: "F_X", ResultParamCode: "COST_STAGE_OUT", InputParamCodes: []string{"COST_RM_TOTAL"}}}
	calculated := map[string]bool{"R_AX": true}
	zeroFilled := map[string]bool{} // R_AX absent => never zero-filled => never referenced

	if err := rejectCalculatedParamsWithoutFormula(42, calculated, formulas, zeroFilled); err != nil {
		t.Fatalf("unreferenced CALCULATED param must not block; got %v", err)
	}
}

func TestRejectCalculatedParamsWithoutFormula_NilSetDisablesGuard(t *testing.T) {
	formulas := []Formula{{FormulaCode: "F_A", ResultParamCode: "A", InputParamCodes: []string{"MISSING"}}}
	if err := rejectCalculatedParamsWithoutFormula(1, nil, formulas, map[string]bool{"MISSING": true}); err != nil {
		t.Fatalf("nil CalculatedParams must disable the guard, got %v", err)
	}
}

// TestComputeProduct_CalculatedParamWithoutFormulaFails is the end-to-end proof that
// the bug is real: with CalculatedParams unset (pre-guard behavior) ComputeProduct
// happily returns a cost computed from the fabricated 0; with it set it refuses.
func TestComputeProduct_CalculatedParamWithoutFormulaFails(t *testing.T) {
	base := ComputeInput{
		ProductSysID: 7,
		Period:       "202601",
		Route:        &costroute.Graph{Head: &costroute.Head{HeadID: 1}},
		CAPP:         map[string]float64{"BASE_COST": 100},
		Formulas: []Formula{{
			FormulaCode:     "F_YARN_ADD_NON_STD_BC_LOSS",
			FormulaType:     "CALCULATION",
			Expression:      "BASE_COST + NON_STD_BC_SP * -1.0",
			ResultParamCode: "ADD_NON_STD_BC_LOSS",
			InputParamCodes: []string{"BASE_COST", "NON_STD_BC_SP"},
		}},
		EvalCache: evaluator.NewCache(),
	}

	// Pre-guard behavior: silent zero-fill of NON_STD_BC_SP, no error at all.
	out, err := ComputeProduct(context.Background(), base)
	if err != nil {
		t.Fatalf("baseline compute (guard disabled) should succeed, got %v", err)
	}
	if out.CostPerUnit != 100 {
		t.Fatalf("baseline proves the silent zero-fill: expected 100 (the -1.0 term lost), got %v", out.CostPerUnit)
	}
	if _, present := out.ParamSnapshot["NON_STD_BC_SP"]; present {
		t.Fatal("zero-filled param should be omitted from the snapshot, leaving no trace at all")
	}

	// With the guard wired, the same input must fail hard and name the param.
	guarded := base
	guarded.CalculatedParams = map[string]bool{"NON_STD_BC_SP": true}
	if _, err := ComputeProduct(context.Background(), guarded); err == nil {
		t.Fatal("expected ComputeProduct to refuse a CALCULATED param with no active formula")
	} else if !errors.Is(err, ErrCalculatedParamNoFormula) {
		t.Fatalf("expected ErrCalculatedParamNoFormula, got %v", err)
	} else if !strings.Contains(err.Error(), "NON_STD_BC_SP") {
		t.Fatalf("error must name the param_code, got %q", err.Error())
	}
}
