// Package grpc provides gRPC server implementation for the finance service.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appboxbobbincost "github.com/mutugading/goapps-backend/services/finance/internal/application/boxbobbincost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// BoxBobbinCostHandler implements financev1.BoxBobbinCostServiceServer.
type BoxBobbinCostHandler struct {
	financev1.UnimplementedBoxBobbinCostServiceServer
	createHandler     *appboxbobbincost.CreateHandler
	getHandler        *appboxbobbincost.GetHandler
	listHandler       *appboxbobbincost.ListHandler
	updateHandler     *appboxbobbincost.UpdateHandler
	deleteHandler     *appboxbobbincost.DeleteHandler
	createRateHandler *appboxbobbincost.CreateRateHandler
	deleteRateHandler *appboxbobbincost.DeleteRateHandler
	validation        *ValidationHelper
}

// NewBoxBobbinCostHandler constructs a BoxBobbinCostHandler.
func NewBoxBobbinCostHandler(repo boxbobbincost.Repository) (*BoxBobbinCostHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &BoxBobbinCostHandler{
		createHandler:     appboxbobbincost.NewCreateHandler(repo),
		getHandler:        appboxbobbincost.NewGetHandler(repo),
		listHandler:       appboxbobbincost.NewListHandler(repo),
		updateHandler:     appboxbobbincost.NewUpdateHandler(repo),
		deleteHandler:     appboxbobbincost.NewDeleteHandler(repo),
		createRateHandler: appboxbobbincost.NewCreateRateHandler(repo),
		deleteRateHandler: appboxbobbincost.NewDeleteRateHandler(repo),
		validation:        v,
	}, nil
}

