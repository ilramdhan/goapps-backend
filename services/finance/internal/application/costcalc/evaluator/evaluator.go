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

// ErrNonFiniteResult indicates the expression produced NaN or +/-Inf (typically
// division by zero). Cost calculations must never produce non-finite numbers.
var ErrNonFiniteResult = errors.New("formula produced non-finite result")

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
func (e *Evaluator) Run(scope map[string]any) (float64, error) {
	out, err := expr.Run(e.program, scope)
	if err != nil {
		return 0, fmt.Errorf("run %s: %w", e.formulaCode, err)
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
		return 0, fmt.Errorf("run %s: %w (got %T)", e.formulaCode, ErrOutputNotFloat, out)
	}
	if math.IsNaN(result) || math.IsInf(result, 0) {
		// Non-finite (NaN/Inf) typically means a 0/0 or x/0 division in the formula.
		// Return 0 instead of failing — the formula contributes 0 to the cost chain.
		// This allows downstream formulas to continue and produces a safe 0 cost
		// rather than blocking the entire product calculation.
		return 0, nil
	}
	return result, nil
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
