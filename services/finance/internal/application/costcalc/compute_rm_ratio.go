package costcalc

import (
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// Keys and sources of the DENIER ratio mode, re-exported from the shared
// costroute domain computation so calc and route graph cannot diverge.
const (
	// cappKeySpandexDraft is the product's own SPANDEX_DRAFT param (000570).
	cappKeySpandexDraft = costroute.ParamCodeSpandexDraft
	ratioSourceDenier   = costroute.RatioSourceDenier
	ratioSourceInput    = costroute.RatioSourceInput
	// denierSourceMaster / denierSourceOverride are RMCostDetail.DenierSource
	// values of a denier line (master denier vs. RM denier override, 000578).
	denierSourceMaster   = "MASTER"
	denierSourceOverride = "OVERRIDE"
)

// denierLine is the DENIER-mode resolution of one route RM line.
type denierLine struct {
	denier float64
	ratio  float64
	// group marks a GROUP denier line, whose unit cost is the calc-type
	// selected cost_val/cost_mark (RMCostRates.CostVal) for both RM_RATE and
	// RM_LANDED_COST instead of the GROUP cascades.
	group bool
	// overridden marks a line whose denier is the product's RM denier
	// override (000578) instead of the master denier.
	overridden bool
}

// rmRatioPlan holds the DENIER-mode ratio overrides of one product, keyed by
// the route RM line. A nil plan means INPUT mode: every line keeps its route
// ratio and its type's usual unit-cost resolver.
type rmRatioPlan struct {
	lines map[*costroute.Rm]denierLine
	notes []string
}

// planRMRatios builds the DENIER-mode ratio plan for in. It returns nil for
// INPUT-mode products ("" / "INPUT" / anything other than DENIER), which keeps
// aggregateRMCost on its legacy path byte for byte.
//
// Every seq producing the target product is resolved by
// costroute.ComputeSeqRMRatios (the same function the route graph viewer
// uses); only its DENIER-source lines enter the plan, every other line keeps
// RouteRmRatio. Notes are deduplicated across seqs in first-seen order. It
// never fails the product.
func planRMRatios(in ComputeInput) *rmRatioPlan {
	if !costroute.IsDenierMode(in.RMRatioMode) || in.Route == nil {
		return nil
	}
	plan := &rmRatioPlan{lines: map[*costroute.Rm]denierLine{}}
	src := ratioSourceData(in)
	seen := map[string]bool{}
	for _, seq := range in.Route.Seqs {
		if seq == nil || seq.ProductSysID != in.ProductSysID {
			continue
		}
		lines, notes := costroute.ComputeSeqRMRatios(seq, src)
		for i, l := range lines {
			if l.Source != ratioSourceDenier {
				continue
			}
			rm := seq.Rms[i]
			plan.lines[rm] = denierLine{denier: l.Denier, ratio: l.Ratio, group: rm.RmType == costroute.RmTypeGroup, overridden: l.Overridden}
		}
		for _, n := range notes {
			if !seen[n] {
				seen[n] = true
				plan.notes = append(plan.notes, n)
			}
		}
	}
	return plan
}

// ratioSourceData adapts a ComputeInput to the shared domain computation.
func ratioSourceData(in ComputeInput) costroute.RatioSourceData {
	return costroute.RatioSourceData{
		Mode:                  in.RMRatioMode,
		UpstreamDenier:        in.UpstreamDenier,
		GroupDenier:           in.RMGroupDenier,
		GroupUseDraft:         in.RMGroupUseDraft,
		SpandexDraft:          in.CAPP[cappKeySpandexDraft],
		ProductDenierOverride: in.RMDenierOverride.Product,
		GroupDenierOverride:   in.RMDenierOverride.Group,
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
// denierSource is "" for non-denier lines, else MASTER or OVERRIDE.
func (p *rmRatioPlan) lineUnitAndRatio(in ComputeInput, rm *costroute.Rm, resolve rmUnitResolver) (unit, ratio, denier float64, source, denierSource string, err error) {
	if p == nil {
		unit, err = resolve(in, rm)
		return unit, rm.RouteRmRatio, 0, "", "", err
	}
	l, ok := p.lines[rm]
	if !ok {
		unit, err = resolve(in, rm)
		return unit, rm.RouteRmRatio, 0, ratioSourceInput, "", err
	}
	if l.group {
		unit, err = resolveDenierGroupCost(in, rm)
	} else {
		unit, err = resolve(in, rm)
	}
	denierSource = denierSourceMaster
	if l.overridden {
		denierSource = denierSourceOverride
	}
	return unit, l.ratio, l.denier, ratioSourceDenier, denierSource, err
}

// notesOrNil returns the plan's deduplicated notes, or nil for an INPUT-mode
// (nil) plan or a plan without notes.
func (p *rmRatioPlan) notesOrNil() []string {
	if p == nil || len(p.notes) == 0 {
		return nil
	}
	return p.notes
}