// CreateBoxBobbinCost creates a new box bobbin cost config.
func (h *BoxBobbinCostHandler) CreateBoxBobbinCost(ctx context.Context, req *financev1.CreateBoxBobbinCostRequest) (*financev1.CreateBoxBobbinCostResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordBoxBobbinCostOperation("create", false)
		return &financev1.CreateBoxBobbinCostResponse{Base: baseResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appboxbobbincost.CreateCommand{
		Code:         req.BbcCode,
		Name:         req.BbcName,
		BBCType:      req.BbcType,
		NoOfBob:      int(req.NoOfBob),
		Notes:        req.Notes,
		BbnReuse:     req.BbnReuse,
		BoxReuse:     req.BoxReuse,
		BoxCost:      req.BoxCost,
		BobinCost:    req.BobinCost,
		BoxCostVal:   req.BoxCostVal,
		BobinCostVal: req.BobinCostVal,
		BbnReuseVal:  req.BbnReuseVal,
		BoxReuseVal:  req.BoxReuseVal,
		CreatedBy:    getUserFromContext(ctx),
	})
	if err != nil {
		RecordBoxBobbinCostOperation("create", false)
		return &financev1.CreateBoxBobbinCostResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordBoxBobbinCostOperation("create", true)
	return &financev1.CreateBoxBobbinCostResponse{
		Base: successResponse("Box bobbin cost created successfully"),
		Data: boxBobbinCostEntityToProto(entity),
	}, nil
}

// GetBoxBobbinCost retrieves a box bobbin cost config by ID.
func (h *BoxBobbinCostHandler) GetBoxBobbinCost(ctx context.Context, req *financev1.GetBoxBobbinCostRequest) (*financev1.GetBoxBobbinCostResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordBoxBobbinCostOperation("get", false)
		return &financev1.GetBoxBobbinCostResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.BbcId)
	if err != nil {
		RecordBoxBobbinCostOperation("get", false)
		return &financev1.GetBoxBobbinCostResponse{Base: invalidIDResponse("bbc_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.getHandler.Handle(ctx, appboxbobbincost.GetQuery{ID: id})
	if err != nil {
		RecordBoxBobbinCostOperation("get", false)
		return &financev1.GetBoxBobbinCostResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordBoxBobbinCostOperation("get", true)
	return &financev1.GetBoxBobbinCostResponse{
		Base: successResponse("Box bobbin cost retrieved successfully"),
		Data: boxBobbinCostEntityToProto(entity),
	}, nil
}

// UpdateBoxBobbinCost updates an existing box bobbin cost config.
func (h *BoxBobbinCostHandler) UpdateBoxBobbinCost(ctx context.Context, req *financev1.UpdateBoxBobbinCostRequest) (*financev1.UpdateBoxBobbinCostResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordBoxBobbinCostOperation("update", false)
		return &financev1.UpdateBoxBobbinCostResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.BbcId)
	if err != nil {
		RecordBoxBobbinCostOperation("update", false)
		return &financev1.UpdateBoxBobbinCostResponse{Base: invalidIDResponse("bbc_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	var noOfBob *int
	if req.NoOfBob != nil {
		v := int(*req.NoOfBob)
		noOfBob = &v
	}

	entity, err := h.updateHandler.Handle(ctx, appboxbobbincost.UpdateCommand{
		ID:           id,
		Name:         req.BbcName,
		BBCType:      req.BbcType,
		NoOfBob:      noOfBob,
		Notes:        req.Notes,
		IsActive:     req.IsActive,
		BbnReuse:     req.BbnReuse,
		BoxReuse:     req.BoxReuse,
		BoxCost:      req.BoxCost,
		BobinCost:    req.BobinCost,
		BoxCostVal:   req.BoxCostVal,
		BobinCostVal: req.BobinCostVal,
		BbnReuseVal:  req.BbnReuseVal,
		BoxReuseVal:  req.BoxReuseVal,
		UpdatedBy:    getUserFromContext(ctx),
	})
	if err != nil {
		RecordBoxBobbinCostOperation("update", false)
		return &financev1.UpdateBoxBobbinCostResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordBoxBobbinCostOperation("update", true)
	return &financev1.UpdateBoxBobbinCostResponse{
		Base: successResponse("Box bobbin cost updated successfully"),
		Data: boxBobbinCostEntityToProto(entity),
	}, nil
}

// DeleteBoxBobbinCost soft-deletes a box bobbin cost config.
func (h *BoxBobbinCostHandler) DeleteBoxBobbinCost(ctx context.Context, req *financev1.DeleteBoxBobbinCostRequest) (*financev1.DeleteBoxBobbinCostResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordBoxBobbinCostOperation("delete", false)
		return &financev1.DeleteBoxBobbinCostResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.BbcId)
	if err != nil {
		RecordBoxBobbinCostOperation("delete", false)
		return &financev1.DeleteBoxBobbinCostResponse{Base: invalidIDResponse("bbc_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	if err := h.deleteHandler.Handle(ctx, appboxbobbincost.DeleteCommand{ID: id, DeletedBy: getUserFromContext(ctx)}); err != nil {
		RecordBoxBobbinCostOperation("delete", false)
		return &financev1.DeleteBoxBobbinCostResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordBoxBobbinCostOperation("delete", true)
	return &financev1.DeleteBoxBobbinCostResponse{Base: successResponse("Box bobbin cost deleted successfully")}, nil
}

// ListBoxBobbinCosts lists box bobbin cost configs with search, filter, and pagination.
func (h *BoxBobbinCostHandler) ListBoxBobbinCosts(ctx context.Context, req *financev1.ListBoxBobbinCostsRequest) (*financev1.ListBoxBobbinCostsResponse, error) {
	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := appboxbobbincost.ListQuery{
		Page:      page,
		PageSize:  pageSize,
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		t := true
		query.IsActive = &t
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		f := false
		query.IsActive = &f
	default:
	}

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		RecordBoxBobbinCostOperation("list", false)
		return &financev1.ListBoxBobbinCostsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordBoxBobbinCostOperation("list", true)

	items := make([]*financev1.BoxBobbinCost, len(result.Items))
	for i, e := range result.Items {
		items[i] = boxBobbinCostEntityToProto(e)
	}

	return &financev1.ListBoxBobbinCostsResponse{
		Base: successResponse("Box bobbin costs retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// CreateBoxBobbinCostRate adds a rate entry to an existing box bobbin cost config.
func (h *BoxBobbinCostHandler) CreateBoxBobbinCostRate(ctx context.Context, req *financev1.CreateBoxBobbinCostRateRequest) (*financev1.CreateBoxBobbinCostRateResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordBoxBobbinCostOperation("create_rate", false)
		return &financev1.CreateBoxBobbinCostRateResponse{Base: baseResp}, nil
	}

	parentID, err := uuid.Parse(req.BbcId)
	if err != nil {
		RecordBoxBobbinCostOperation("create_rate", false)
		return &financev1.CreateBoxBobbinCostRateResponse{Base: invalidIDResponse("bbc_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	rate, err := h.createRateHandler.Handle(ctx, appboxbobbincost.CreateRateCommand{
		ParentID:   parentID,
		Period:     req.BbcrPeriod,
		BobRateMkt: req.BbcrBobRateMkt,
		BoxRateMkt: req.BbcrBoxRateMkt,
		BobRateVal: req.BbcrBobRateVal,
		BoxRateVal: req.BbcrBoxRateVal,
		CreatedBy:  getUserFromContext(ctx),
	})
	if err != nil {
		RecordBoxBobbinCostOperation("create_rate", false)
		return &financev1.CreateBoxBobbinCostRateResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordBoxBobbinCostOperation("create_rate", true)
	return &financev1.CreateBoxBobbinCostRateResponse{
		Base: successResponse("Box bobbin cost rate created successfully"),
		Data: boxBobbinCostRateToProto(rate),
	}, nil
}

// DeleteBoxBobbinCostRate removes a rate entry from a box bobbin cost config.
func (h *BoxBobbinCostHandler) DeleteBoxBobbinCostRate(ctx context.Context, req *financev1.DeleteBoxBobbinCostRateRequest) (*financev1.DeleteBoxBobbinCostRateResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordBoxBobbinCostOperation("delete_rate", false)
		return &financev1.DeleteBoxBobbinCostRateResponse{Base: baseResp}, nil
	}

	rateID, err := uuid.Parse(req.BbcrId)
	if err != nil {
		RecordBoxBobbinCostOperation("delete_rate", false)
		return &financev1.DeleteBoxBobbinCostRateResponse{Base: invalidIDResponse("bbcr_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	if err := h.deleteRateHandler.Handle(ctx, appboxbobbincost.DeleteRateCommand{RateID: rateID, DeletedBy: getUserFromContext(ctx)}); err != nil {
		RecordBoxBobbinCostOperation("delete_rate", false)
		return &financev1.DeleteBoxBobbinCostRateResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordBoxBobbinCostOperation("delete_rate", true)
	return &financev1.DeleteBoxBobbinCostRateResponse{Base: successResponse("Box bobbin cost rate deleted successfully")}, nil
}

// ExportBoxBobbinCosts is not yet implemented.
func (h *BoxBobbinCostHandler) ExportBoxBobbinCosts(_ context.Context, _ *financev1.ExportBoxBobbinCostsRequest) (*financev1.ExportBoxBobbinCostsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ExportBoxBobbinCosts not implemented")
}

// ImportBoxBobbinCosts is not yet implemented.
func (h *BoxBobbinCostHandler) ImportBoxBobbinCosts(_ context.Context, _ *financev1.ImportBoxBobbinCostsRequest) (*financev1.ImportBoxBobbinCostsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ImportBoxBobbinCosts not implemented")
}

// DownloadBoxBobbinCostTemplate is not yet implemented.
func (h *BoxBobbinCostHandler) DownloadBoxBobbinCostTemplate(_ context.Context, _ *financev1.DownloadBoxBobbinCostTemplateRequest) (*financev1.DownloadBoxBobbinCostTemplateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DownloadBoxBobbinCostTemplate not implemented")
}

// boxBobbinCostEntityToProto converts a domain BoxBobbinCost entity to its proto representation.
func boxBobbinCostEntityToProto(e *boxbobbincost.Entity) *financev1.BoxBobbinCost {
	p := &financev1.BoxBobbinCost{
		BbcId:        e.ID().String(),
		BbcCode:      e.Code(),
		BbcName:      e.Name(),
		BbcType:      e.BBCType(),
		NoOfBob:      safeconv.IntToInt32(e.NoOfBob()),
		IsActive:     e.IsActive(),
		Notes:        e.Notes(),
		BbnReuse:     e.BbnReuse(),
		BoxReuse:     e.BoxReuse(),
		BoxCost:      e.BoxCost(),
		BobinCost:    e.BobinCost(),
		BoxCostVal:   e.BoxCostVal(),
		BobinCostVal: e.BobinCostVal(),
		BbnReuseVal:  e.BbnReuseVal(),
		BoxReuseVal:  e.BoxReuseVal(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	for _, r := range e.Rates() {
		p.Rates = append(p.Rates, boxBobbinCostRateToProto(r))
	}
	if e.UpdatedAt() != nil {
		p.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		p.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return p
}

// boxBobbinCostRateToProto converts a domain RateEntry to its proto representation.
func boxBobbinCostRateToProto(r *boxbobbincost.RateEntry) *financev1.BoxBobbinCostRate {
	p := &financev1.BoxBobbinCostRate{
		BbcrId:         r.ID().String(),
		BbcrBbcId:      r.ParentID().String(),
		BbcrPeriod:     r.Period(),
		BbcrBobRateMkt: r.BobRateMkt(),
		BbcrBoxRateMkt: r.BoxRateMkt(),
		BbcrBobRateVal: r.BobRateVal(),
		BbcrBoxRateVal: r.BoxRateVal(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: r.CreatedAt().Format(time.RFC3339),
			CreatedBy: r.CreatedBy(),
		},
	}
	if r.UpdatedAt() != nil {
		p.Audit.UpdatedAt = r.UpdatedAt().Format(time.RFC3339)
	}
	if r.UpdatedBy() != nil {
		p.Audit.UpdatedBy = *r.UpdatedBy()
	}
	return p
}
