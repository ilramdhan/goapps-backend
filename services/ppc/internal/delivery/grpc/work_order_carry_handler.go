package grpc

import (
	"context"
	"errors"
	"slices"
	"strconv"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// ── WO carry-forward RPCs. Mirrors the plan-item pair. ─────────────────────

// WO carry-forward lives on the existing workOrderHandler alongside the other
// WO RPCs already defined in work_order_handler.go.

// ListWorkOrderCarryForwardCandidates lists WOs in a source month, each
// decorated with its carry eligibility.
func (h *workOrderHandler) ListWorkOrderCarryForwardCandidates(
	ctx context.Context, req *ppcv1.ListWorkOrderCarryForwardCandidatesRequest,
) (*ppcv1.ListWorkOrderCarryForwardCandidatesResponse, error) {
	candidates, err := h.svc.ListWorkOrderCarryCandidates(ctx, req.GetSourceMonth(), req.GetTargetMonth())
	if err != nil {
		return &ppcv1.ListWorkOrderCarryForwardCandidatesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	// The repository carries only the finance product sys id — the product
	// master lives in finance, not this database. Resolve the whole page in one
	// batch so a planner sees product names instead of blank cells.
	products := h.planItems.LookupProducts(ctx, carryCandidateSysIDs(candidates))

	data := make([]*ppcv1.WorkOrderCarryCandidate, len(candidates))
	for i, c := range candidates {
		data[i] = &ppcv1.WorkOrderCarryCandidate{
			Wo:                  workOrderToProto(c.WO, nil),
			RemainingQty:        formatDecimal(c.QtyRemaining()),
			MachineLabel:        c.MachineLabel,
			ProductLabel:        productLabel(products[c.ProductSysID]),
			IneligibilityReason: c.IneligibilityReason,
			AlreadyCarried:      slices.Contains(c.Coverage.CarriedToMonths, req.GetTargetMonth()),
		}
	}
	return &ppcv1.ListWorkOrderCarryForwardCandidatesResponse{
		Base: successResponse("WO carry-forward candidates retrieved"),
		Data: data,
	}, nil
}

// carryCandidateSysIDs collects the product sys ids referenced by a page of
// carry candidates, for a single batched product lookup.
func carryCandidateSysIDs(candidates []*workorderdomain.CarryCandidate) []int64 {
	ids := make([]int64, len(candidates))
	for i, c := range candidates {
		ids[i] = c.ProductSysID
	}
	return ids
}

// productLabel renders a finance product for a planner: its code, falling back
// to its name. A missing product yields "" rather than an id — the candidate
// row simply shows no product, which the UI renders as a dash.
func productLabel(p *financev1.CostMasterProduct) string {
	if p == nil {
		return ""
	}
	if code := p.GetProductCode(); code != "" {
		return code
	}
	return p.GetProductName()
}

// ProcessWorkOrderCarryForward carries one WO into a new month as a
// CONTINUATION.
func (h *workOrderHandler) ProcessWorkOrderCarryForward(
	ctx context.Context, req *ppcv1.ProcessWorkOrderCarryForwardRequest,
) (*ppcv1.ProcessWorkOrderCarryForwardResponse, error) {
	cmd, base := buildWOCarryForwardCommand(ctx, req)
	if base != nil {
		return &ppcv1.ProcessWorkOrderCarryForwardResponse{Base: base}, nil
	}
	created, err := h.svc.ProcessWorkOrderCarryForward(ctx, cmd)
	if err != nil {
		// The reason is the whole value of this error — "is still a draft —
		// confirm it first" tells the planner what to do next, where the generic
		// sentinel only says no. The WO id in the error string is never sent.
		var ineligibleErr workorderdomain.CarryIneligibleError
		if errors.As(err, &ineligibleErr) {
			return &ppcv1.ProcessWorkOrderCarryForwardResponse{
				Base: &commonv1.BaseResponse{
					IsSuccess:  false,
					StatusCode: "400",
					Message:    "This work order " + ineligibleErr.Reason,
				},
			}, nil
		}
		return &ppcv1.ProcessWorkOrderCarryForwardResponse{Base: carryErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.ProcessWorkOrderCarryForwardResponse{
		Base: successResponse("WO carry-forward processed successfully"),
		Data: workOrderToProto(created, nil),
	}, nil
}

// carryRefusals are the carry-forward rejections that are the planner's to
// resolve, not the server's fault.
//
// domainErrorToBaseResponse classifies by substring, and none of these contain
// "invalid", "not found" or "must be" — so they all fell through to its 500
// default. A 500 tells the planner the system is broken and to retry, when in
// fact the answer will not change until they pick a different month or quantity;
// it also puts a routine refusal into the error-rate alerting. Matching on the
// sentinel rather than on prose keeps the classification from drifting when the
// message is reworded.
var carryRefusals = []error{
	workorderdomain.ErrNothingToCarry,
	workorderdomain.ErrAlreadyCarriedIntoMonth,
	workorderdomain.ErrCarryQtyExceedsRemaining,
	workorderdomain.ErrWONotEligibleForCarry,
	workorderdomain.ErrCarryTargetNotLater,
	workorderdomain.ErrInvalidTargetMonth,
}

// carryErrorToBaseResponse renders a carry-forward failure, reporting the
// planner-resolvable refusals as 400 and deferring everything else to the shared
// mapper.
func carryErrorToBaseResponse(err error) *commonv1.BaseResponse {
	for _, refusal := range carryRefusals {
		if errors.Is(err, refusal) {
			return errorResponse("400", err.Error())
		}
	}
	return domainErrorToBaseResponse(err)
}

// buildWOCarryForwardCommand decodes and validates a WO carry-forward request.
func buildWOCarryForwardCommand(
	ctx context.Context, req *ppcv1.ProcessWorkOrderCarryForwardRequest,
) (workorderapp.ProcessWorkOrderCarryForwardCommand, *commonv1.BaseResponse) {
	cmd := workorderapp.ProcessWorkOrderCarryForwardCommand{
		SourceWOID:  req.GetSourceWoId(),
		TargetMonth: req.GetTargetMonth(),
		LotNo:       req.GetLotNo(),
		// The continuation is created by whoever clicked carry-forward, not by
		// the source WO's author — without this the new WO's created_by is 0.
		ActedBy: actorID(ctx),
	}
	if req.CarryQty != "" {
		qty, err := strconv.ParseFloat(req.GetCarryQty(), 64)
		if err != nil {
			return cmd, &commonv1.BaseResponse{
				IsSuccess:  false,
				StatusCode: "400",
				Message:    "carry_qty must be a valid decimal number",
			}
		}
		cmd.CarryQty = &qty
	}
	return cmd, nil
}
