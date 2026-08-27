// Package mbdozing contains the application-layer use cases for MB dozing (LDR).
//
// The layer is READ-ONLY (user decision K-18): it loads master data, calls the
// pure domain calculator, and returns a result. It performs no INSERT, UPDATE or
// DELETE against any table, and it never writes mbwl_meta — that belongs to P8.
package mbdozing

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/formula"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbdozing"
)

// Canonical formula codes for the two supported modes. They are used both to
// look mst_formula up (for the stored formula_code) and as the reported code
// when no master row has been seeded yet.
const (
	// FormulaCodeScale identifies the denier/filament scaling formula (C-1).
	FormulaCodeScale = "F_MB_LDR_SCALE"
	// FormulaCodeXSection identifies the cross-section conversion formula (C-2).
	FormulaCodeXSection = "F_MB_LDR_XSECTION"
)

// Application-layer errors for the calculate use case.
var (
	// ErrMissingScaleInput is returned when a SCALE request omits an operand.
	ErrMissingScaleInput = errors.New("mbdozing: mode SCALE requires ldr_ref, denier_ref, filament_ref, denier_target and filament_target")
	// ErrMissingXSectionInput is returned when an XSECTION request omits an operand.
	ErrMissingXSectionInput = errors.New("mbdozing: mode XSECTION requires ldr_source, from_cross_section and to_cross_section")
)

// CalculateCommand is the mode-discriminated input of the calculate use case.
// The pointer fields mirror the proto's optional fields: nil means "not sent",
// which is distinct from zero.
type CalculateCommand struct {
	Mode string

	// Mode SCALE.
	LDRRef         *float64
	DenierRef      *float64
	FilamentRef    *int32
	DenierTarget   *float64
	FilamentTarget *int32

	// Mode XSECTION.
	LDRSource        *float64
	FromCrossSection *string
	ToCrossSection   *string
}

// CalculateResult is the outcome of the calculate use case.
type CalculateResult struct {
	// ResultLDR is nil when no result could be produced without inventing a
	// number — specifically when the requested cross-section pair has no factor
	// row. Substituting a neutral factor of one is forbidden (constraint D13).
	ResultLDR *float64
	// FormulaCode identifies the formula that describes the calculation.
	FormulaCode string
	// CalculationTrace is a human-readable, fully-substituted rendering of the
	// arithmetic. It is BUILT IN GO, never produced by evaluating an expression.
	CalculationTrace string
	// FactorAvailable is false when the cross-section pair has no factor row.
	// That is a normal outcome, not an error.
	FactorAvailable bool
	// Message explains a false FactorAvailable to the user; empty otherwise.
	Message string
}

// CalculateHandler executes the MB dozing calculation.
type CalculateHandler struct {
	factorRepo  mbcrosssection.FactorRepository
	formulaRepo formula.Repository
}

// NewCalculateHandler constructs a CalculateHandler.
func NewCalculateHandler(factorRepo mbcrosssection.FactorRepository, formulaRepo formula.Repository) *CalculateHandler {
	return &CalculateHandler{factorRepo: factorRepo, formulaRepo: formulaRepo}
}

// Handle dispatches on the requested mode.
//
// Only the two modes the domain implements are accepted. The third mode the
// source spreadsheet hints at is withheld (gate G6-C3) pending MB-department
// confirmation; an unknown mode falls through to the domain's ErrInvalidMode
// rather than being guessed at.
func (h *CalculateHandler) Handle(ctx context.Context, cmd CalculateCommand) (*CalculateResult, error) {
	switch cmd.Mode {
	case mbdozing.ModeScale:
		return h.handleScale(ctx, cmd)
	case mbdozing.ModeXSection:
		return h.handleXSection(ctx, cmd)
	default:
		return nil, mbdozing.ErrInvalidMode
	}
}

