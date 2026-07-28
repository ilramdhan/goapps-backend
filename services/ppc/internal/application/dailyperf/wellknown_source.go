package dailyperf

import (
	"context"

	dpdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// WOResolveContext resolves the parameter-resolution context for a WO: its
// finance product sys id, optional reference WO, and machine. Implemented in the
// postgres layer. It keeps domain/dailyperf free of any workorder import.
type WOResolveContext interface {
	ResolveContext(ctx context.Context, woID int64) (productSysID int64, refWoID *int64, machineID int64, err error)
}

// WellKnownSource adapts the workorder parameter Resolver to the daily-perf
// WellKnownParamSource port. It resolves all display groups, filters to the
// well-known efficiency codes, and reads their numeric values. When the resolver
// or context is unavailable it degrades to zero-value params (efficiency 0).
type WellKnownSource struct {
	resolver *workorderdomain.Resolver
	woCtx    WOResolveContext
}

// NewWellKnownSource builds a well-known parameter source. Either dependency may
// be nil, in which case resolution degrades to zero-value params.
func NewWellKnownSource(resolver *workorderdomain.Resolver, woCtx WOResolveContext) *WellKnownSource {
	return &WellKnownSource{resolver: resolver, woCtx: woCtx}
}

// WellKnown resolves the pinned efficiency parameters for a WO on a machine.
func (s *WellKnownSource) WellKnown(ctx context.Context, machineID, woID int64) (dpdomain.WellKnownParams, error) {
	if s.resolver == nil || s.woCtx == nil || woID == 0 {
		return dpdomain.WellKnownParams{}, nil
	}

	productSysID, refWoID, ctxMachineID, err := s.woCtx.ResolveContext(ctx, woID)
	if err != nil {
		return dpdomain.WellKnownParams{}, nil //nolint:nilerr // degrade to zero-value params
	}
	if machineID == 0 {
		machineID = ctxMachineID
	}

	resolved, err := s.resolver.Resolve(ctx, workorderdomain.ResolveRequest{
		ProductSysID: productSysID,
		MachineID:    machineID,
		RefWoID:      refWoID,
		DisplayGroup: "", // all groups
	})
	if err != nil {
		return dpdomain.WellKnownParams{}, nil //nolint:nilerr // degrade to zero-value params
	}

	return mapWellKnown(resolved), nil
}

// mapWellKnown extracts the numeric well-known values from resolved parameters.
func mapWellKnown(resolved []workorderdomain.ResolvedParam) dpdomain.WellKnownParams {
	var params dpdomain.WellKnownParams
	for _, rp := range resolved {
		if !workorderdomain.IsWellKnownCode(rp.ParamCode) || rp.Num == nil {
			continue
		}
		switch rp.ParamCode {
		case workorderdomain.WellKnownDenier:
			params.Denier = *rp.Num
		case workorderdomain.WellKnownYarnSpeed:
			params.Speed = *rp.Num
		case workorderdomain.WellKnownNoOfPosition:
			params.Positions = *rp.Num
		case workorderdomain.WellKnownStdWeight:
			params.StdWeight = *rp.Num
		}
	}
	return params
}
