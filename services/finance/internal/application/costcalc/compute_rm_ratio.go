package costcalc

import (
	"fmt"
	"strings"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// Keys read by the DENIER ratio mode.
const (
	// snapKeyDenier is the upstream product's committed DENIER param (TOP 13).
	snapKeyDenier = "DENIER"
	// cappKeySpandexDraft is the product's own SPANDEX_DRAFT param (000570).
	cappKeySpandexDraft = "SPANDEX_DRAFT"
)

// Ratio sources recorded on RMCostDetail.RatioSource for DENIER-mode products.
const (
	ratioSourceDenier = "DENIER"
	ratioSourceInput  = "INPUT"
)

// denierLine is the DENIER-mode resolution of one route RM line.
type denierLine struct {
	denier float64
	ratio  float64
	// group marks a GROUP denier line, whose unit cost is the calc-type
	// selected cost_val/cost_mark (RMCostRates.CostVal) for both RM_RATE and
	// RM_LANDED_COST instead of the GROUP cascades.
	group bool
}

// rmRatioPlan holds the DENIER-mode ratio overrides of one product, keyed by
// the route RM line. A nil plan means INPUT mode: every line keeps its route
// ratio and its type's usual unit-cost resolver.
type rmRatioPlan struct {
	lines map[*costroute.Rm]denierLine
	notes []string
}

// isDenierMode reports whether mode selects the denier-share ratio.
func isDenierMode(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), RMRatioModeDenier)
}

// planRMRatios builds the DENIER-mode ratio plan for in. It returns nil for
// INPUT-mode products ("" / "INPUT" / anything other than DENIER), which keeps
// aggregateRMCost on its legacy path byte for byte.
//
// For every seq producing the target product:
//   - PRODUCT line: d = UpstreamSnapshots[upstream]["DENIER"]
//   - GROUP line whose head has a denier: d = head denier / CAPP SPANDEX_DRAFT
//   - any other line keeps RouteRmRatio and is not in the plan
//
// ratio_i = d_i / Σd over the seq's denier lines. A missing or non-positive
// input sets that line's d to 0 and records a note; Σd = 0 sets every denier
// line's ratio to 0 and records a note. It never fails the product.
func planRMRatios(in ComputeInput) *rmRatioPlan {
	if !isDenierMode(in.RMRatioMode) || in.Route == nil {
		return nil
	}
	plan := &rmRatioPlan{lines: map[*costroute.Rm]denierLine{}}
	seen := map[string]bool{}
	note := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		if !seen[msg] {
			seen[msg] = true
			plan.notes = append(plan.notes, msg)
		}
	}
	for _, seq := range in.Route.Seqs {
		if seq == nil || seq.ProductSysID != in.ProductSysID {
			continue
		}
		planSeqRatios(in, seq, plan, note)
	}
	return plan
}

// planSeqRatios fills plan with the denier lines of one seq.
func planSeqRatios(in ComputeInput, seq *costroute.Seq, plan *rmRatioPlan, note func(string, ...any)) {
	var total float64
	lines := make([]*costroute.Rm, 0, len(seq.Rms))
	for _, rm := range seq.Rms {
		if rm == nil {
			continue
		}
		d, group, ok := lineDenier(in, rm, note)
		if !ok {
			continue
		}
		plan.lines[rm] = denierLine{denier: d, group: group}
		lines = append(lines, rm)
		total += d
	}
	if len(lines) == 0 {
		return
	}
	if total <= 0 {
		note("DENIER ratio: total denier is 0 at route level %d; denier-based RM lines treated as ratio 0", seq.RouteLevel)
		return
	}
	for _, rm := range lines {
		l := plan.lines[rm]
		l.ratio = l.denier / total
		plan.lines[rm] = l
	}
}

// lineDenier resolves one line's denier. ok=false means the line is not a
// denier line and keeps its route ratio.
func lineDenier(in ComputeInput, rm *costroute.Rm, note func(string, ...any)) (d float64, group, ok bool) {
	switch rm.RmType {
	case costroute.RmTypeProduct:
		v := in.UpstreamSnapshots[rm.RmProductSysID][snapKeyDenier]
		if v <= 0 {
			note("DENIER ratio: upstream product %d has no DENIER; treated as 0", rm.RmProductSysID)
			return 0, false, true
		}
		return v, false, true
	case costroute.RmTypeGroup:
		headDenier, has := in.RMGroupDenier[rm.RmGroupCode]
		if !has {
			return 0, false, false
		}
		if headDenier <= 0 {
			note("DENIER ratio: RM group %s has no denier; treated as 0", rm.RmGroupCode)
			return 0, true, true
		}
		draft := in.CAPP[cappKeySpandexDraft]
		if draft <= 0 {
			note("DENIER ratio: SPANDEX_DRAFT missing or not positive; RM group %s treated as 0", rm.RmGroupCode)
			return 0, true, true
		}
		return headDenier / draft, true, true
	default:
		return 0, false, false
	}
}

// resolveDenierGroupCost is the unit cost of a GROUP denier line: the
// calc-type-selected cost_val/cost_mark (honoring the head's valuation /
// marketing flag), used for both RM_RATE and RM_LANDED_COST. A missing
// cst_rm_cost row keeps the ErrMissingRMCost block.
func resolveDenierGroupCost(in ComputeInput, rm *costroute.Rm) (float64, error) {
	rates, ok := in.RMCosts[rm.RmGroupCode+"|"]
	if !ok {
		return 0, fmt.Errorf("%w: group %s", costcalcdom.ErrMissingRMCost, rm.RmGroupCode)
	}
	return rates.CostVal, nil
}

// lineUnitAndRatio resolves one RM line's unit cost, ratio and detail
// annotations under plan (nil plan = INPUT mode, the legacy behavior).
func (p *rmRatioPlan) lineUnitAndRatio(in ComputeInput, rm *costroute.Rm, resolve rmUnitResolver) (unit, ratio, denier float64, source string, err error) {
	if p == nil {
		unit, err = resolve(in, rm)
		return unit, rm.RouteRmRatio, 0, "", err
	}
	l, ok := p.lines[rm]
	if !ok {
		unit, err = resolve(in, rm)
		return unit, rm.RouteRmRatio, 0, ratioSourceInput, err
	}
	if l.group {
		unit, err = resolveDenierGroupCost(in, rm)
	} else {
		unit, err = resolve(in, rm)
	}
	return unit, l.ratio, l.denier, ratioSourceDenier, err
}

// notesOrNil returns the plan's deduplicated notes, or nil for an INPUT-mode
// (nil) plan or a plan without notes.
func (p *rmRatioPlan) notesOrNil() []string {
	if p == nil || len(p.notes) == 0 {
		return nil
	}
	return p.notes
}
