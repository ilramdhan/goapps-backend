// Package grpc provides gRPC server implementation for the finance service.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appspinfixedcost "github.com/mutugading/goapps-backend/services/finance/internal/application/spinfixedcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/spinfixedcost"
)

const spinFixedCostIDField = "id"

// SpinFixedCostHandler implements financev1.SpinFixedCostServiceServer.
type SpinFixedCostHandler struct {
	financev1.UnimplementedSpinFixedCostServiceServer
	createHandler *appspinfixedcost.CreateHandler
	getHandler    *appspinfixedcost.GetHandler
	listHandler   *appspinfixedcost.ListHandler
	updateHandler *appspinfixedcost.UpdateHandler
	deleteHandler *appspinfixedcost.DeleteHandler
	validation    *ValidationHelper
}

// NewSpinFixedCostHandler constructs a SpinFixedCostHandler.
func NewSpinFixedCostHandler(repo spinfixedcost.Repository) (*SpinFixedCostHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &SpinFixedCostHandler{
		createHandler: appspinfixedcost.NewCreateHandler(repo),
		getHandler:    appspinfixedcost.NewGetHandler(repo),
		listHandler:   appspinfixedcost.NewListHandler(repo),
		updateHandler: appspinfixedcost.NewUpdateHandler(repo),
		deleteHandler: appspinfixedcost.NewDeleteHandler(repo),
		validation:    v,
	}, nil
}

