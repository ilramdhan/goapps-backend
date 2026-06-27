// Package costcalc contains the application-layer logic for the cost calculation engine.
package costcalc

// FormulaTypeRMLookup identifies RM_LOOKUP formulas that must be evaluated by the
// route-aware RM aggregator rather than the expr-lang evaluator.
const FormulaTypeRMLookup = "RM_LOOKUP"

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
