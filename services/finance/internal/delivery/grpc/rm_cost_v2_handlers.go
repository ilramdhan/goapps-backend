// Package grpc — V2 RM Cost RPC handlers (ListCostDetails, UpdateRMCostInputs,
// UpdateCostDetailFixRate). Lives in a separate file to keep rm_cost_handler.go
// focused on the V1 surface.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	apprmcost "github.com/mutugading/goapps-backend/services/finance/internal/application/rmcost"
	rmcostdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
)

// ListCostDetails returns the per-item snapshot rows for one cost row.
func (h *RMCostHandler) ListCostDetails(ctx context.Context, req *financev1.ListCostDetailsRequest) (*financev1.ListCostDetailsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostDetailsResponse{Base: baseResp}, nil
	}
	id, err := uuid.Parse(req.RmCostId)
	if err != nil {
		return &financev1.ListCostDetailsResponse{Base: &commonv1.BaseResponse{
			IsSuccess:  false,
			StatusCode: "400",
			Message:    "invalid rm_cost_id",
		}}, nil
	}
	details, err := h.costDetailRepo.ListByCostID(ctx, id)
	if err != nil {
		return &financev1.ListCostDetailsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	out := make([]*financev1.RMCostDetail, len(details))
	for i, d := range details {
		out[i] = costDetailToProto(d)
	}
	return &financev1.ListCostDetailsResponse{
		Base: successResponse("Cost details retrieved"),
		Data: out,
	}, nil
}

// UpdateRMCostInputs edits per-row marketing inputs / simulation_rate / flags.
func (h *RMCostHandler) UpdateRMCostInputs(ctx context.Context, req *financev1.UpdateRMCostInputsRequest) (*financev1.UpdateRMCostInputsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateRMCostInputsResponse{Base: baseResp}, nil
	}
	id, err := uuid.Parse(req.RmCostId)
	if err != nil {
		return &financev1.UpdateRMCostInputsResponse{Base: &commonv1.BaseResponse{
			IsSuccess:  false,
			StatusCode: "400",
			Message:    "invalid rm_cost_id",
		}}, nil
	}
	cmd := apprmcost.EditInputsCommand{
		RMCostID:                     id,
		MarketingFreightRate:         req.MarketingFreightRate,
		MarketingAntiDumpingPct:      req.MarketingAntiDumpingPct,
		MarketingDutyPct:             req.MarketingDutyPct,
		MarketingTransportRate:       req.MarketingTransportRate,
		MarketingDefaultValue:        req.MarketingDefaultValue,
		SimulationRate:               req.SimulationRate,
		ClearMarketingFreightRate:    req.ClearMarketingFreightRate,
		ClearMarketingAntiDumpingPct: req.ClearMarketingAntiDumpingPct,
		ClearMarketingDutyPct:        req.ClearMarketingDutyPct,
		ClearMarketingTransportRate:  req.ClearMarketingTransportRate,
		ClearMarketingDefaultValue:   req.ClearMarketingDefaultValue,
		ClearSimulationRate:          req.ClearSimulationRate,
		UpdatedBy:                    getUserFromContext(ctx),
	}
	if req.ValuationFlag != nil {
		s := protoValuationFlagToString(*req.ValuationFlag)
		if s == "" {
			s = "AUTO"
		}
		cmd.ValuationFlag = &s
	}
	if req.MarketingFlag != nil {
		s := protoMarketingFlagToString(*req.MarketingFlag)
		if s == "" {
			s = "AUTO"
		}
		cmd.MarketingFlag = &s
	}
	cost, err := h.editInputsHandler.Handle(ctx, cmd)
	if err != nil {
		return &financev1.UpdateRMCostInputsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &financev1.UpdateRMCostInputsResponse{
		Base: successResponse("RM Cost inputs updated"),
		Data: rmCostToProto(cost),
	}, nil
}

