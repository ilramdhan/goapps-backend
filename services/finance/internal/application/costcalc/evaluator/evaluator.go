// Package evaluator wraps github.com/expr-lang/expr for safe, cached formula evaluation.
package evaluator

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// ErrUnsafeFunction indicates the expression references a denylisted identifier.
var ErrUnsafeFunction = errors.New("unsafe identifier in formula")

// ErrOutputNotFloat indicates the expression evaluated to a non-numeric value.
var ErrOutputNotFloat = errors.New("formula did not return float64")

// ErrNonFiniteResult names the condition "the expression produced NaN or
// +/-Inf" (typically a 0/0 or x/0 division in the formula).
//
// IMPORTANT — this sentinel is deliberately NOT returned by Run. Run swallows
// non-finite results and yields (0, nil) instead; see the comment in Run for
// why. The sentinel is kept because it is the vocabulary for the condition and
// because turning the swallow into a hard error is a live option — but making
// that switch changes every costing number that currently rides on a
// fabricated 0, so it is a deliberate product decision, not a refactor.
// Until that decision is taken, callers detect the condition through
// RunWithDiag's NonFiniteKind return value, not through this error.
var ErrNonFiniteResult = errors.New("formula produced non-finite result")

// NonFiniteKind classifies a non-finite evaluation result. The three cases are
// kept apart on purpose: -Inf collapsing to +0 loses the sign and is a
// different class of defect from a 0/0 NaN, and conflating them in a metric
// label would hide that.
type NonFiniteKind string

// Non-finite classifications. NonFiniteNone is the zero value, so a trace or
// diagnostic that was never touched reads as "finite" without extra handling.
const (
	NonFiniteNone   NonFiniteKind = ""
	NonFiniteNaN    NonFiniteKind = "nan"
	NonFinitePosInf NonFiniteKind = "pos_inf"
	NonFiniteNegInf NonFiniteKind = "neg_inf"
)

// String returns the label form used for metrics and JSON.
func (k NonFiniteKind) String() string { return string(k) }

// forbiddenPrefixes are identifier prefixes that may indicate sandbox escape attempts.
// expr's stdlib doesn't expose these by default, but we belt-and-suspenders reject them.
var forbiddenPrefixes = []string{"os.", "exec.", "file.", "syscall.", "io.", "net.", "http.", "runtime.", "reflect."}

// Evaluator is a compiled, reusable formula program.
type Evaluator struct {
	program     *vm.Program
	formulaCode string
	expression  string
}

// Compile validates and compiles a formula expression. Use the cache to avoid
// re-compiling the same expression on every call.
func Compile(formulaCode, expression string) (*Evaluator, error) {
	if err := preCheck(expression); err != nil {
		return nil, err
	}
	prog, err := expr.Compile(
		expression,
		expr.AllowUndefinedVariables(),
		expr.AsFloat64(),
	)
	if err != nil {
		return nil, fmt.Errorf("compile %s: %w", formulaCode, err)
	}
	return &Evaluator{program: prog, formulaCode: formulaCode, expression: expression}, nil
}

// Run executes the compiled program with the provided variable scope.
// All scope values must be numeric (int, float, or convertible) — expr will coerce.
// Returns the float64 result or an error.
//
// Run is the behavior-preserving wrapper over RunWithDiag: it discards the
// non-finite classification. Callers that want to observe the swallowed
// NaN/Inf (metrics, logs, persisted trace) must call RunWithDiag instead.
func (e *Evaluator) Run(scope map[string]any) (float64, error) {
	result, _, err := e.RunWithDiag(scope)
	return result, err
}

// RunWithDiag executes the compiled program and additionally reports whether
// the raw result was non-finite, and of which kind.
//
// The returned float64 is IDENTICAL to what Run returns for every input — this
// method adds observability only, never a numeric difference. When the kind is
// anything other than NonFiniteNone, the returned value is 0.
//
// The evaluator package stays free of logger and metrics dependencies on
// purpose: it hands the classification back to the caller (compute.go), which
// owns the observability wiring. That keeps this package a pure, trivially
// testable function of its inputs.
func (e *Evaluator) RunWithDiag(scope map[string]any) (float64, NonFiniteKind, error) {
	out, err := expr.Run(e.program, scope)
	if err != nil {
		return 0, NonFiniteNone, fmt.Errorf("run %s: %w", e.formulaCode, err)
	}
	var result float64
	switch v := out.(type) {
	case float64:
		result = v
	case int:
		result = float64(v)
	case int64:
		result = float64(v)
	default:
		return 0, NonFiniteNone, fmt.Errorf("run %s: %w (got %T)", e.formulaCode, ErrOutputNotFloat, out)
	}
	if kind := classifyNonFinite(result); kind != NonFiniteNone {
		// Non-finite (NaN/Inf) typically means a 0/0 or x/0 division in the formula.
		// Return 0 instead of failing — the formula contributes 0 to the cost chain.
		// This allows downstream formulas to continue and produces a safe 0 cost
		// rather than blocking the entire product calculation.
		//
		// This 0 is a FABRICATED value, indistinguishable by magnitude from a
		// genuinely computed 0. The kind returned alongside it is the only way
		// to tell the two apart — do not drop it silently.
		return 0, kind, nil
	}
	return result, NonFiniteNone, nil
}

// classifyNonFinite maps a raw result to its non-finite kind, separating the
// two infinity signs. math.IsInf(v, 0) matches both signs at once, which is
// exactly the conflation this function exists to undo.
func classifyNonFinite(v float64) NonFiniteKind {
	switch {
	case math.IsNaN(v):
		return NonFiniteNaN
	case math.IsInf(v, 1):
		return NonFinitePosInf
	case math.IsInf(v, -1):
		return NonFiniteNegInf
	default:
		return NonFiniteNone
	}
}

// FormulaCode returns the code this evaluator was compiled for.
func (e *Evaluator) FormulaCode() string { return e.formulaCode }

// Expression returns the source expression text.
func (e *Evaluator) Expression() string { return e.expression }

// preCheck rejects expressions containing forbidden identifier prefixes.
func preCheck(expression string) error {
	for _, p := range forbiddenPrefixes {
		if strings.Contains(expression, p) {
			return fmt.Errorf("%w: contains %q", ErrUnsafeFunction, p)
		}
	}
	return nil
}
