package workorder

import (
	"errors"
)

// Specific causes of a failed lot generation.
//
// Before these existed, all three collapsed into ErrLotSpecUnavailable, whose
// message named three possibilities and resolved none: "product item/shade codes
// or standard bobbin weights are unavailable". A planner reading it could not
// tell which master to open, so the only workable response was to guess.
//
// Each cause wraps ErrLotSpecUnavailable, so `errors.Is(err, ErrLotSpecUnavailable)`
// still identifies "a lot could not be generated" for call sites that do not care
// which input was missing.
// Cause phrases. These strings are a CONTRACT, not incidental prose.
//
// BaseResponse carries only a message and a status code, so the frontend has no
// structured cause field to read: it identifies the cause by matching one of
// these phrases in order to attach a link to the master that fixes it
// (goapps-frontend/src/components/ppc/work-order/lot-error-hint.tsx).
//
// Three rules keep that from being fragile:
//
//  1. each phrase appears in EXACTLY ONE rendered message -- no phrase may occur
//     in another cause's text or in any FixHint
//     (TestLotCausePhrases_AreMutuallyExclusive);
//  2. therefore match order cannot change the outcome, so a general case can
//     never shadow a specific one;
//  3. the literal text of each phrase is frozen by
//     TestCausePhraseLiterals_AreFrozen, which asserts them byte-for-byte.
//
// HOW THE TYPESCRIPT COPY IS BOUND -- read this before rewording anything.
// The frontend duplicates these four strings as plain TS literals; there is no
// codegen and no shared JSON, so nothing mechanically binds the two languages.
// Rule 1 alone does NOT protect the frontend: a reword applied consistently
// across the Go side keeps exclusivity intact, so the exclusivity test still
// passes, the frontend's own test still passes against its now-stale copy, and
// production silently loses the link.
//
// Rule 3 is what closes that hole. Rewording a phrase fails
// TestCausePhraseLiterals_AreFrozen, whose failure message names the exact
// frontend file and line range to update in the same change. That makes the
// duplication a DECLARED two-site edit rather than an invisible one.
const (
	// CausePhraseItemShade identifies a missing ERP item/shade code.
	CausePhraseItemShade = "no ERP item code and shade code"
	// CausePhraseStdWeight identifies an unresolved STD_WEIGHT.
	CausePhraseStdWeight = "standard bobbin weight (STD_WEIGHT) is not set"
	// CausePhraseNoProduct identifies a plan item with no finance product.
	CausePhraseNoProduct = "plan item is not linked to a product"
	// CausePhraseLotNotRegistered identifies a manual lot absent from lot_master.
	CausePhraseLotNotRegistered = "lot number is not registered in lot master"
	// CausePhraseGenerationUnavailable identifies a missing lot provisioner --
	// a deployment fault rather than missing master data, but the planner's
	// workaround is the same lot-master action, so it is classified alongside
	// the others rather than left to render no alert at all.
	CausePhraseGenerationUnavailable = "lot number generation is not available"
)

// LotCausePhrases is every cause phrase, for the mutual-exclusivity and
// frozen-literal tests and for anyone auditing the frontend contract.
var LotCausePhrases = []string{
	CausePhraseItemShade,
	CausePhraseStdWeight,
	CausePhraseNoProduct,
	CausePhraseLotNotRegistered,
	CausePhraseGenerationUnavailable,
}

var (
	// ErrLotItemShadeUnavailable is returned when the product's ERP item code or
	// shade code could not be resolved from finance. Without both, a generated
	// lot has no key: lot_master is keyed by item_code + shade_code (PRD §9).
	ErrLotItemShadeUnavailable = errors.New("the product has " + CausePhraseItemShade)
	// ErrLotStdWeightUnavailable is returned when the STD_WEIGHT parameter could
	// not be resolved to a positive value for the product on the chosen machine.
	// The full standard weight is never estimated -- see LotUnfullWeightRatio in
	// lot.go for why only the unfull weight may be.
	ErrLotStdWeightUnavailable = errors.New("the " + CausePhraseStdWeight)
	// ErrLotProductUnavailable is returned when the plan item resolves to no
	// finance product at all, so neither of the other two lookups can even be
	// attempted.
	ErrLotProductUnavailable = errors.New("the " + CausePhraseNoProduct)
)

// LotSpecError names the specific missing input behind a failed lot generation,
// together with the human-readable labels a planner needs to fix it.
//
// It deliberately carries codes and names, never ids: the product is identified
// by its ERP item code or product name and the machine by its machine number, so
// no message can leak an internal id to a user.
type LotSpecError struct {
	// Cause is one of the sentinels above.
	Cause error
	// ProductLabel identifies the product to a human -- its item code or, when
	// that is exactly what is missing, its name. Empty when nothing better than
	// "the selected plan item" is known.
	ProductLabel string
	// MachineLabel identifies the machine to a human (its machine number). Empty
	// when the failure is not machine-specific.
	MachineLabel string
	// FixHint names the page that resolves the problem.
	FixHint string
	// SubjectSuppressed drops the "for <product>" clause entirely. Some causes
	// already name their own subject -- "the plan item is not linked to a
	// product" reads as nonsense followed by "for the selected plan item's
	// product" -- so those set this rather than fighting the template.
	SubjectSuppressed bool
}

// Error renders the full planner-facing sentence: what failed, for which
// product (and machine), and where to fix it.
func (e *LotSpecError) Error() string {
	msg := ErrLotSpecUnavailable.Error() + ": " + e.Cause.Error()
	if !e.SubjectSuppressed {
		subject := "the selected plan item's product"
		if e.ProductLabel != "" {
			subject = "product " + e.ProductLabel
		}
		msg += " for " + subject
	}
	if e.MachineLabel != "" {
		msg += " on machine " + e.MachineLabel
	}
	if e.FixHint != "" {
		msg += ". " + e.FixHint
	}
	return msg
}

// Unwrap exposes both the specific cause and the ErrLotSpecUnavailable wrapper
// to errors.Is, so a caller may match on either level.
func (e *LotSpecError) Unwrap() []error {
	return []error{e.Cause, ErrLotSpecUnavailable}
}

// NewLotItemShadeError builds the "no item/shade codes" failure for a product.
func NewLotItemShadeError(productLabel string) error {
	return &LotSpecError{
		Cause:        ErrLotItemShadeUnavailable,
		ProductLabel: productLabel,
		FixHint:      "Set the ERP item code and shade code on the product master, or enter a lot number already registered in lot master.",
	}
}

// NewLotStdWeightError builds the "no STD_WEIGHT" failure for a product on a
// machine. The machine matters because STD_WEIGHT resolves through the
// product+machine layer before falling back to the product-wide value.
func NewLotStdWeightError(productLabel, machineLabel string) error {
	return &LotSpecError{
		Cause:        ErrLotStdWeightUnavailable,
		ProductLabel: productLabel,
		MachineLabel: machineLabel,
		FixHint:      "Set STD_WEIGHT for this product under Production Plan > Masters > Product Machine Parameters, or on the product's cost parameters.",
	}
}

// NewLotProductError builds the "plan item has no product" failure. Its cause
// already names the plan item, so the product subject is suppressed.
func NewLotProductError() error {
	return &LotSpecError{
		Cause:             ErrLotProductUnavailable,
		SubjectSuppressed: true,
		FixHint:           "Link a product to the plan item, or enter a lot number already registered in lot master.",
	}
}
