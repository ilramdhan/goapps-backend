package workorder

import (
	"context"

	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// routeRmIndex maps crm_rm_id to its route component, so a stored allocation
// (which persists only the id) can be decorated with the human-readable labels
// finance resolved for that route edge.
type routeRmIndex map[int64]workorderdomain.RouteRmComponent

// routeRmComponents fetches the RM components of the WO's product route.
//
// Every failure mode degrades to an empty index rather than an error: prefill
// and labeling are conveniences layered over stored data, and a finance outage
// must not make a WO unreadable. Callers distinguish "no route" from "route with
// no RMs" only by the component count, which is all either needs.
func (s *Service) routeRmComponents(ctx context.Context, planItemID int64) routeRmIndex {
	if s.routeRms == nil || s.planItems == nil {
		return nil
	}
	productSysID, err := s.planItems.ProductSysID(ctx, planItemID)
	if err != nil {
		return nil
	}
	comps, err := s.routeRms.RouteRmComponents(ctx, productSysID)
	if err != nil {
		return nil
	}
	idx := make(routeRmIndex, len(comps))
	for _, c := range comps {
		idx[c.CrmRmID] = c
	}
	return idx
}

// decorateRmAllocations stamps presentation labels onto stored allocations from
// the route index. Lines whose route edge is gone (route revised since the WO
// was saved) keep their stored values and are simply left unlabeled — the
// frontend renders those as "not in the current route" rather than as an id.
func decorateRmAllocations(allocs []*workorderdomain.RmAllocation, idx routeRmIndex) {
	if len(idx) == 0 {
		return
	}
	for _, a := range allocs {
		if a == nil {
			continue
		}
		c, ok := idx[a.CrmRmID]
		if !ok {
			continue
		}
		a.RmCode = c.RmCode
		a.RmName = c.RmName
		a.RouteStageName = c.RouteStageName
		a.RouteLevel = c.RouteLevel
		a.RouteRmRatio = c.Ratio
	}
}

// DecorateRmAllocations resolves and applies RM presentation labels for a WO's
// stored allocations. Best-effort: a degraded finance leaves the lines
// unlabeled rather than failing the read.
func (s *Service) DecorateRmAllocations(ctx context.Context, planItemID int64, allocs []*workorderdomain.RmAllocation) {
	if len(allocs) == 0 {
		return
	}
	decorateRmAllocations(allocs, s.routeRmComponents(ctx, planItemID))
}
