package mbdozing_test

import (
	"errors"
	"math"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbdozing"
)

const tolerance = 1e-4

// newFactor builds a factor row the way storage hydration does, so tests can also
// construct rows with operations the constructor would reject (e.g. "ADD").
func newFactor(from, to string, factor float64, op string) *mbcrosssection.FactorEntity {
	return mbcrosssection.ReconstructFactor(
		"", from, to, factor, op, "", true, "", "seed_test", "", "", "", "",
	)
}

func TestScaleLDR_GoldenCase(t *testing.T) {
	// spec:3319 — 380/108 at LDR 0.9 -> 500/96.
	const want = 0.7397296803562773

	got, err := mbdozing.ScaleLDR(mbdozing.ScaleInput{
		LDRRef:         0.9,
		DenierRef:      380,
		FilamentRef:    108,
		DenierTarget:   500,
		FilamentTarget: 96,
	})
	if err != nil {
		t.Fatalf("ScaleLDR returned unexpected error: %v", err)
	}
	if math.Abs(got-want) > tolerance {
		t.Fatalf("ScaleLDR = %v, want %v (tolerance %v)", got, want, tolerance)
	}
}

func TestScaleLDR_ZeroFilament(t *testing.T) {
	cases := map[string]mbdozing.ScaleInput{
		"reference filament zero": {
			LDRRef: 0.9, DenierRef: 380, FilamentRef: 0,
			DenierTarget: 500, FilamentTarget: 96,
		},
		"target filament zero": {
			LDRRef: 0.9, DenierRef: 380, FilamentRef: 108,
			DenierTarget: 500, FilamentTarget: 0,
		},
		"negative filament": {
			LDRRef: 0.9, DenierRef: 380, FilamentRef: 108,
			DenierTarget: 500, FilamentTarget: -96,
		},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := mbdozing.ScaleLDR(in)
			if !errors.Is(err, mbdozing.ErrZeroFilament) {
				t.Fatalf("error = %v, want ErrZeroFilament", err)
			}
			if math.IsInf(got, 0) || math.IsNaN(got) {
				t.Fatalf("result = %v, want a finite zero value", got)
			}
		})
	}
}

func TestRatio_ZeroFilament(t *testing.T) {
	if _, err := mbdozing.Ratio(380, 0); !errors.Is(err, mbdozing.ErrZeroFilament) {
		t.Fatalf("error = %v, want ErrZeroFilament", err)
	}
}

// The two directions are asserted as SEPARATE cases on purpose. A round-trip-only
// test would pass falsely if the code used MULTIPLY for both directions.
func TestConvertCrossSection_RoundToTable_Divides(t *testing.T) {
	got, err := mbdozing.ConvertCrossSection(0.9, newFactor("RND", "TBL", 0.82, mbcrosssection.OperationDivide))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 0.9 / 0.82
	if math.Abs(got-want) > tolerance {
		t.Fatalf("RND->TBL = %v, want %v", got, want)
	}
}

func TestConvertCrossSection_TableToRound_Multiplies(t *testing.T) {
	got, err := mbdozing.ConvertCrossSection(0.9, newFactor("TBL", "RND", 0.82, mbcrosssection.OperationMultiply))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 0.9 * 0.82
	if math.Abs(got-want) > tolerance {
		t.Fatalf("TBL->RND = %v, want %v", got, want)
	}
}

// Directions must not collapse into the same arithmetic.
func TestConvertCrossSection_DirectionsDiffer(t *testing.T) {
	const ldr = 0.9
	toTable, err := mbdozing.ConvertCrossSection(ldr, newFactor("RND", "TBL", 0.82, mbcrosssection.OperationDivide))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	toRound, err := mbdozing.ConvertCrossSection(ldr, newFactor("TBL", "RND", 0.82, mbcrosssection.OperationMultiply))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(toTable-toRound) <= tolerance {
		t.Fatalf("RND->TBL (%v) and TBL->RND (%v) must differ; the operation column was likely ignored", toTable, toRound)
	}
	if toTable <= ldr {
		t.Fatalf("RND->TBL = %v, expected DIVIDE by 0.82 to increase %v", toTable, ldr)
	}
	if toRound >= ldr {
		t.Fatalf("TBL->RND = %v, expected MULTIPLY by 0.82 to decrease %v", toRound, ldr)
	}
}

// Supplementary only — never a substitute for the paired direction tests above.
func TestConvertCrossSection_RoundTrip(t *testing.T) {
	const ldr = 0.9
	toTable, err := mbdozing.ConvertCrossSection(ldr, newFactor("RND", "TBL", 0.82, mbcrosssection.OperationDivide))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	back, err := mbdozing.ConvertCrossSection(toTable, newFactor("TBL", "RND", 0.82, mbcrosssection.OperationMultiply))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(back-ldr) > tolerance {
		t.Fatalf("round trip = %v, want %v", back, ldr)
	}
}

func TestConvertCrossSection_UnknownOperation(t *testing.T) {
	for _, op := range []string{"", "ADD", "multiply", "SUBTRACT"} {
		t.Run("op="+op, func(t *testing.T) {
			got, err := mbdozing.ConvertCrossSection(0.9, newFactor("RND", "TBL", 0.82, op))
			if !errors.Is(err, mbcrosssection.ErrFactorInvalidOperation) {
				t.Fatalf("error = %v, want ErrFactorInvalidOperation", err)
			}
			if got != 0 {
				t.Fatalf("result = %v, want 0 (must not silently multiply)", got)
			}
		})
	}
}

func TestConvertCrossSection_NilFactor(t *testing.T) {
	if _, err := mbdozing.ConvertCrossSection(0.9, nil); !errors.Is(err, mbdozing.ErrFactorNotFound) {
		t.Fatalf("error = %v, want ErrFactorNotFound", err)
	}
}

func TestConvertCrossSection_NonPositiveFactor(t *testing.T) {
	if _, err := mbdozing.ConvertCrossSection(0.9, newFactor("RND", "TBL", 0, mbcrosssection.OperationDivide)); !errors.Is(err, mbcrosssection.ErrFactorNotPositive) {
		t.Fatalf("error = %v, want ErrFactorNotPositive", err)
	}
}

func TestCalculate_Dispatch(t *testing.T) {
	got, err := mbdozing.Calculate(mbdozing.Input{
		Mode: mbdozing.ModeScale,
		Scale: mbdozing.ScaleInput{
			LDRRef: 0.9, DenierRef: 380, FilamentRef: 108,
			DenierTarget: 500, FilamentTarget: 96,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(got-0.7397296803562773) > tolerance {
		t.Fatalf("SCALE dispatch = %v, want 0.7397296803562773", got)
	}

	got, err = mbdozing.Calculate(mbdozing.Input{
		Mode: mbdozing.ModeXSection,
		XSection: mbdozing.XSectionInput{
			LDR:    0.9,
			Factor: newFactor("RND", "TBL", 0.82, mbcrosssection.OperationDivide),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(got-0.9/0.82) > tolerance {
		t.Fatalf("XSECTION dispatch = %v, want %v", got, 0.9/0.82)
	}
}

func TestCalculate_InvalidMode(t *testing.T) {
	for _, mode := range []string{"", "scale", "ADJUST", "UNKNOWN"} {
		t.Run("mode="+mode, func(t *testing.T) {
			if _, err := mbdozing.Calculate(mbdozing.Input{Mode: mode}); !errors.Is(err, mbdozing.ErrInvalidMode) {
				t.Fatalf("error = %v, want ErrInvalidMode", err)
			}
		})
	}
}
