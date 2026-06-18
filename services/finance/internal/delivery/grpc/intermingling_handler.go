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
	appintermingling "github.com/mutugading/goapps-backend/services/finance/internal/application/intermingling"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
)

// InterminglingHandler implements financev1.InterminglingServiceServer.
type InterminglingHandler struct {
	financev1.UnimplementedInterminglingServiceServer
	createHandler *appintermingling.CreateHandler
	getHandler    *appintermingling.GetHandler
	listHandler   *appintermingling.ListHandler
	updateHandler *appintermingling.UpdateHandler
	deleteHandler *appintermingling.DeleteHandler
	validation    *ValidationHelper
}

// NewInterminglingHandler constructs an InterminglingHandler.
func NewInterminglingHandler(repo intermingling.Repository) (*InterminglingHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &InterminglingHandler{
		createHandler: appintermingling.NewCreateHandler(repo),
		getHandler:    appintermingling.NewGetHandler(repo),
		listHandler:   appintermingling.NewListHandler(repo),
		updateHandler: appintermingling.NewUpdateHandler(repo),
		deleteHandler: appintermingling.NewDeleteHandler(repo),
		validation:    v,
	}, nil
}

// CreateIntermingling creates a new intermingling record.
func (h *InterminglingHandler) CreateIntermingling(ctx context.Context, req *financev1.CreateInterminglingRequest) (*financev1.CreateInterminglingResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordInterminglingOperation("create", false)
		return &financev1.CreateInterminglingResponse{Base: baseResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appintermingling.CreateCommand{
		Code:      req.IntmCode,
		Name:      req.IntmName,
		CostPerKg: req.IntmCostPerKg,
		Notes:     req.Notes,
		CreatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		RecordInterminglingOperation("create", false)
		return &financev1.CreateInterminglingResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordInterminglingOperation("create", true)
	return &financev1.CreateInterminglingResponse{
		Base: successResponse("Intermingling created successfully"),
		Data: interminglingEntityToProto(entity),
	}, nil
}

// GetIntermingling retrieves an intermingling record by ID.
func (h *InterminglingHandler) GetIntermingling(ctx context.Context, req *financev1.GetInterminglingRequest) (*financev1.GetInterminglingResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordInterminglingOperation("get", false)
		return &financev1.GetInterminglingResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.IntmId)
	if err != nil {
		RecordInterminglingOperation("get", false)
		return &financev1.GetInterminglingResponse{Base: invalidIDResponse("intm_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.getHandler.Handle(ctx, appintermingling.GetQuery{ID: id})
	if err != nil {
		RecordInterminglingOperation("get", false)
		return &financev1.GetInterminglingResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordInterminglingOperation("get", true)
	return &financev1.GetInterminglingResponse{
		Base: successResponse("Intermingling retrieved successfully"),
		Data: interminglingEntityToProto(entity),
	}, nil
}

// UpdateIntermingling updates an existing intermingling record.
func (h *InterminglingHandler) UpdateIntermingling(ctx context.Context, req *financev1.UpdateInterminglingRequest) (*financev1.UpdateInterminglingResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordInterminglingOperation("update", false)
		return &financev1.UpdateInterminglingResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.IntmId)
	if err != nil {
		RecordInterminglingOperation("update", false)
		return &financev1.UpdateInterminglingResponse{Base: invalidIDResponse("intm_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.updateHandler.Handle(ctx, appintermingling.UpdateCommand{
		ID:        id,
		Name:      req.IntmName,
		CostPerKg: req.IntmCostPerKg,
		Notes:     req.Notes,
		IsActive:  req.IsActive,
		UpdatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		RecordInterminglingOperation("update", false)
		return &financev1.UpdateInterminglingResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordInterminglingOperation("update", true)
	return &financev1.UpdateInterminglingResponse{
		Base: successResponse("Intermingling updated successfully"),
		Data: interminglingEntityToProto(entity),
	}, nil
}

// DeleteIntermingling soft-deletes an intermingling record.
func (h *InterminglingHandler) DeleteIntermingling(ctx context.Context, req *financev1.DeleteInterminglingRequest) (*financev1.DeleteInterminglingResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordInterminglingOperation("delete", false)
		return &financev1.DeleteInterminglingResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.IntmId)
	if err != nil {
		RecordInterminglingOperation("delete", false)
		return &financev1.DeleteInterminglingResponse{Base: invalidIDResponse("intm_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	if err := h.deleteHandler.Handle(ctx, appintermingling.DeleteCommand{ID: id, DeletedBy: getUserFromContext(ctx)}); err != nil {
		RecordInterminglingOperation("delete", false)
		return &financev1.DeleteInterminglingResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordInterminglingOperation("delete", true)
	return &financev1.DeleteInterminglingResponse{Base: successResponse("Intermingling deleted successfully")}, nil
}

// ListInterminglings lists intermingling records with search, filter, and pagination.
func (h *InterminglingHandler) ListInterminglings(ctx context.Context, req *financev1.ListInterminglingsRequest) (*financev1.ListInterminglingsResponse, error) {
	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := appintermingling.ListQuery{
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
		RecordInterminglingOperation("list", false)
		return &financev1.ListInterminglingsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordInterminglingOperation("list", true)

	items := make([]*financev1.Intermingling, len(result.Items))
	for i, e := range result.Items {
		items[i] = interminglingEntityToProto(e)
	}

	return &financev1.ListInterminglingsResponse{
		Base: successResponse("Interminglings retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportInterminglings is not yet implemented.
func (h *InterminglingHandler) ExportInterminglings(_ context.Context, _ *financev1.ExportInterminglingsRequest) (*financev1.ExportInterminglingsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ExportInterminglings not implemented")
}

// ImportInterminglings is not yet implemented.
func (h *InterminglingHandler) ImportInterminglings(_ context.Context, _ *financev1.ImportInterminglingsRequest) (*financev1.ImportInterminglingsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ImportInterminglings not implemented")
}

// DownloadInterminglingTemplate is not yet implemented.
func (h *InterminglingHandler) DownloadInterminglingTemplate(_ context.Context, _ *financev1.DownloadInterminglingTemplateRequest) (*financev1.DownloadInterminglingTemplateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DownloadInterminglingTemplate not implemented")
}

// interminglingEntityToProto converts a domain Intermingling entity to its proto representation.
func interminglingEntityToProto(e *intermingling.Entity) *financev1.Intermingling {
	p := &financev1.Intermingling{
		IntmId:        e.ID().String(),
		IntmCode:      e.Code(),
		IntmName:      e.Name(),
		IntmCostPerKg: e.CostPerKg(),
		IsActive:      e.IsActive(),
		Notes:         e.Notes(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.UpdatedAt() != nil {
		p.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		p.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return p
}
