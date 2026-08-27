// Package costcalc contains the application-layer logic for the cost calculation engine.
package costcalc

import "errors"

// FormulaTypeRMLookup identifies RM_LOOKUP formulas that must be evaluated by the
// route-aware RM aggregator rather than the expr-lang evaluator.
const FormulaTypeRMLookup = "RM_LOOKUP"

// FormulaTypeMBCostLookup identifies MB_COST_LOOKUP formulas — downstream POY-consumer
// formulas that resolve their result directly from a pre-fetched cst_mb_cost value
// (see ComputeInput.MBCosts) rather than any expr-lang evaluation.
const FormulaTypeMBCostLookup = "MB_COST_LOOKUP"

// Formula is the application-level representation of a single computed parameter
// (mst_formula + mst_formula_param). Loaded in topologically-sorted order so that
// evaluation can simply iterate without further dependency analysis.
type Formula struct {
	FormulaCode     string
	FormulaName     string
	FormulaType     string // e.g. "CALCULATION", "RM_LOOKUP", "FROM_MARKETING"
	Expression      string
	ResultParamCode string   // output param this formula assigns into the scope
	InputParamCodes []string // input params expected in scope before eval
	SortOrder       int      // for stable iteration; topo order pre-applied
}

// FormulaTypeMBXSectionLookup identifies MB_XSECTION_LOOKUP formulas — the LDR
// cross-section conversion (F_MB_LDR_XSECTION, migration 000480) whose factor and
// multiply/divide operation must be looked up in mst_mb_cross_section_factor by
// dedicated Go code. No such code exists in the costcalc pipeline, so this type is
// listed in formulaTypesNeedingGoImpl and is rejected rather than evaluated.
const FormulaTypeMBXSectionLookup = "MB_XSECTION_LOOKUP"

// FormulaTypeLookup identifies the generic LOOKUP type from the mst_formula
// formula_type CHECK constraint. A lookup resolves its value from master data, so
// it can never be evaluated as an expr-lang expression; no costcalc Go code
// implements it, hence it is listed in formulaTypesNeedingGoImpl.
const FormulaTypeLookup = "LOOKUP"

// FormulaTypeSQLQuery identifies the SQL_QUERY type from the mst_formula
// formula_type CHECK constraint. Its expression column holds SQL, not an expr-lang
// expression, and nothing in the costcalc pipeline executes it, hence it is listed
// in formulaTypesNeedingGoImpl.
const FormulaTypeSQLQuery = "SQL_QUERY"

// ErrFormulaTypeNotImplemented is returned when a formula carries a formula_type
// that requires dedicated Go resolution code which the costcalc pipeline does not
// have yet.
//
// Why this guard exists (K-22, 2026-08-22). evalSingleFormulaStep's default branch
// hands anything it does not recognize to the expr-lang evaluator, and that path
// fails SILENTLY rather than loudly for lookup types:
//
//   - evaluator.Compile uses expr.AllowUndefinedVariables(), so an expression
//     referencing variables that no code ever puts in scope (XSECTION_OP,
//     XSECTION_FACTOR, LDR_SOURCE) still compiles.
//   - Evaluator.Run converts NaN/Inf into (0, nil) on purpose.
//
// Combined, F_MB_LDR_XSECTION's stored expression
// 'XSECTION_OP == 1 ? LDR_SOURCE * XSECTION_FACTOR : LDR_SOURCE / XSECTION_FACTOR'
// would yield a plausible-looking 0 with no error at all. Today the only thing
// preventing that is an accident of loading — loadPerProductFormulas only loads
// formulas whose result param has a cost_product_applicable_param row, and 000480
// inserts no CAPP row for MB_LDR_XSECTION. A single future CAPP row would open the
// silent path. This error closes it: wrong-and-loud beats wrong-and-quiet.
var ErrFormulaTypeNotImplemented = errors.New(
	"formula type requires dedicated Go implementation which the costcalc pipeline does not have")

// formulaTypesNeedingGoImpl is the explicit denylist of mst_formula.formula_type
// values that require dedicated Go resolution code but have NO implementation in
// the costcalc pipeline. Membership here makes evalSingleFormulaStep fail fast
// instead of silently falling through to expr-lang.
//
// It is a denylist, not a switch case per value, because the DB CHECK constraint
// grows faster than the Go switch: 000005 shipped 3 values, 000402 widened it to
// 11, 000451 to 12, 000480 to 13. Every widening that adds a lookup-semantics type
// must add one line here — the map is the single place to look.
//
// Deliberately NOT listed (these are correctly evaluated by expr-lang today, and
// this guard must not change any behavior that works):
//
//	CALCULATION, CONDITIONAL, CONSTANT — plain arithmetic/ternary/literal expressions.
//	FROM_MARKETING                     — uses the marketing_result() built-in that
//	                                     injectMarketingResult puts into scope.
//	SNAPSHOT / RM_LOOKUP / MB_COST_LOOKUP — have their own explicit cases above the
//	                                     default branch.
//	PENDING                            — all 13 seeded rows are is_active = FALSE and
//	                                     carry a literal 'TBD' expression, so the
//	                                     loader never returns them and expr-lang would
//	                                     fail the compile loudly if it ever did.
//	INITIAL_VALUE, INTERMINGLING       — no formula row uses either as a formula_type
//	                                     anywhere in migrations/seeds, and their
//	                                     intended semantics are not documented, so
//	                                     classifying them is a costing decision, not
//	                                     a code decision. Left out on purpose.
//
// The value is the reason string surfaced in the error, so the operator learns
// what is missing, not just that something is.
var formulaTypesNeedingGoImpl = map[string]string{
	FormulaTypeMBXSectionLookup: "needs a mst_mb_cross_section_factor lookup (factor + multiply/divide op) — see migration 000480",
	FormulaTypeLookup:           "needs a master-data lookup; its expression is not an expr-lang expression",
	FormulaTypeSQLQuery:         "needs SQL execution; its expression column holds SQL, not an expr-lang expression",
}