// CreateSpinFixedCost creates a new spin fixed cost record.
func (h *SpinFixedCostHandler) CreateSpinFixedCost(ctx context.Context, req *financev1.CreateSpinFixedCostRequest) (*financev1.CreateSpinFixedCostResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordSpinFixedCostOperation("create", false)
		return &financev1.CreateSpinFixedCostResponse{Base: baseResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appspinfixedcost.CreateCommand{
		Period:             req.Period,
		CommonPoyDenier:    req.CommonPoyDenier,
		PoyProduction:      req.PoyProduction,
		SpinPowerMonth:     req.SpinPowerMonth,
		SpinManpowerMonth:  req.SpinManpowerMonth,
		SpinOverheadsMonth: req.SpinOverheadsMonth,
		SpinConssprsMonth:  req.SpinConssprsMonth,
		CreatedBy:          getUserFromContext(ctx),
	})
	if err != nil {
		RecordSpinFixedCostOperation("create", false)
		return &financev1.CreateSpinFixedCostResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordSpinFixedCostOperation("create", true)
	return &financev1.CreateSpinFixedCostResponse{
		Base: successResponse("Spin fixed cost created successfully"),
		Data: spinFixedCostEntityToProto(entity),
	}, nil
}

// GetSpinFixedCost retrieves a spin fixed cost record by ID.
func (h *SpinFixedCostHandler) GetSpinFixedCost(ctx context.Context, req *financev1.GetSpinFixedCostRequest) (*financev1.GetSpinFixedCostResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordSpinFixedCostOperation("get", false)
		return &financev1.GetSpinFixedCostResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.Id)
	if err != nil {
		RecordSpinFixedCostOperation("get", false)
		return &financev1.GetSpinFixedCostResponse{Base: invalidIDResponse(spinFixedCostIDField)}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.getHandler.Handle(ctx, appspinfixedcost.GetQuery{ID: id})
	if err != nil {
		RecordSpinFixedCostOperation("get", false)
		return &financev1.GetSpinFixedCostResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordSpinFixedCostOperation("get", true)
	return &financev1.GetSpinFixedCostResponse{
		Base: successResponse("Spin fixed cost retrieved successfully"),
		Data: spinFixedCostEntityToProto(entity),
	}, nil
}

// UpdateSpinFixedCost updates an existing spin fixed cost record.
func (h *SpinFixedCostHandler) UpdateSpinFixedCost(ctx context.Context, req *financev1.UpdateSpinFixedCostRequest) (*financev1.UpdateSpinFixedCostResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordSpinFixedCostOperation("update", false)
		return &financev1.UpdateSpinFixedCostResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.Id)
	if err != nil {
		RecordSpinFixedCostOperation("update", false)
		return &financev1.UpdateSpinFixedCostResponse{Base: invalidIDResponse(spinFixedCostIDField)}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.updateHandler.Handle(ctx, appspinfixedcost.UpdateCommand{
		ID:                 id,
		CommonPoyDenier:    req.CommonPoyDenier,
		PoyProduction:      req.PoyProduction,
		SpinPowerMonth:     req.SpinPowerMonth,
		SpinManpowerMonth:  req.SpinManpowerMonth,
		SpinOverheadsMonth: req.SpinOverheadsMonth,
		SpinConssprsMonth:  req.SpinConssprsMonth,
		IsActive:           req.IsActive,
		UpdatedBy:          getUserFromContext(ctx),
	})
	if err != nil {
		RecordSpinFixedCostOperation("update", false)
		return &financev1.UpdateSpinFixedCostResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordSpinFixedCostOperation("update", true)
	return &financev1.UpdateSpinFixedCostResponse{
		Base: successResponse("Spin fixed cost updated successfully"),
		Data: spinFixedCostEntityToProto(entity),
	}, nil
}

// DeleteSpinFixedCost soft-deletes a spin fixed cost record.
func (h *SpinFixedCostHandler) DeleteSpinFixedCost(ctx context.Context, req *financev1.DeleteSpinFixedCostRequest) (*financev1.DeleteSpinFixedCostResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordSpinFixedCostOperation("delete", false)
		return &financev1.DeleteSpinFixedCostResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.Id)
	if err != nil {
		RecordSpinFixedCostOperation("delete", false)
		return &financev1.DeleteSpinFixedCostResponse{Base: invalidIDResponse(spinFixedCostIDField)}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	if err := h.deleteHandler.Handle(ctx, appspinfixedcost.DeleteCommand{ID: id, DeletedBy: getUserFromContext(ctx)}); err != nil {
		RecordSpinFixedCostOperation("delete", false)
		return &financev1.DeleteSpinFixedCostResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordSpinFixedCostOperation("delete", true)
	return &financev1.DeleteSpinFixedCostResponse{Base: successResponse("Spin fixed cost deleted successfully")}, nil
}

// ListSpinFixedCosts lists spin fixed cost records with search, filter, and pagination.
func (h *SpinFixedCostHandler) ListSpinFixedCosts(ctx context.Context, req *financev1.ListSpinFixedCostsRequest) (*financev1.ListSpinFixedCostsResponse, error) {
	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := appspinfixedcost.ListQuery{
		Page:      page,
		PageSize:  pageSize,
		Search:    req.Search,
		Period:    req.Period,
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
		RecordSpinFixedCostOperation("list", false)
		return &financev1.ListSpinFixedCostsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordSpinFixedCostOperation("list", true)

	items := make([]*financev1.SpinFixedCost, len(result.Items))
	for i, e := range result.Items {
		items[i] = spinFixedCostEntityToProto(e)
	}

	return &financev1.ListSpinFixedCostsResponse{
		Base: successResponse("Spin fixed costs retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// spinFixedCostEntityToProto converts a domain Spin Fixed Cost entity to its proto representation.
func spinFixedCostEntityToProto(e *spinfixedcost.Entity) *financev1.SpinFixedCost {
	p := &financev1.SpinFixedCost{
		Id:                 e.ID().String(),
		Period:             e.Period(),
		CommonPoyDenier:    e.CommonPoyDenier(),
		PoyProduction:      e.PoyProduction(),
		SpinPowerMonth:     e.SpinPowerMonth(),
		SpinManpowerMonth:  e.SpinManpowerMonth(),
		SpinOverheadsMonth: e.SpinOverheadsMonth(),
		SpinConssprsMonth:  e.SpinConssprsMonth(),
		IsActive:           e.IsActive(),
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
