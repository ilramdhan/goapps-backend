package mbcomposition

import (
	"errors"
	"math"
)

// SumTolerance is the absolute tolerance, in percentage points, allowed when
// checking that a composition's non-carrier percentages add up to 100%.
//
// Rationale (decision D17 / R16): legacy recipes carry per-row values rounded to
// 3 decimals, so a perfectly valid recipe can add up to 99.995 or 100.005. A
// tolerance of 0.01 accepts that drift while still rejecting real data errors
// such as 75, 65 or 40.42 (R15).
const SumTolerance = 0.01

// boundarySlack absorbs float64 representation error so that a total sitting
// exactly on SumTolerance (100.01 / 99.99) is accepted rather than rejected by a
// ~5e-15 artifact of binary floating point. It is NOT a loosening of the rule —
// see ValidateSum for the full rationale (G23).
const boundarySlack = 1e-9

// Sentinel errors for the composition sum rule.
var (
	// ErrCompositionEmpty is returned when a composition has no rows at all.
	// This is deliberately a *separate* case from ErrCompositionSumInvalid (R17):
	// "nothing entered yet" is a different user-facing problem from
	// "what was entered does not add up".
	ErrCompositionEmpty = errors.New("mbcomposition: composition has no rows")
	// ErrCompositionSumInvalid is returned when the composition percentages do
	// not add up to 100% within SumTolerance.
	ErrCompositionSumInvalid = errors.New("mbcomposition: composition percentages must total 100%")
)

// ValidateSum checks that total (the sum of non-carrier composition percentages)
// equals 100% within SumTolerance, for a composition made of rowCount rows.
//
// rowCount == 0 short-circuits to ErrCompositionEmpty and is checked *before*
// the total, because an empty composition sums to 0 and would otherwise be
// reported as a misleading "does not total 100%" error.
//
// The comparison is deliberately math.Abs(total-100) > SumTolerance and never
// total != 100: total is a float64 built by summing decimal values that have no
// exact binary representation, so an equality test would reject sums that are
// correct to every digit a user can see (e.g. 33.34+33.33+33.33 does not yield
// exactly 100.0 in float64).
//
// boundarySlack (G23, 2026-08-22) makes the tolerance boundary INCLUSIVE without
// widening the rule. The same float64 inexactness that motivates the math.Abs
// comparison also affects the boundary itself: the literal 100.01 is stored just
// above 100.01, so 100.01-100 evaluates to 0.01000000000000511591, which is
// strictly greater than SumTolerance and used to reject a total that is valid by
// every digit the user can see. Adding one representation-error's worth of slack
// (1e-9, nine orders of magnitude below SumTolerance) admits exactly the values
// that round-trip to the boundary and nothing more: 100.02 and 99.98 are still
// rejected, since they miss by 0.01 — ten million times the slack.
func ValidateSum(total float64, rowCount int) error {
	if rowCount == 0 {
		return ErrCompositionEmpty
	}
	if math.Abs(total-100) > SumTolerance+boundarySlack {
		return ErrCompositionSumInvalid
	}
	return nil
}
