package costcalc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/mutugading/goapps-backend/pkg/costcalc/metrics"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// tracerName is the instrumentation scope for cost-calc compute spans.
const tracerName = "finance-service"

// Span names for the compute-level trace hierarchy.
const (
	spanCostCalcProduct     = "cost_calc.product"
	spanCostCalcFormulaEval = "cost_calc.formula_eval"
)

// Reserved scope keys produced by ComputeProduct before formula evaluation.
const (
	// ScopeKeyCostRMTotal carries the aggregated RM cost (sum of unit x ratio)
	// into the formula evaluator. Formulas usually reference this as the
	// starting point for cost calculations.
	ScopeKeyCostRMTotal = "COST_RM_TOTAL"
	// ScopeKeyFinalCost is the param code that the last formula in the topo
	// chain MUST assign into. ComputeProduct returns scope[ScopeKeyFinalCost]
	// as the product's cost-per-unit.
	ScopeKeyFinalCost = "COST_STAGE_OUT"
	// ScopeKeyConversion is read out at the end as the total conversion cost
	// (labor, overhead). Optional, defaults to 0 if missing.
	ScopeKeyConversion = "COST_CONVERSION"
)

// ComputeInput aggregates everything ComputeProduct needs for one product.
// All fields are pre-loaded by the chunk processor (S8b.7) via the bulk
// loader (S8b.5); ComputeProduct itself performs no I/O.
type ComputeInput struct {
	ProductSysID  int64
	Period        string
	CalcType      costcalcdom.CalculationType
	Route         *costroute.Graph
	CAPP          map[string]float64
	Formulas      []Formula
	RMCosts       map[string]float64 // key matches loader.LoadRMCosts: "<rmCode>|<itemCode>"
	UpstreamCosts map[int64]float64
	EvalCache     *evaluator.Cache
	// SellingSnapshot holds param values from the SELLING session for this product+period.
	// Used to implement marketing_result() built-in. Empty map when no SELLING result exists.
	SellingSnapshot map[string]float64
}

// RMCostDetail records one RM line's contribution to the total RM cost.
type RMCostDetail struct {
	RouteLevel   int32   `json:"route_level"`
	RMType       string  `json:"rm_type"`
	RefCode      string  `json:"ref_code"`
	ShadeCode    string  `json:"shade_code,omitempty"`
	UnitCost     float64 `json:"unit_cost"`
	Ratio        float64 `json:"ratio"`
	Contribution float64 `json:"contribution"`
}

// FormulaEvalTrace records one formula's evaluation step for diagnostics.
type FormulaEvalTrace struct {
	FormulaCode     string             `json:"formula_code"`
	Expression      string             `json:"expression"`
	Inputs          map[string]float64 `json:"inputs"`
	ResultParamCode string             `json:"result_param_code"`
	Output          float64            `json:"output"`
}

// LevelContribution rolls up contributions per route level.
type LevelContribution struct {
	Level      int32   `json:"level"`
	RMCost     float64 `json:"rm_cost"`
	Conversion float64 `json:"conversion"`
}

// ComputeOutput is the result of one product compute pass.
type ComputeOutput struct {
	CostPerUnit     float64
	TotalRMCost     float64
	TotalConversion float64
	TotalCost       float64
	RMCostDetail    []RMCostDetail
	ParamSnapshot   map[string]float64
	FormulaTrace    []FormulaEvalTrace
	CostByLevel     []LevelContribution
	InputHash       string
}