// handleScale computes the C-1 scaling entirely in Go.
func (h *CalculateHandler) handleScale(ctx context.Context, cmd CalculateCommand) (*CalculateResult, error) {
	if cmd.LDRRef == nil || cmd.DenierRef == nil || cmd.FilamentRef == nil ||
		cmd.DenierTarget == nil || cmd.FilamentTarget == nil {
		return nil, ErrMissingScaleInput
	}

	in := mbdozing.ScaleInput{
		LDRRef:         *cmd.LDRRef,
		DenierRef:      *cmd.DenierRef,
		FilamentRef:    float64(*cmd.FilamentRef),
		DenierTarget:   *cmd.DenierTarget,
		FilamentTarget: float64(*cmd.FilamentTarget),
	}

	result, err := mbdozing.Calculate(mbdozing.Input{Mode: mbdozing.ModeScale, Scale: in})
	if err != nil {
		return nil, err
	}

	// The mst_formula row, when present, contributes ONLY its code. Its
	// expression is never executed (user decision K-16): expr has no sqrt and
	// Evaluator.Run turns NaN/Inf into (0, nil), so a broken expression would
	// silently surface as LDR 0.
	code := h.resolveFormulaCode(ctx, FormulaCodeScale)

	return &CalculateResult{
		ResultLDR:        &result,
		FormulaCode:      code,
		CalculationTrace: scaleTrace(in, result),
		FactorAvailable:  true,
	}, nil
}

// handleXSection converts an LDR between cross-section codes using the master factor.
func (h *CalculateHandler) handleXSection(ctx context.Context, cmd CalculateCommand) (*CalculateResult, error) {
	if cmd.LDRSource == nil || cmd.FromCrossSection == nil || cmd.ToCrossSection == nil {
		return nil, ErrMissingXSectionInput
	}

	from, to := *cmd.FromCrossSection, *cmd.ToCrossSection
	code := h.resolveFormulaCode(ctx, FormulaCodeXSection)

	factor, err := h.factorRepo.GetByPair(ctx, from, to)
	if err != nil {
		// CONSTRAINT D13 — a pair with no factor row is a NORMAL outcome, not a
		// gRPC error. v1 only supports pairs that have a seeded factor row.
		// result_ldr is left unset; silently substituting a neutral factor of
		// one is forbidden, because a wrong number is worse than no number.
		if errors.Is(err, mbcrosssection.ErrFactorNotFound) {
			return &CalculateResult{
				ResultLDR:       nil,
				FormulaCode:     code,
				FactorAvailable: false,
				Message: fmt.Sprintf(
					"no conversion factor is defined for %s to %s; this cross-section pair is not supported yet, so no LDR can be produced",
					from, to),
			}, nil
		}
		return nil, err
	}

	result, err := mbdozing.Calculate(mbdozing.Input{
		Mode:     mbdozing.ModeXSection,
		XSection: mbdozing.XSectionInput{LDR: *cmd.LDRSource, Factor: factor},
	})
	if err != nil {
		return nil, err
	}

	return &CalculateResult{
		ResultLDR:        &result,
		FormulaCode:      code,
		CalculationTrace: xsectionTrace(*cmd.LDRSource, factor, result),
		FactorAvailable:  true,
	}, nil
}

// resolveFormulaCode reads mst_formula for the canonical code and returns the
// stored code when the row exists. Only the code is taken; the expression column
// is never read into an evaluator. A missing row is not an error — the canonical
// constant is returned so the response still identifies the formula.
func (h *CalculateHandler) resolveFormulaCode(ctx context.Context, canonical string) string {
	if h.formulaRepo == nil {
		return canonical
	}
	c, err := formula.NewCode(canonical)
	if err != nil {
		return canonical
	}
	f, err := h.formulaRepo.GetByCode(ctx, c)
	if err != nil || f == nil {
		return canonical
	}
	return f.Code().String()
}

// num renders a float without trailing zeros, for readable traces.
func num(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

// scaleTrace renders the C-1 arithmetic with every operand substituted.
// It is pure string formatting — no expression is compiled or evaluated.
func scaleTrace(in mbdozing.ScaleInput, result float64) string {
	return fmt.Sprintf(
		"LDR = %s * sqrt(%s / %s) / sqrt(%s / %s) = %s",
		num(in.LDRRef),
		num(in.DenierRef), num(in.FilamentRef),
		num(in.DenierTarget), num(in.FilamentTarget),
		num(result),
	)
}

// xsectionTrace renders the C-2 arithmetic. The operator shown is the master
// row's own operation, not an assumption that conversion always multiplies.
func xsectionTrace(ldr float64, f *mbcrosssection.FactorEntity, result float64) string {
	op := "*"
	if f.Operation() == mbcrosssection.OperationDivide {
		op = "/"
	}
	return fmt.Sprintf(
		"LDR = %s %s %s (%s -> %s) = %s",
		num(ldr), op, num(f.Factor()), f.FromCode(), f.ToCode(), num(result),
	)
}
