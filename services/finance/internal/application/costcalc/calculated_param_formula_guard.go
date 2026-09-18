package costcalc

import (
	"errors"
	"fmt"
	"sort"
)

// ErrCalculatedParamNoFormula is returned when a parameter that master data declares
// as mst_parameter.param_category = 'CALCULATED' is consumed by the formula chain of a
// product, yet no ACTIVE mst_formula row produces it.
//
// Why this is fatal rather than a zero (D-02). loadPerProductFormulas filters
// `f.is_active = TRUE` (loader.go), so an inactive/PENDING formula is simply never
// returned. LoadCAPP only returns params that carry a persisted cost_product_parameter
// value, and a CALCULATED param has none by definition — the engine is supposed to
// produce it. buildInitialScope then hits neither source and writes the synthetic
// placeholder float64(0) into scope for every InputParamCode it cannot resolve. The
// consuming formula's arithmetic therefore runs against a fabricated 0 and returns a
// perfectly plausible number.
//
// The zeroFilled bookkeeping only suppresses the key from cpc_param_snapshot; it never
// suppresses the arithmetic. So the wrong number is persisted as the cost while the
// evidence of how it got there is the one thing omitted.
//
// Concrete case: migration 000408 seeds F_YARN_NON_STD_BC_SP (-> NON_STD_BC_SP) and
// F_YARN_ADD_NON_STD_BC_LOSS (-> ADD_NON_STD_BC_LOSS) both PENDING/is_active = FALSE.
// Activating the consumer before its producer makes the consumed term evaluate as 0 —
// the term disappears from the cost with no error, no trace entry and no snapshot key.
//
// A CALCULATED param without an active formula is a master-data configuration error,
// never a legitimate zero, so the calculation must stop and name the param.
var ErrCalculatedParamNoFormula = errors.New(
	"parameter is declared CALCULATED but no active formula produces it — " +
		"this is a master data configuration error (inactive/PENDING mst_formula row), not a zero value")

// rejectCalculatedParamsWithoutFormula fails the compute pass when a CALCULATED param is
// actually consumed by this product's formula chain but nothing active produces it.
//
// Inputs:
//
//	calculatedParams — param codes assigned to the product via cost_product_applicable_param
//	                   whose mst_parameter.param_category is 'CALCULATED'. An empty or nil
//	                   map DISABLES the guard, so any wiring that does not supply it (tests,
//	                   callers built before this field existed) behaves exactly as before.
//	formulas         — the active formulas loaded for the product; their ResultParamCode set
//	                   is the set of params that genuinely do get produced.
//	zeroFilled       — the synthetic-placeholder set from buildInitialScope. Membership is
//	                   what makes this harmful: a CALCULATED param nobody references is not
//	                   in scope at all and cannot corrupt any number, so it is not an error.
//
// The check is deliberately narrowed by zeroFilled rather than applied to every CALCULATED
// param, so master data carrying not-yet-wired CALCULATED params keeps calculating as long
// as no formula actually reads them.
func rejectCalculatedParamsWithoutFormula(
	productSysID int64,
	calculatedParams map[string]bool,
	formulas []Formula,
	zeroFilled map[string]bool,
) error {
	if len(calculatedParams) == 0 {
		return nil
	}

	produced := make(map[string]bool, len(formulas))
	for _, f := range formulas {
		produced[f.ResultParamCode] = true
	}

	offenders := make([]string, 0, len(calculatedParams))
	for code, isCalculated := range calculatedParams {
		if !isCalculated || produced[code] || !zeroFilled[code] {
			continue
		}
		offenders = append(offenders, code)
	}
	if len(offenders) == 0 {
		return nil
	}
	// Map iteration is random; sort so the error text is deterministic and diffable.
	sort.Strings(offenders)

	return fmt.Errorf("%w: product %d, param_code=%v", ErrCalculatedParamNoFormula, productSysID, offenders)
}