// UpdateCostDetailFixRate edits one detail row's fix_rate and recomputes FL chain.
func (h *RMCostHandler) UpdateCostDetailFixRate(ctx context.Context, req *financev1.UpdateCostDetailFixRateRequest) (*financev1.UpdateCostDetailFixRateResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateCostDetailFixRateResponse{Base: baseResp}, nil
	}
	id, err := uuid.Parse(req.CostDetailId)
	if err != nil {
		return &financev1.UpdateCostDetailFixRateResponse{Base: &commonv1.BaseResponse{
			IsSuccess:  false,
			StatusCode: "400",
			Message:    "invalid cost_detail_id",
		}}, nil
	}
	res, err := h.editFixRateHandler.Handle(ctx, apprmcost.EditFixRateCommand{
		CostDetailID: id,
		FixRate:      req.FixRate,
		UpdatedBy:    getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.UpdateCostDetailFixRateResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &financev1.UpdateCostDetailFixRateResponse{
		Base:       successResponse("Fix rate updated"),
		Detail:     costDetailToProto(res.Detail),
		ParentCost: rmCostToProto(res.Cost),
	}, nil
}

// costDetailToProto maps a domain CostDetail to its proto form.
//
//nolint:gocyclo,gocognit // wide field mapping is shallow, no branching beyond field assigns
func costDetailToProto(d *rmcostdomain.CostDetail) *financev1.RMCostDetail {
	snap := d.Snapshot()
	out := &financev1.RMCostDetail{
		CostDetailId: d.ID().String(),
		CostId:       d.CostID().String(),
		Period:       d.Period(),
		GroupHeadId:  d.GroupHeadID().String(),
		ItemCode:     d.ItemCode(),
		ItemName:     d.ItemName(),
		GradeCode:    d.GradeCode(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: d.CreatedAt().Format(time.RFC3339),
			CreatedBy: d.CreatedBy(),
		},
	}
	if id := d.GroupDetailID(); id != nil {
		s := id.String()
		out.GroupDetailId = &s
	}
	if t := d.UpdatedAt(); t != nil {
		out.Audit.UpdatedAt = t.Format(time.RFC3339)
	}
	if by := d.UpdatedBy(); by != nil {
		out.Audit.UpdatedBy = *by
	}
	out.FreightRate = snap.FreightRate
	out.AntiDumpingPct = snap.AntiDumpingPct
	out.DutyPct = snap.DutyPct
	out.TransportRate = snap.TransportRate
	out.ValuationDefaultValue = snap.ValuationDefaultValue
	out.ConsVal = snap.ConsVal
	out.ConsQty = snap.ConsQty
	out.ConsRate = snap.ConsRate
	out.ConsFreightVal = snap.ConsFreightVal
	out.ConsValBased = snap.ConsValBased
	out.ConsRateBased = snap.ConsRateBased
	out.ConsAntiDumpingVal = snap.ConsAntiDumpingVal
	out.ConsAntiDumpingRate = snap.ConsAntiDumpingRate
	out.ConsDutyVal = snap.ConsDutyVal
	out.ConsDutyRate = snap.ConsDutyRate
	out.ConsTransportVal = snap.ConsTransportVal
	out.ConsTransportRate = snap.ConsTransportRate
	out.ConsLandedCost = snap.ConsLandedCost
	out.StockVal = snap.StockVal
	out.StockQty = snap.StockQty
	out.StockRate = snap.StockRate
	out.StockFreightVal = snap.StockFreightVal
	out.StockValBased = snap.StockValBased
	out.StockRateBased = snap.StockRateBased
	out.StockAntiDumpingVal = snap.StockAntiDumpingVal
	out.StockAntiDumpingRate = snap.StockAntiDumpingRate
	out.StockDutyVal = snap.StockDutyVal
	out.StockDutyRate = snap.StockDutyRate
	out.StockTransportVal = snap.StockTransportVal
	out.StockTransportRate = snap.StockTransportRate
	out.StockLandedCost = snap.StockLandedCost
	out.PoVal = snap.POVal
	out.PoQty = snap.POQty
	out.PoRate = snap.PORate
	out.FixRate = snap.FixRate
	out.FixFreightRate = snap.FixFreightRate
	out.FixRateBased = snap.FixRateBased
	out.FixAntiDumpingRate = snap.FixAntiDumpingRate
	out.FixDutyRate = snap.FixDutyRate
	out.FixTransportRate = snap.FixTransportRate
	out.FixLandedCost = snap.FixLandedCost
	return out
}
