package grpc

import (
	"context"
	"errors"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costrequesttype"
)

// CostRequestTypeHandler implements financev1.CostRequestTypeServiceServer.
type CostRequestTypeHandler struct {
	financev1.UnimplementedCostRequestTypeServiceServer
	repo       domain.Repository
	validation *ValidationHelper
}

// NewCostRequestTypeHandler constructs the handler.
func NewCostRequestTypeHandler(repo domain.Repository) (*CostRequestTypeHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &CostRequestTypeHandler{repo: repo, validation: v}, nil
}

// ListCostRequestTypes returns a paginated list.
func (h *CostRequestTypeHandler) ListCostRequestTypes(ctx context.Context, req *financev1.ListCostRequestTypesRequest) (*financev1.ListCostRequestTypesResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostRequestTypesResponse{Base: baseResp}, nil
	}
	page, pageSize := paginationFromProto(req.Pagination)
	items, total, err := h.repo.List(ctx, domain.Filter{
		Search: req.GetSearch(), ActiveFilter: req.GetActiveFilter(),
		Page: int(page), PageSize: int(pageSize),
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return &financev1.ListCostRequestTypesResponse{Base: NotFoundResponse(err.Error())}, nil
		}
		return &financev1.ListCostRequestTypesResponse{Base: InternalErrorResponse(err.Error())}, nil
	}
	data := make([]*financev1.CostRequestType, 0, len(items))
	for _, t := range items {
		data = append(data, &financev1.CostRequestType{
			TypeId: t.TypeID, Code: t.Code, DisplayName: t.DisplayName,
			StateMachineVariant: t.StateMachineVariant, DefaultUrgency: t.DefaultUrgency, IsActive: t.IsActive,
		})
	}
	return &financev1.ListCostRequestTypesResponse{
		Base:       successResponse("OK"),
		Data:       data,
		Pagination: paginationResponse(page, pageSize, total),
	}, nil
}

// Ensure commonv1 import is used (PaginationResponse handled by paginationResponse helper).
var _ = commonv1.PaginationResponse{}
