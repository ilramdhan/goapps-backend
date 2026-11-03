package costcalc

import (
	"slices"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// Route total conversion params (migration 000605, Excel conv-cost-calc.xlsx
// columns EF..EI).
const (
	// paramUpstreamTotalCapConv is engine-injected (no formula): the route
	// total captive conversion carried in from this product's upstream
	// PRODUCT RMs. Consumed by F_YARN_TOTAL_CAP_CONV / F_YARN_TOTAL_DEL_CONV.
	paramUpstreamTotalCapConv = "UPSTREAM_TOTAL_CAP_CONV"
	// paramTotalCapConv is the formula-produced route total captive
	// conversion (Excel EH) read from each upstream's committed snapshot.
	paramTotalCapConv = "TOTAL_CAP_CONV"
)

// applyUpstreamTotalCapConv injects UPSTREAM_TOTAL_CAP_CONV =
// Σ ratio × upstream snapshot TOTAL_CAP_CONV over this product's direct
// upstream PRODUCT RMs, skipping MB-typed upstreams.
//
// It is a weighted SUM (not the ratio-weighted average VB loss inheritance
// uses): RM_RATE = Σ ratio × upstream cost, so the upstream conversion share
// embedded in RM_RATE scales by the same ratio. ratio is the effective ratio
// aggregateRMCost uses (the DENIER-mode plan ratio when the plan overrides the
// line, else RouteRmRatio), so TOTAL_CAP_CONV reconciles with CAP_FINAL.
//
// Products whose formula chain does not consume the param are left untouched
// (no scope write, no new snapshot key). Otherwise the value is always written
// (0 when there is no eligible upstream) and cleared from zeroFilled so it is
// persisted in cpc_param_snapshot. A missing upstream snapshot or key counts
// as 0 and is logged.
func applyUpstreamTotalCapConv(in ComputeInput, plan *rmRatioPlan, scope map[string]any, zeroFilled map[string]bool) {
	if !formulasConsume(in.Formulas, paramUpstreamTotalCapConv) {
		return
	}
	snapshots := in.UpstreamSnapshots
	var typeCodes map[int64]string
	if in.VBLoss != nil {
		typeCodes = in.VBLoss.UpstreamTypeCodes
		if snapshots == nil {
			snapshots = in.VBLoss.UpstreamParamSnapshots
		}
	}

	var total float64
	for _, rm := range upstreamProductRMs(in) {
		if strings.EqualFold(strings.TrimSpace(typeCodes[rm.RmProductSysID]), productTypeCodeMB) {
			continue
		}
		snap, hasSnap := snapshots[rm.RmProductSysID]
		v, ok := snap[paramTotalCapConv]
		if !ok {
			warnMissingUpstreamTotalCapConv(in, rm.RmProductSysID, hasSnap)
			continue
		}
		total += v * plan.effectiveRatio(rm)
	}
	scope[paramUpstreamTotalCapConv] = total
	delete(zeroFilled, paramUpstreamTotalCapConv)
}

// formulasConsume reports whether any formula lists code as an input.
func formulasConsume(formulas []Formula, code string) bool {
	for _, f := range formulas {
		if slices.Contains(f.InputParamCodes, code) {
			return true
		}
	}
	return false
}

// effectiveRatio is the ratio aggregateRMCost applies to rm under plan (see
// lineUnitAndRatio): the DENIER-derived ratio for a planned line, else the
// route ratio. A nil plan (INPUT mode) always yields the route ratio.
func (p *rmRatioPlan) effectiveRatio(rm *costroute.Rm) float64 {
	if p != nil {
		if l, ok := p.lines[rm]; ok {
			return l.ratio
		}
	}
	return rm.RouteRmRatio
}

// warnMissingUpstreamTotalCapConv logs an upstream TOTAL_CAP_CONV taken as 0.
func warnMissingUpstreamTotalCapConv(in ComputeInput, upstreamID int64, hasSnap bool) {
	reason := "upstream snapshot has no " + paramTotalCapConv
	if !hasSnap {
		reason = "upstream has no committed cost row for this period/calc type"
	}
	log.Warn().
		Int64("product_sys_id", in.ProductSysID).
		Int64("upstream_product_sys_id", upstreamID).
		Str("period", in.Period).
		Str("calc_type", string(in.CalcType)).
		Str("reason", reason).
		Msg("route total conversion: upstream TOTAL_CAP_CONV missing, counted as 0")
}