// ComputeProduct executes the cost calculation for one product. Pure function;
// safe to invoke concurrently across products provided each call gets its own
// ComputeInput. The evaluator cache is internally synchronized.
func ComputeProduct(ctx context.Context, in ComputeInput) (*ComputeOutput, error) {
	start := time.Now()
	defer func() {
		metrics.ProductComputeSeconds.Observe(time.Since(start).Seconds())
	}()

	// Start the per-product span. When tracing is disabled this is the no-op
	// tracer and adds no allocation beyond the cheap Start/End calls.
	ctx, span := otel.Tracer(tracerName).Start(ctx, spanCostCalcProduct, trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	span.SetAttributes(attribute.Int64("product_sys_id", in.ProductSysID))
	if in.Route != nil && in.Route.Head != nil {
		span.SetAttributes(attribute.Int64("route_head_id", in.Route.Head.HeadID))
	}

	if in.Route == nil {
		err := fmt.Errorf("compute product %d: route is nil", in.ProductSysID)
		recordProductSpanError(span, err)
		return nil, err
	}
	if in.EvalCache == nil {
		err := fmt.Errorf("compute product %d: eval cache is nil", in.ProductSysID)
		recordProductSpanError(span, err)
		return nil, err
	}

	// 1. Initialize scope from CAPP values.
	scope := make(map[string]any, len(in.CAPP)+len(in.Formulas)+8)
	for k, v := range in.CAPP {
		scope[k] = v
	}

	// 1b. Pre-fill missing formula input params with 0 so expr-lang never sees nil.
	// AllowUndefinedVariables() returns nil for absent vars, causing arithmetic
	// panics like "<nil> > int". Defaulting to 0 is safe: conditional formulas
	// (e.g. VB_QTY > 0 ? X/VB_QTY : 0) will take the zero branch, and additive
	// formulas produce 0 contributions rather than crashing.
	// Also pre-fill the formula's own ResultParamCode: some formulas (e.g.
	// F_YARN_SPECIAL_COST_FLAG_PASS with expression "SPECIAL_COST_FLAG") read
	// their own result param but declare no explicit InputParamCodes entries.
	for _, f := range in.Formulas {
		if f.FormulaType == FormulaTypeRMLookup {
			continue // RM_LOOKUP handled separately below
		}
		for _, code := range f.InputParamCodes {
			if _, exists := scope[code]; !exists {
				scope[code] = float64(0)
			}
		}
		// Ensure the result param itself is 0-defaulted for pass-through formulas.
		if _, exists := scope[f.ResultParamCode]; !exists {
			scope[f.ResultParamCode] = float64(0)
		}
	}

	// Inject marketing_result() built-in function.
	// Priority: (1) SELLING session snapshot, (2) existing CAPP scope value, (3) 0.
	// Falling back to CAPP preserves the imported param value when no SELLING session
	// has run yet — prevents FROM_MARKETING formulas from zeroing out user-supplied data.
	// Signature matches expr-lang's Function type alias: func(...any) (any, error).
	sellingSnap := in.SellingSnapshot
	scope["marketing_result"] = func(args ...any) (any, error) {
		// args: product (any), paramCode (string), period (any) — matches expr call signature.
		if len(args) < 2 {
			return float64(0), nil
		}
		paramCode, ok := args[1].(string)
		if !ok {
			return float64(0), nil
		}
		if v, found := sellingSnap[paramCode]; found {
			return v, nil
		}
		// Fallback to CAPP scope value — preserves imported param when no SELLING session exists.
		if existing, found := scope[paramCode]; found {
			if fv, ok2 := existing.(float64); ok2 {
				return fv, nil
			}
		}
		return float64(0), nil
	}

	// 2. Aggregate RM cost across every sequence in the route.
	totalRM, rmDetail, levelMap, err := aggregateRMCost(in)
	if err != nil {
		recordProductSpanError(span, err)
		return nil, err
	}
	scope[ScopeKeyCostRMTotal] = totalRM

	// 3. Evaluate formulas in topo order (loader pre-sorted).
	// RM_LOOKUP formulas use a custom Oracle DSL that expr-lang cannot parse.
	// They compute "sum of RM costs for current level" — identical to what
	// aggregateRMCost() already computed as COST_RM_TOTAL. Phase-1 approximation:
	// alias totalRM into the formula's ResultParamCode so downstream CALCULATION
	// formulas can proceed. Phase-2 will implement per-pricing-type splitting.
	formulaTrace := make([]FormulaEvalTrace, 0, len(in.Formulas))
	for _, f := range in.Formulas {
		if f.FormulaType == "SNAPSHOT" {
			// SNAPSHOT formulas capture a value at a point in time.
			// Read the referenced param from scope (already computed) and echo it.
			// Expression format: "snapshot(PARAM_CODE) at process start"
			// For now: treat as a pass-through read of the first input param.
			val := float64(0)
			if len(f.InputParamCodes) > 0 {
				if v, ok := scope[f.InputParamCodes[0]]; ok {
					if fv, ok2 := v.(float64); ok2 {
						val = fv
					}
				}
			} else if v, ok := scope[f.ResultParamCode]; ok {
				if fv, ok2 := v.(float64); ok2 {
					val = fv
				}
			}
			scope[f.ResultParamCode] = val
			formulaTrace = append(formulaTrace, FormulaEvalTrace{
				FormulaCode:     f.FormulaCode,
				Expression:      f.Expression,
				ResultParamCode: f.ResultParamCode,
				Output:          val,
			})
			continue
		}
		if f.FormulaType == FormulaTypeRMLookup {
			// Phase-1: RM_LOOKUP → alias totalRM into result param.
			scope[f.ResultParamCode] = totalRM
			formulaTrace = append(formulaTrace, FormulaEvalTrace{
				FormulaCode:     f.FormulaCode,
				Expression:      f.Expression,
				ResultParamCode: f.ResultParamCode,
				Output:          totalRM,
				Inputs:          map[string]float64{"COST_RM_TOTAL": totalRM},
			})
			continue
		}
		formulaResult, evalErr := evalOneFormula(ctx, in.EvalCache, f, scope)
		if evalErr != nil {
			wrapped := fmt.Errorf("%w: %s for product %d: %w",
				costcalcdom.ErrFormulaEval, f.FormulaCode, in.ProductSysID, evalErr)
			recordProductSpanError(span, wrapped)
			return nil, wrapped
		}
		formulaTrace = append(formulaTrace, formulaResult)
		scope[f.ResultParamCode] = formulaResult.Output
	}

	// 4. Extract final cost + optional conversion.
	//
	// Resolution order:
	//   a) scope["COST_STAGE_OUT"] — explicit terminal sink (yarn / multi-formula products)
	//   b) sole terminal formula — formula whose output is not consumed by any
	//      other formula in this product's set (simple / test products)
	//   c) COST_RM_TOTAL — product has no formulas at all (pure RM cost)
	finalCost, ok := scopeFloat(scope, ScopeKeyFinalCost)
	if !ok {
		fc, fcErr := resolveFinalCost(in, scope, totalRM)
		if fcErr != nil {
			recordProductSpanError(span, fcErr)
			return nil, fcErr
		}
		finalCost = fc
	}
	conv, _ := scopeFloat(scope, ScopeKeyConversion)
	// If no formula explicitly writes COST_CONVERSION, derive it from final - RM.
	// This gives accurate conversion reporting even when the formula chain does not
	// produce an explicit COST_CONVERSION output.
	if conv == 0 && finalCost > totalRM {
		conv = finalCost - totalRM
	}

	span.SetAttributes(attribute.String("status", "success"))
	return &ComputeOutput{
		CostPerUnit:     finalCost,
		TotalRMCost:     totalRM,
		TotalConversion: conv,
		TotalCost:       finalCost,
		RMCostDetail:    rmDetail,
		ParamSnapshot:   scopeSnapshot(scope),
		FormulaTrace:    formulaTrace,
		CostByLevel:     buildCostByLevel(levelMap, conv),
		InputHash:       inputHash(in, totalRM),
	}, nil
}

// aggregateRMCost iterates every sequence in the route and sums the RM
// contributions. Note: per the costroute design the level-1 seq produces the
// FG; we treat every seq as contributing because intermediate seqs feed the FG
// via their RMs (the formula chain rolls them up — see S8b.7 chunk processor).
func aggregateRMCost(in ComputeInput) (float64, []RMCostDetail, map[int32]float64, error) {
	var totalRM float64
	detail := []RMCostDetail{}
	byLevel := map[int32]float64{}

	for _, seq := range in.Route.Seqs {
		if seq == nil {
			continue
		}
		// Only sequences producing the target product contribute to its
		// per-unit cost. Upstream sequences feed in via UpstreamCosts when
		// their FG product is referenced as a PRODUCT-type RM.
		if seq.ProductSysID != in.ProductSysID {
			continue
		}
		level := seq.RouteLevel
		for _, rm := range seq.Rms {
			if rm == nil {
				continue
			}
			unit, err := resolveRMUnitCost(in, rm)
			if err != nil {
				return 0, nil, nil, fmt.Errorf("product %d level %d: %w", in.ProductSysID, level, err)
			}
			contribution := unit * rm.RouteRmRatio
			totalRM += contribution
			byLevel[level] += contribution
			detail = append(detail, RMCostDetail{
				RouteLevel:   level,
				RMType:       rm.RmType,
				RefCode:      rmRefCode(rm),
				ShadeCode:    rm.RouteRmShadeCode,
				UnitCost:     unit,
				Ratio:        rm.RouteRmRatio,
				Contribution: contribution,
			})
		}
	}
	return totalRM, detail, byLevel, nil
}

// resolveRMUnitCost picks the per-unit cost for a single RM line based on its
// discriminator. Returns a wrapped sentinel error so the chunk processor can
// classify the product as BLOCKED.
func resolveRMUnitCost(in ComputeInput, rm *costroute.Rm) (float64, error) {
	switch rm.RmType {
	case costroute.RmTypeProduct:
		cost, ok := in.UpstreamCosts[rm.RmProductSysID]
		if !ok {
			return 0, fmt.Errorf("%w: upstream product %d", costcalcdom.ErrMissingUpstreamCost, rm.RmProductSysID)
		}
		return cost, nil
	case costroute.RmTypeItem:
		key := rm.RmItemCode + "|"
		cost, ok := in.RMCosts[key]
		if !ok {
			return 0, fmt.Errorf("%w: item %s", costcalcdom.ErrMissingRMCost, rm.RmItemCode)
		}
		return cost, nil
	case costroute.RmTypeGroup:
		key := rm.RmGroupCode + "|"
		cost, ok := in.RMCosts[key]
		if !ok {
			return 0, fmt.Errorf("%w: group %s", costcalcdom.ErrMissingRMCost, rm.RmGroupCode)
		}
		return cost, nil
	default:
		return 0, fmt.Errorf("unknown RM type %q", rm.RmType)
	}
}

func rmRefCode(rm *costroute.Rm) string {
	switch rm.RmType {
	case costroute.RmTypeProduct:
		return fmt.Sprintf("product:%d", rm.RmProductSysID)
	case costroute.RmTypeItem:
		return rm.RmItemCode
	case costroute.RmTypeGroup:
		return rm.RmGroupCode
	default:
		return ""
	}
}

// recordProductSpanError marks the product span failed and tags it with the
// error status. The blocked-vs-failed nuance is classified by the caller
// (recordComputeError); here we only distinguish success from non-success.
func recordProductSpanError(span trace.Span, err error) {
	span.RecordError(err)
	span.SetAttributes(attribute.String("status", "error"))
}

func evalOneFormula(ctx context.Context, cache *evaluator.Cache, f Formula, scope map[string]any) (FormulaEvalTrace, error) {
	start := time.Now()
	defer func() {
		metrics.FormulaEvalSeconds.WithLabelValues(f.FormulaCode).Observe(time.Since(start).Seconds())
	}()

	// Formula evaluation is a hot path (thousands/sec). Only allocate a span
	// when the parent is actually recording (i.e. the trace is sampled),
	// otherwise skip span creation entirely for zero overhead.
	if trace.SpanFromContext(ctx).IsRecording() {
		_, span := otel.Tracer(tracerName).Start(ctx, spanCostCalcFormulaEval,
			trace.WithAttributes(attribute.String("formula_code", f.FormulaCode)))
		defer span.End()
	}

	prevSize := cache.Size()
	ev, err := cache.GetOrCompile(f.FormulaCode, f.Expression)
	if err != nil {
		return FormulaEvalTrace{}, err
	}
	if cache.Size() > prevSize {
		metrics.RecordEvalCacheMiss()
		metrics.EvalCacheEntries.Set(float64(cache.Size()))
	} else {
		metrics.RecordEvalCacheHit()
	}
	out, err := ev.Run(scope)
	if err != nil {
		return FormulaEvalTrace{}, err
	}
	return FormulaEvalTrace{
		FormulaCode:     f.FormulaCode,
		Expression:      f.Expression,
		Inputs:          pickFormulaInputs(f, scope),
		ResultParamCode: f.ResultParamCode,
		Output:          out,
	}, nil
}

func pickFormulaInputs(f Formula, scope map[string]any) map[string]float64 {
	out := make(map[string]float64, len(f.InputParamCodes))
	for _, code := range f.InputParamCodes {
		if v, ok := scopeFloat(scope, code); ok {
			out[code] = v
		}
	}
	return out
}

func scopeFloat(scope map[string]any, key string) (float64, bool) {
	v, ok := scope[key]
	if !ok {
		return 0, false
	}
	return toFloat(v)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func scopeSnapshot(scope map[string]any) map[string]float64 {
	out := make(map[string]float64, len(scope))
	for k, v := range scope {
		if f, ok := toFloat(v); ok {
			out[k] = f
		}
	}
	return out
}

func buildCostByLevel(byLevel map[int32]float64, totalConv float64) []LevelContribution {
	levels := make([]int32, 0, len(byLevel))
	for l := range byLevel {
		levels = append(levels, l)
	}
	slices.Sort(levels)

	out := make([]LevelContribution, 0, len(levels))
	for _, l := range levels {
		out = append(out, LevelContribution{
			Level:      l,
			RMCost:     byLevel[l],
			Conversion: 0,
		})
	}
	// Conversion cost is assigned to the FG (lowest-level=1) seq for now; a
	// future per-level OH allocation can replace this once modeled.
	if totalConv > 0 && len(out) > 0 {
		out[0].Conversion = totalConv
	}
	return out
}

// findTerminalFormula returns the single formula whose result param is not
// consumed as an input by any other formula in the set — i.e. the DAG sink.
// Used when a product's formula chain does not explicitly produce COST_STAGE_OUT.
//
// When multiple sinks exist (e.g. CAP_FINAL + VB1_DEL…VB5_DEL), the function
// picks the terminal with the deepest computation chain (most formula ancestors).
// This is robust to product type changes and param renames: the "primary" cost
// formula naturally has more intermediate steps feeding it than variant/helper
// terminals like OIL_GAIN or VOLUME_BUCKET_X_DEL_COST.
func findTerminalFormula(formulas []Formula) (*Formula, error) {
	// Build set of all params consumed as inputs (to identify terminals).
	allInputs := make(map[string]bool, len(formulas)*2)
	for _, f := range formulas {
		for _, inp := range f.InputParamCodes {
			allInputs[inp] = true
		}
	}

	// Map resultParamCode → formula for depth traversal.
	byResult := make(map[string]*Formula, len(formulas))
	for i := range formulas {
		byResult[formulas[i].ResultParamCode] = &formulas[i]
	}

	// computeDepth returns the length of the longest formula ancestor chain.
	// Memoised via depthCache to avoid re-traversal.
	depthCache := make(map[string]int, len(formulas))
	var computeDepth func(code string) int
	computeDepth = func(resultCode string) int {
		if v, ok := depthCache[resultCode]; ok {
			return v
		}
		f, ok := byResult[resultCode]
		if !ok {
			depthCache[resultCode] = 0
			return 0
		}
		maxParent := 0
		for _, inp := range f.InputParamCodes {
			if d := computeDepth(inp); d > maxParent {
				maxParent = d
			}
		}
		depth := maxParent + 1
		depthCache[resultCode] = depth
		return depth
	}

	var terminals []Formula
	for _, f := range formulas {
		if !allInputs[f.ResultParamCode] {
			terminals = append(terminals, f)
		}
	}

	switch len(terminals) {
	case 1:
		return &terminals[0], nil
	case 0:
		return nil, fmt.Errorf("formula DAG has no terminal node (cycle or empty set)")
	default:
		// Multiple terminals: pick the one with the deepest computation chain.
		// Ties broken by FormulaCode for determinism.
		best := &terminals[0]
		bestDepth := computeDepth(best.ResultParamCode)
		for i := 1; i < len(terminals); i++ {
			d := computeDepth(terminals[i].ResultParamCode)
			if d > bestDepth || (d == bestDepth && terminals[i].FormulaCode < best.FormulaCode) {
				best = &terminals[i]
				bestDepth = d
			}
		}
		return best, nil
	}
}

// resolveFinalCost determines the final cost when COST_STAGE_OUT is absent from scope.
// Pure-RM products (no formulas) return totalRM; formula products infer the DAG sink.
func resolveFinalCost(in ComputeInput, scope map[string]any, totalRM float64) (float64, error) {
	if len(in.Formulas) == 0 {
		return totalRM, nil
	}
	terminal, termErr := findTerminalFormula(in.Formulas)
	if termErr != nil {
		return 0, fmt.Errorf("%w: product %d: %w", costcalcdom.ErrFormulaEval, in.ProductSysID, termErr)
	}
	fc, _ := scopeFloat(scope, terminal.ResultParamCode)
	return fc, nil
}

func inputHash(in ComputeInput, totalRM float64) string {
	h := sha256.New()
	if _, e := fmt.Fprintf(h, "p:%d|period:%s|type:%s|rm:%.6f|cappN:%d|fN:%d",
		in.ProductSysID, in.Period, in.CalcType, totalRM, len(in.CAPP), len(in.Formulas)); e != nil {
		_ = e
	}
	// Sort CAPP keys for deterministic hash.
	cappKeys := make([]string, 0, len(in.CAPP))
	for k := range in.CAPP {
		cappKeys = append(cappKeys, k)
	}
	slices.Sort(cappKeys)
	for _, k := range cappKeys {
		if _, e := fmt.Fprintf(h, "|capp:%s=%.6f", k, in.CAPP[k]); e != nil {
			_ = e
		}
	}
	for _, f := range in.Formulas {
		if _, e := fmt.Fprintf(h, "|f:%s=%s", f.FormulaCode, f.Expression); e != nil {
			_ = e
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}
