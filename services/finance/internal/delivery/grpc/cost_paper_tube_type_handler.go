package grpc

import (
	"context"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costpapertubetype"
)

// CostPaperTubeTypeHandler implements financev1.CostPaperTubeTypeServiceServer.
type CostPaperTubeTypeHandler struct {
	financev1.UnimplementedCostPaperTubeTypeServiceServer
	repo       domain.Repository
	validation *ValidationHelper
}

// NewCostPaperTubeTypeHandler constructs the handler.
func NewCostPaperTubeTypeHandler(repo domain.Repository) (*CostPaperTubeTypeHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &CostPaperTubeTypeHandler{repo: repo, validation: v}, nil
}

// ListCostPaperTubeTypes returns a paginated list.
func (h *CostPaperTubeTypeHandler) ListCostPaperTubeTypes(ctx context.Context, req *financev1.ListCostPaperTubeTypesRequest) (*financev1.ListCostPaperTubeTypesResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostPaperTubeTypesResponse{Base: baseResp}, nil
	}
	page, pageSize := paginationFromProto(req.Pagination)
	items, total, err := h.repo.List(ctx, domain.Filter{
		Search: req.GetSearch(), ActiveFilter: req.GetActiveFilter(),
		Page: int(page), PageSize: int(pageSize),
	})
	if err != nil {
		return &financev1.ListCostPaperTubeTypesResponse{Base: InternalErrorResponse(err.Error())}, nil
	}
	data := make([]*financev1.CostPaperTubeType, 0, len(items))
	for _, t := range items {
		data = append(data, &financev1.CostPaperTubeType{
			PaperTubeTypeId: t.PaperTubeTypeID, Code: t.Code,
			DisplayName: t.DisplayName, IsActive: t.IsActive,
		})
	}
	return &financev1.ListCostPaperTubeTypesResponse{
		Base:       successResponse("OK"),
		Data:       data,
		Pagination: paginationResponse(page, pageSize, total),
	}, nil
}
