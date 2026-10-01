package costcalc

import (
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// Product type codes (cost_product_type.cpt_type_code) the VB loss rule
// branches on.
const (
	// productTypeCodePOY is the only stage that evaluates F_YARN_VB1..5_LOSS.
	productTypeCodePOY = "POY"
	// productTypeCodeMB is the MB (masterbatch pigment) type. MB is not a yarn
	// stage: MB products keep the legacy behavior, and MB-typed upstream RMs
	// never contribute to a downstream product's inherited VB loss.
	productTypeCodeMB = "MB"
)

// vbLossParamCodes are the five volume-bucket change-over loss params (TOP
// 113-117, VB1-L..VB5-L), in bucket order. They are produced by
// F_YARN_VB1..5_LOSS (expression per migration 000516) and consumed only by
// F_YARN_VB1..5_DEL (VOLUME_BUCKET_n_DEL_COST = DELIVERY_COST_QLTY_LOSS +
// VOLUME_BUCKET_n_LOSS).
//
// Business rule (user decision 2026-10-01): the change-over loss is a POY
// spinning concept, so it is computed at the POY stage only and every later
// route stage (DTY, FDY, PTY, ..., FG) carries the POY value forward instead
// of recomputing it with its own machine inputs. The list is a Go constant on
// purpose — it changes rarely enough that master-data configuration is not
// worth the complexity.
var vbLossParamCodes = [5]string{
	"VOLUME_BUCKET_1_LOSS",
	"VOLUME_BUCKET_2_LOSS",
	"VOLUME_BUCKET_3_LOSS",
	"VOLUME_BUCKET_4_LOSS",
	"VOLUME_BUCKET_5_LOSS",
}

// VBLossInheritance carries what ComputeProduct needs to decide whether a
// product computes its VB loss params or inherits them from upstream. Loaded
// once per chunk by bulkLoad (LoadProductTypeCodes + LoadUpstreamParamSnapshots).
//
// A nil *VBLossInheritance on ComputeInput disables the rule entirely and keeps
// the pre-rule behavior (every product with the formulas evaluates them). That
// is what mbbatch and callers that predate the rule get.
type VBLossInheritance struct {
	// ProductTypeCode is this product's cost_product_type.cpt_type_code; ""
	// when the loader found none.
	ProductTypeCode string
	// UpstreamTypeCodes maps product sys id -> cpt_type_code for (at least)
	// every upstream PRODUCT-type RM of the chunk. Used to skip MB upstreams.
	UpstreamTypeCodes map[int64]string
	// UpstreamParamSnapshots maps upstream product sys id -> its committed
	// cpc_param_snapshot for the same period + calc type. An upstream with no
	// committed row is absent.
	UpstreamParamSnapshots map[int64]map[string]float64
}

// applyInheritedVBLoss implements the POY-only VB loss rule for one product.
//
//   - nil inheritance context, POY, or MB: returns nil — the VB loss formulas
//     run exactly as before.
//   - unknown type code (""): returns nil and logs a warning. Computing is the
//     safe fallback: it reproduces the pre-rule number instead of silently
//     zeroing or inheriting for a product whose stage we cannot identify.
//   - any other type: writes the ratio-weighted average of the direct upstream
//     PRODUCT RMs' snapshot values into scope for each of the five params,
//     clears them from zeroFilled (so they are persisted in ParamSnapshot even
//     when this product's CAPP lacks the params — that is what lets FG <- DTY
//     <- POY chain transitively), and returns the set of param codes whose
//     producing formulas evalFormulaChain must skip.
func applyInheritedVBLoss(in ComputeInput, scope map[string]any, zeroFilled map[string]bool) map[string]bool {
	vb := in.VBLoss
	if vb == nil {
		return nil
	}
	typeCode := strings.ToUpper(strings.TrimSpace(vb.ProductTypeCode))
	switch typeCode {
	case "":
		log.Warn().
			Int64("product_sys_id", in.ProductSysID).
			Str("period", in.Period).
			Msg("VB loss rule: product type code not found; computing VOLUME_BUCKET_n_LOSS with this product's own formulas (pre-rule behavior)")
		return nil
	case productTypeCodePOY, productTypeCodeMB:
		return nil
	}

	values := inheritedVBLossValues(in, vb)
	inherited := make(map[string]bool, len(vbLossParamCodes))
	for i, code := range vbLossParamCodes {
		scope[code] = values[i]
		delete(zeroFilled, code)
		inherited[code] = true
	}
	return inherited
}

// inheritedVBLossValues computes, per bucket, Σ(upstream value × ratio) / Σ
// ratio over this product's own seqs' PRODUCT-type RMs, skipping MB-typed
// upstreams. A missing upstream snapshot or key counts as 0 but keeps its
// ratio weight, and is logged. No eligible upstream RM yields all zeros.
func inheritedVBLossValues(in ComputeInput, vb *VBLossInheritance) [5]float64 {
	var weighted [5]float64
	var totalRatio float64
	for _, rm := range upstreamProductRMs(in) {
		if strings.EqualFold(strings.TrimSpace(vb.UpstreamTypeCodes[rm.RmProductSysID]), productTypeCodeMB) {
			continue
		}
		totalRatio += rm.RouteRmRatio
		snap, hasSnap := vb.UpstreamParamSnapshots[rm.RmProductSysID]
		for i, code := range vbLossParamCodes {
			v, ok := snap[code]
			if !ok {
				warnMissingUpstreamVBLoss(in, rm.RmProductSysID, code, hasSnap)
				continue
			}
			weighted[i] += v * rm.RouteRmRatio
		}
	}
	var out [5]float64
	if totalRatio == 0 {
		return out
	}
	for i := range weighted {
		out[i] = weighted[i] / totalRatio
	}
	return out
}

// upstreamProductRMs returns the PRODUCT-type RM lines of the seqs that
// produce in.ProductSysID — the same seq filter aggregateRMCost applies, so
// the inherited value is weighted by exactly the ratios that feed this
// product's RM cost.
func upstreamProductRMs(in ComputeInput) []*costroute.Rm {
	if in.Route == nil {
		return nil
	}
	var out []*costroute.Rm
	for _, seq := range in.Route.Seqs {
		if seq == nil || seq.ProductSysID != in.ProductSysID {
			continue
		}
		for _, rm := range seq.Rms {
			if rm != nil && rm.RmType == costroute.RmTypeProduct {
				out = append(out, rm)
			}
		}
	}
	return out
}

// warnMissingUpstreamVBLoss logs one upstream VB loss value that was taken as
// 0 because the upstream had no committed snapshot (or the key was absent).
func warnMissingUpstreamVBLoss(in ComputeInput, upstreamID int64, code string, hasSnap bool) {
	reason := "upstream snapshot has no such key"
	if !hasSnap {
		reason = "upstream has no committed cost row for this period/calc type"
	}
	log.Warn().
		Int64("product_sys_id", in.ProductSysID).
		Int64("upstream_product_sys_id", upstreamID).
		Str("period", in.Period).
		Str("calc_type", string(in.CalcType)).
		Str("param_code", code).
		Str("reason", reason).
		Msg("VB loss rule: inherited upstream value missing, counted as 0 (ratio weight kept)")
}

// inheritedVBLossExpression is the trace Expression recorded in place of a
// skipped F_YARN_VBn_LOSS evaluation, so cpc_formula_trace shows why the value
// differs from what the formula would have produced on this stage.
const inheritedVBLossExpression = "inherited: ratio-weighted average of direct upstream PRODUCT RMs' snapshot value (VB loss is computed at POY only)"

// inheritedFormulaTrace builds the trace entry for a formula whose result
// param was injected by applyInheritedVBLoss and therefore not evaluated.
func inheritedFormulaTrace(f Formula, scope map[string]any) FormulaEvalTrace {
	v, _ := scopeFloat(scope, f.ResultParamCode)
	return FormulaEvalTrace{
		FormulaCode:     f.FormulaCode,
		Expression:      inheritedVBLossExpression,
		ResultParamCode: f.ResultParamCode,
		Output:          v,
	}
}
