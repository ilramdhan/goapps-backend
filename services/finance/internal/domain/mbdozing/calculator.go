// Package mbdozing holds the pure MB dozing (LDR) calculation functions.
//
// Everything in this package is a PURE FUNCTION: no I/O, no database access, no
// proto types, no expression evaluation. Callers in the application layer are
// responsible for loading master data and passing it in.
//
// ⚠ WHY THE FORMULA IS HARD-CODED IN GO AND NOT EVALUATED FROM mst_formula
// (user decision K-16, gate G19-SQRT, 2026-08-22) — DO NOT "FIX" THIS BACK TO
// THE EXPRESSION EVALUATOR:
//
//   - The SCALE formula needs sqrt. The expr-lang builtin set in expr@v1.17.8
//     has NO sqrt function. The design document's claim that it does is WRONG.
//   - The evaluator is configured with AllowUndefinedVariables(), so an
//     expression calling sqrt(...) COMPILES SUCCESSFULLY and only fails at run
//     time — the failure is not caught at formula-save time.
//   - Worse, Evaluator.Run converts NaN/Inf results into (0, nil)
//     (internal/application/costcalc/evaluator/evaluator.go:70-77), so a broken
//     sqrt expression would be swallowed silently and surface as LDR = 0.
//
// Consequence: an mst_formula row for these calculations is read ONLY for its
// formula_code and for building a human-readable calculation_trace. Its
// expression is NEVER executed. That read happens in the application layer;
// this package does not touch mst_formula at all.
//
// NOTE ON STRENGTH ADJUSTMENT (C-3): deliberately NOT implemented. The plan
// withholds it pending MB-department confirmation of the hard-coded "4" in the
// source spreadsheet. No adjust-by-strength function exists here on purpose.
package mbdozing

import (
	"math"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
)

// Calculation modes supported by Calculate.
const (
	// ModeScale scales a reference LDR between denier/filament pairs (formula C-1).
	ModeScale = "SCALE"
	// ModeXSection converts an LDR between cross-section codes (formula C-2).
	ModeXSection = "XSECTION"
)

// ScaleInput carries the operands of the SCALE calculation (C-1).
type ScaleInput struct {
	// LDRRef is the known/recommended LDR of the reference denier/filament pair.
	LDRRef float64
	// DenierRef and FilamentRef describe the reference pair.
	DenierRef   float64
	FilamentRef float64
	// DenierTarget and FilamentTarget describe the pair whose LDR is sought.
	DenierTarget   float64
	FilamentTarget float64
}

// XSectionInput carries the operands of the XSECTION calculation (C-2).
type XSectionInput struct {
	// LDR is the source LDR to convert.
	LDR float64
	// Factor is the ordered (from_code -> to_code) master row. It is required:
	// there is no fallback factor of 1.0.
	Factor *mbcrosssection.FactorEntity
}

// Input is the union of the per-mode inputs, discriminated by Mode.
type Input struct {
	Mode     string
	Scale    ScaleInput
	XSection XSectionInput
}

// Calculate dispatches on in.Mode and returns the resulting LDR.
// An unrecognized mode returns ErrInvalidMode; it never falls through to a default.
func Calculate(in Input) (float64, error) {
	switch in.Mode {
	case ModeScale:
		return ScaleLDR(in.Scale)
	case ModeXSection:
		return ConvertCrossSection(in.XSection.LDR, in.XSection.Factor)
	default:
		return 0, ErrInvalidMode
	}
}

// Ratio returns sqrt(denier / filament) — the C-1 helper (Excel `=SQRT(C6/C7)`).
// Computed with Go's math.Sqrt, never through the expression evaluator; see the
// package comment for why.
func Ratio(denier, filament float64) (float64, error) {
	if filament <= 0 {
		return 0, ErrZeroFilament
	}
	return math.Sqrt(denier / filament), nil
}

// ScaleLDR implements formula C-1:
//
//	ratio(d, f)  = sqrt(d / f)
//	LDR_target   = LDR_ref * ratio_ref / ratio_target
//
// Verified golden case: 380/108 at LDR 0.9 -> 500/96 yields 0.7397296803562773.
func ScaleLDR(in ScaleInput) (float64, error) {
	ratioRef, err := Ratio(in.DenierRef, in.FilamentRef)
	if err != nil {
		return 0, err
	}
	ratioTarget, err := Ratio(in.DenierTarget, in.FilamentTarget)
	if err != nil {
		return 0, err
	}
	if ratioTarget == 0 {
		return 0, ErrZeroFilament
	}
	return in.LDRRef * ratioRef / ratioTarget, nil
}

// ConvertCrossSection implements formula C-2. The direction of the arithmetic
// comes from the master row's operation column (mbcf_operation), NOT from an
// assumption that conversion is always a multiplication: the seeded RND->TBL and
// TBL->RND rows share the same numeric factor and differ only in operation.
// The factor value itself is never hard-coded here.
func ConvertCrossSection(ldr float64, f *mbcrosssection.FactorEntity) (float64, error) {
	if f == nil {
		return 0, ErrFactorNotFound
	}
	if f.Factor() <= 0 {
		return 0, mbcrosssection.ErrFactorNotPositive
	}
	switch f.Operation() {
	case mbcrosssection.OperationMultiply:
		return ldr * f.Factor(), nil
	case mbcrosssection.OperationDivide:
		return ldr / f.Factor(), nil
	default:
		return 0, mbcrosssection.ErrFactorInvalidOperation
	}
}
