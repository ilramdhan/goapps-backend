package grpc

import (
	"context"
	"slices"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	planitemapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/planitem"
	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

// Plan-item carry-forward RPCs. Mirrors the demand pair in demand_handler.go.

// ListPlanCarryForwardCandidates lists plan items eligible for carry-forward.
func (h *planItemHandler) ListPlanCarryForwardCandidates(
	ctx context.Context, req *ppcv1.ListPlanCarryForwardCandidatesRequest,
) (*ppcv1.ListPlanCarryForwardCandidatesResponse, error) {
	candidates, err := h.svc.ListCarryCandidates(ctx, req.GetSourceMonth(), req.GetTargetMonth())
	if err != nil {
		return &ppcv1.ListPlanCarryForwardCandidatesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*planitemdomain.PlanItem, len(candidates))
	for i, c := range candidates {
		items[i] = c.Item
	}
	products := h.svc.LookupProducts(ctx, planItemSysIDs(items))

	data := make([]*ppcv1.PlanCarryCandidate, len(candidates))
	for i, c := range candidates {
		data[i] = &ppcv1.PlanCarryCandidate{
			Item:           planItemToProto(c.Item, products),
			QtyUncovered:   formatDecimal(c.QtyUncovered()),
			QtyCovered:     formatDecimal(c.Coverage.QtyCovered),
			WorkOrderCount: c.Coverage.WorkOrderCount,
			// The proto field is per-target: true when THIS request's target month
			// already holds a child of this row. The domain tracks every month it
			// was carried into, so narrow it back down here.
			AlreadyCarried: slices.Contains(c.Coverage.CarriedToMonths, req.GetTargetMonth()),
			DemandLabel:    c.DemandLabel,
		}
	}
	return &ppcv1.ListPlanCarryForwardCandidatesResponse{
		Base: successResponse("Plan carry-forward candidates retrieved"),
		Data: data,
	}, nil
}

// ProcessPlanCarryForward executes a carry-forward action for one plan item.
func (h *planItemHandler) ProcessPlanCarryForward(
	ctx context.Context, req *ppcv1.ProcessPlanCarryForwardRequest,
) (*ppcv1.ProcessPlanCarryForwardResponse, error) {
	cmd, base := buildPlanCarryForwardCommand(ctx, req)
	if base != nil {
		return &ppcv1.ProcessPlanCarryForwardResponse{Base: base}, nil
	}
	created, err := h.svc.ProcessPlanCarryForward(ctx, cmd)
	if err != nil {
		return &ppcv1.ProcessPlanCarryForwardResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	// CANCEL creates nothing, so Data stays nil rather than carrying a
	// zero-valued plan item the client would render as a real row.
	if created == nil {
		return &ppcv1.ProcessPlanCarryForwardResponse{
			Base: successResponse("Plan item closed without being carried forward"),
		}, nil
	}
	products := h.svc.LookupProducts(ctx, []int64{created.CpmProductSysID()})
	return &ppcv1.ProcessPlanCarryForwardResponse{
		Base: successResponse("Plan carry-forward processed successfully"),
		Data: planItemToProto(created, products),
	}, nil
}

// buildPlanCarryForwardCommand decodes and validates a plan carry-forward
// request, matching buildCarryForwardCommand's shape on the demand side.
func buildPlanCarryForwardCommand(
	ctx context.Context, req *ppcv1.ProcessPlanCarryForwardRequest,
) (planitemapp.ProcessPlanCarryForwardCommand, *commonv1.BaseResponse) {
	cmd := planitemapp.ProcessPlanCarryForwardCommand{
		SourcePlanItemID: req.GetSourcePlanItemId(),
		Action:           planCarryActionToString(req.GetAction()),
		TargetMonth:      req.GetTargetMonth(),
		ActedBy:          actorID(ctx),
	}
	if req.NewDeadline != nil {
		v, base := dateField("new_deadline", req.GetNewDeadline())
		if base != nil {
			return cmd, base
		}
		cmd.NewDeadline = &v
	}
	if req.CarryQty != nil {
		v, base := decimalField("carry_qty", req.GetCarryQty())
		if base != nil {
			return cmd, base
		}
		cmd.CarryQty = &v
	}
	return cmd, nil
}
