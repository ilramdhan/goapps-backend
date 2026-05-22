package grpc

import (
	"context"
	"errors"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costrmtype"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costrmtype"
)

// CostRmTypeHandler implements financev1.CostRmTypeServiceServer.
type CostRmTypeHandler struct {
	financev1.UnimplementedCostRmTypeServiceServer
	createHandler *app.CreateHandler
	getHandler    *app.GetHandler
	updateHandler *app.UpdateHandler
	listHandler   *app.ListHandler
	validation    *ValidationHelper
}

// NewCostRmTypeHandler constructs the handler.
func NewCostRmTypeHandler(repo domain.Repository) (*CostRmTypeHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &CostRmTypeHandler{
		createHandler: app.NewCreateHandler(repo),
		getHandler:    app.NewGetHandler(repo),
		updateHandler: app.NewUpdateHandler(repo),
		listHandler:   app.NewListHandler(repo),
		validation:    v,
	}, nil
}

// CreateCostRmType creates a new RM type.
func (h *CostRmTypeHandler) CreateCostRmType(ctx context.Context, req *financev1.CreateCostRmTypeRequest) (*financev1.CreateCostRmTypeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.CreateCostRmTypeResponse{Base: baseResp}, nil
	}
	t, err := h.createHandler.Handle(ctx, app.CreateCommand{
		TypeCode:         req.GetTypeCode(),
		TypeName:         req.GetTypeName(),
		ReferenceTarget:  req.GetReferenceTarget(),
		AllowSubSequence: req.GetAllowSubSequence(),
	})
	if err != nil {
		return &financev1.CreateCostRmTypeResponse{Base: rmTypeErrToBase(err)}, nil
	}
	return &financev1.CreateCostRmTypeResponse{
		Base: successResponse("Cost RM type created"),
		Data: costRmTypeToProto(t),
	}, nil
}

// GetCostRmType returns by id.
func (h *CostRmTypeHandler) GetCostRmType(ctx context.Context, req *financev1.GetCostRmTypeRequest) (*financev1.GetCostRmTypeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.GetCostRmTypeResponse{Base: baseResp}, nil
	}
	t, err := h.getHandler.Handle(ctx, app.GetQuery{TypeID: req.GetTypeId()})
	if err != nil {
		return &financev1.GetCostRmTypeResponse{Base: rmTypeErrToBase(err)}, nil
	}
	return &financev1.GetCostRmTypeResponse{
		Base: successResponse("OK"),
		Data: costRmTypeToProto(t),
	}, nil
}

// UpdateCostRmType updates name + active flag.
func (h *CostRmTypeHandler) UpdateCostRmType(ctx context.Context, req *financev1.UpdateCostRmTypeRequest) (*financev1.UpdateCostRmTypeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateCostRmTypeResponse{Base: baseResp}, nil
	}
	t, err := h.updateHandler.Handle(ctx, app.UpdateCommand{
		TypeID:   req.GetTypeId(),
		TypeName: req.GetTypeName(),
		IsActive: req.GetIsActive(),
	})
	if err != nil {
		return &financev1.UpdateCostRmTypeResponse{Base: rmTypeErrToBase(err)}, nil
	}
	return &financev1.UpdateCostRmTypeResponse{
		Base: successResponse("Cost RM type updated"),
		Data: costRmTypeToProto(t),
	}, nil
}

// ListCostRmTypes paginates RM types.
func (h *CostRmTypeHandler) ListCostRmTypes(ctx context.Context, req *financev1.ListCostRmTypesRequest) (*financev1.ListCostRmTypesResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostRmTypesResponse{Base: baseResp}, nil
	}
	page := int32(1)
	pageSize := int32(20)
	if req.Pagination != nil {
		if req.Pagination.Page > 0 {
			page = req.Pagination.Page
		}
		if req.Pagination.PageSize > 0 {
			pageSize = req.Pagination.PageSize
		}
	}
	res, err := h.listHandler.Handle(ctx, app.ListQuery{
		Search:          req.GetSearch(),
		ReferenceTarget: req.GetReferenceTarget(),
		ActiveFilter:    req.GetActiveFilter(),
		Page:            int(page),
		PageSize:        int(pageSize),
	})
	if err != nil {
		return &financev1.ListCostRmTypesResponse{Base: rmTypeErrToBase(err)}, nil
	}
	items := make([]*financev1.CostRmType, 0, len(res.Items))
	for _, t := range res.Items {
		items = append(items, costRmTypeToProto(t))
	}
	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = int32((res.Total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &financev1.ListCostRmTypesResponse{
		Base: successResponse("OK"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: page,
			PageSize:    pageSize,
			TotalItems:  res.Total,
			TotalPages:  totalPages,
		},
	}, nil
}

// =============================================================================
// mappers
// =============================================================================

func costRmTypeToProto(t *domain.CostRmType) *financev1.CostRmType {
	return &financev1.CostRmType{
		TypeId:           t.TypeID(),
		TypeCode:         t.TypeCode(),
		TypeName:         t.TypeName(),
		ReferenceTarget:  t.ReferenceTarget(),
		AllowSubSequence: t.AllowSubSequence(),
		IsActive:         t.IsActive(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: t.CreatedAt().Format(time.RFC3339),
			UpdatedAt: t.UpdatedAt().Format(time.RFC3339),
		},
	}
}

func rmTypeErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return ConflictResponse(err.Error())
	case errors.Is(err, domain.ErrInvalidTypeCode),
		errors.Is(err, domain.ErrInvalidTypeName),
		errors.Is(err, domain.ErrInvalidReferenceTarget):
		return ErrorResponse("400", err.Error())
	default:
		return InternalErrorResponse(err.Error())
	}
}
