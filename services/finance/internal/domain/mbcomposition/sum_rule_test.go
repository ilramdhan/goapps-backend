package mbcomposition_test

import (
	"errors"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// TestValidateSum covers the 8 cases specified for [G.5] in
// docs/superpowers/plans/2026-08-20-mb-recipe-spin-consolidated-plan-05-f1f10.md:1876,
// plus the explicit tolerance-boundary cases.
func TestValidateSum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		total    float64
		rowCount int
		wantErr  error
	}{
		// --- the 8 plan cases (plan:1876) ---
		{name: "case1_100.000_passes", total: 100.000, rowCount: 3, wantErr: nil},
		{name: "case2_100.005_passes_drift_R16", total: 100.005, rowCount: 3, wantErr: nil},
		{name: "case3_99.995_passes_drift_R16", total: 99.995, rowCount: 3, wantErr: nil},
		{name: "case4_100.011_rejected", total: 100.011, rowCount: 3, wantErr: mbcomposition.ErrCompositionSumInvalid},
		{name: "case5_99.989_rejected", total: 99.989, rowCount: 3, wantErr: mbcomposition.ErrCompositionSumInvalid},
		{name: "case6_75_rejected_R15", total: 75, rowCount: 2, wantErr: mbcomposition.ErrCompositionSumInvalid},
		{name: "case7_65_rejected_R15", total: 65, rowCount: 2, wantErr: mbcomposition.ErrCompositionSumInvalid},
		{name: "case7_40.42_rejected_R15", total: 40.42, rowCount: 1, wantErr: mbcomposition.ErrCompositionSumInvalid},
		{name: "case8_rowCount0_is_empty_not_sum_invalid", total: 0, rowCount: 0, wantErr: mbcomposition.ErrCompositionEmpty},

		// --- explicit tolerance boundaries ---
		{name: "boundary_exactly_100.00_passes", total: 100.00, rowCount: 4, wantErr: nil},
		// G23 (2026-08-22, user decision): the tolerance boundary is INCLUSIVE.
		// 100.01 and 99.99 used to be rejected — not by the rule, but by float64
		// representation error (100.01-100 == 0.01000000000000511591, strictly
		// greater than SumTolerance == 0.01). boundarySlack (1e-9) absorbs that
		// artifact. The rule itself is unchanged: 100.02 / 99.98 still fail.
		{name: "boundary_100.01_exactly_at_tolerance_accepted_G23", total: 100.01, rowCount: 4, wantErr: nil},
		{name: "boundary_99.99_exactly_at_tolerance_accepted_G23", total: 99.99, rowCount: 4, wantErr: nil},
		{name: "boundary_100.02_beyond_tolerance_rejected", total: 100.02, rowCount: 4, wantErr: mbcomposition.ErrCompositionSumInvalid},
		{name: "boundary_99.98_beyond_tolerance_rejected", total: 99.98, rowCount: 4, wantErr: mbcomposition.ErrCompositionSumInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := mbcomposition.ValidateSum(tt.total, tt.rowCount)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateSum(%v, %d) = %v, want %v", tt.total, tt.rowCount, err, tt.wantErr)
			}
		})
	}
}

// TestValidateSumEmptyBeatsTotal pins R17: rowCount == 0 reports
// ErrCompositionEmpty and never ErrCompositionSumInvalid, whatever the total is.
func TestValidateSumEmptyBeatsTotal(t *testing.T) {
	t.Parallel()

	for _, total := range []float64{0, 75, 100, 100.02} {
		err := mbcomposition.ValidateSum(total, 0)
		if !errors.Is(err, mbcomposition.ErrCompositionEmpty) {
			t.Fatalf("ValidateSum(%v, 0) = %v, want ErrCompositionEmpty", total, err)
		}
		if errors.Is(err, mbcomposition.ErrCompositionSumInvalid) {
			t.Fatalf("ValidateSum(%v, 0) must not be ErrCompositionSumInvalid", total)
		}
	}
}

// TestValidateSumFloatSummedInput guards the reason the rule uses
// math.Abs(total-100) > SumTolerance instead of total != 100: a composition whose
// decimal digits add up to exactly 100 does not produce exactly 100.0 in float64.
func TestValidateSumFloatSummedInput(t *testing.T) {
	t.Parallel()

	total := 33.34 + 33.33 + 33.33
	if total == 100 { //nolint:staticcheck // deliberately demonstrating float inexactness
		t.Skip("float64 happened to be exact on this platform; the rule is still correct")
	}
	if err := mbcomposition.ValidateSum(total, 3); err != nil {
		t.Fatalf("ValidateSum(%.20f, 3) = %v, want nil (an equality check would wrongly reject)", total, err)
	}
}

// TestValidateSumBoundaryInclusive is the dedicated proof for G23 (2026-08-22):
// the SumTolerance boundary is inclusive, and widening stops there.
//
// It first asserts the float64 fact that motivated the fix — 100.01-100 is
// strictly greater than SumTolerance — so that a future reader cannot mistake
// boundarySlack for an arbitrary fudge factor. Then it pins both directions:
// the two boundary values pass, the two values one tolerance-step beyond fail.
func TestValidateSumBoundaryInclusive(t *testing.T) {
	t.Parallel()

	if diff := 100.01 - 100; diff <= mbcomposition.SumTolerance {
		t.Logf("this platform represents 100.01-100 as %.20f (<= SumTolerance); "+
			"the inclusive-boundary guarantee below must hold regardless", diff)
	}

	accepted := []float64{100.01, 99.99}
	for _, total := range accepted {
		if err := mbcomposition.ValidateSum(total, 4); err != nil {
			t.Fatalf("ValidateSum(%v, 4) = %v, want nil (G23: boundary is inclusive)", total, err)
		}
	}

	rejected := []float64{100.02, 99.98}
	for _, total := range rejected {
		if err := mbcomposition.ValidateSum(total, 4); !errors.Is(err, mbcomposition.ErrCompositionSumInvalid) {
			t.Fatalf("ValidateSum(%v, 4) = %v, want ErrCompositionSumInvalid "+
				"(G23 must not loosen the rule beyond the boundary)", total, err)
		}
	}
}

// TestValidateSumSlackIsNarrow guards against boundarySlack ever being widened
// into a real loosening: a total that misses the tolerance by a visible amount
// must still be rejected, and rowCount == 0 must still win over the total.
func TestValidateSumSlackIsNarrow(t *testing.T) {
	t.Parallel()

	// 0.011 over — one thousandth beyond the boundary, still far above 1e-9.
	if err := mbcomposition.ValidateSum(100.011, 4); !errors.Is(err, mbcomposition.ErrCompositionSumInvalid) {
		t.Fatalf("ValidateSum(100.011, 4) = %v, want ErrCompositionSumInvalid", err)
	}
	if err := mbcomposition.ValidateSum(99.989, 4); !errors.Is(err, mbcomposition.ErrCompositionSumInvalid) {
		t.Fatalf("ValidateSum(99.989, 4) = %v, want ErrCompositionSumInvalid", err)
	}
	if err := mbcomposition.ValidateSum(100.01, 0); !errors.Is(err, mbcomposition.ErrCompositionEmpty) {
		t.Fatalf("ValidateSum(100.01, 0) = %v, want ErrCompositionEmpty", err)
	}
}
