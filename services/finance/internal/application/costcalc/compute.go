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

	// 2. Aggregate RM cost across every sequence in the route.
	totalRM, rmDetail, levelMap, err := aggregateRMCost(in)
	if err != nil {
		recordProductSpanError(span, err)
		return nil, err
	}
	scope[ScopeKeyCostRMTotal] = totalRM

	// 3. Evaluate formulas in topo order (loader pre-sorted).
	formulaTrace := make([]FormulaEvalTrace, 0, len(in.Formulas))
	for _, f := range in.Formulas {
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
	finalCost, ok := scopeFloat(scope, ScopeKeyFinalCost)
	if !ok {
		err := fmt.Errorf("compute product %d: formula chain did not produce %s",
			in.ProductSysID, ScopeKeyFinalCost)
		recordProductSpanError(span, err)
		return nil, err
	}
	conv, _ := scopeFloat(scope, ScopeKeyConversion)

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
