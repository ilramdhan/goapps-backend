package grpc

import (
	"context"
	"errors"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costerp"
)

// CostErpHandler implements financev1.CostErpLookupServiceServer.
type CostErpHandler struct {
	financev1.UnimplementedCostErpLookupServiceServer
	repo       domain.Repository
	validation *ValidationHelper
}

// NewCostErpHandler constructs the handler.
func NewCostErpHandler(repo domain.Repository) (*CostErpHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &CostErpHandler{repo: repo, validation: v}, nil
}

// =============================================================================
// Items
// =============================================================================

// ListCostErpItems lists ERP items.
func (h *CostErpHandler) ListCostErpItems(ctx context.Context, req *financev1.ListCostErpItemsRequest) (*financev1.ListCostErpItemsResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostErpItemsResponse{Base: baseResp}, nil
	}
	page, pageSize := paginationFromProto(req.Pagination)
	items, total, err := h.repo.ListItems(ctx, domain.ItemFilter{
		Search: req.GetSearch(), ItemType: req.GetItemType(),
		ActiveFilter: req.GetActiveFilter(),
		Page:         int(page), PageSize: int(pageSize),
	})
	if err != nil {
		return &financev1.ListCostErpItemsResponse{Base: erpErrToBase(err)}, nil
	}
	data := make([]*financev1.CostErpItem, 0, len(items))
	for _, it := range items {
		data = append(data, erpItemToProto(it))
	}
	return &financev1.ListCostErpItemsResponse{
		Base:       successResponse("OK"),
		Data:       data,
		Pagination: paginationResponse(page, pageSize, total),
	}, nil
}

// GetCostErpItem returns one ERP item.
func (h *CostErpHandler) GetCostErpItem(ctx context.Context, req *financev1.GetCostErpItemRequest) (*financev1.GetCostErpItemResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.GetCostErpItemResponse{Base: baseResp}, nil
	}
	it, err := h.repo.GetItem(ctx, req.GetItemId())
	if err != nil {
		return &financev1.GetCostErpItemResponse{Base: erpErrToBase(err)}, nil
	}
	return &financev1.GetCostErpItemResponse{
		Base: successResponse("OK"),
		Data: erpItemToProto(it),
	}, nil
}

// =============================================================================
// Grades
// =============================================================================

// ListCostErpGrades lists ERP grades.
func (h *CostErpHandler) ListCostErpGrades(ctx context.Context, req *financev1.ListCostErpGradesRequest) (*financev1.ListCostErpGradesResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostErpGradesResponse{Base: baseResp}, nil
	}
	page, pageSize := paginationFromProto(req.Pagination)
	items, total, err := h.repo.ListGrades(ctx, domain.LookupFilter{
		Search: req.GetSearch(), ActiveFilter: req.GetActiveFilter(),
		Page: int(page), PageSize: int(pageSize),
	})
	if err != nil {
		return &financev1.ListCostErpGradesResponse{Base: erpErrToBase(err)}, nil
	}
	data := make([]*financev1.CostErpGrade, 0, len(items))
	for _, g := range items {
		data = append(data, &financev1.CostErpGrade{
			GradeId: g.GradeID, GradeCode: g.GradeCode, GradeName: g.GradeName,
			IsActive: g.IsActive, SyncedAt: g.SyncedAt.Format(time.RFC3339),
		})
	}
	return &financev1.ListCostErpGradesResponse{
		Base:       successResponse("OK"),
		Data:       data,
		Pagination: paginationResponse(page, pageSize, total),
	}, nil
}

// =============================================================================
// Shades
// =============================================================================

// ListCostErpShades lists ERP shades.
func (h *CostErpHandler) ListCostErpShades(ctx context.Context, req *financev1.ListCostErpShadesRequest) (*financev1.ListCostErpShadesResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostErpShadesResponse{Base: baseResp}, nil
	}
	page, pageSize := paginationFromProto(req.Pagination)
	items, total, err := h.repo.ListShades(ctx, domain.LookupFilter{
		Search: req.GetSearch(), ActiveFilter: req.GetActiveFilter(),
		Page: int(page), PageSize: int(pageSize),
	})
	if err != nil {
		return &financev1.ListCostErpShadesResponse{Base: erpErrToBase(err)}, nil
	}
	data := make([]*financev1.CostErpShade, 0, len(items))
	for _, s := range items {
		data = append(data, &financev1.CostErpShade{
			ShadeId: s.ShadeID, ShadeCode: s.ShadeCode, ShadeName: s.ShadeName,
			IsActive: s.IsActive, SyncedAt: s.SyncedAt.Format(time.RFC3339),
		})
	}
	return &financev1.ListCostErpShadesResponse{
		Base:       successResponse("OK"),
		Data:       data,
		Pagination: paginationResponse(page, pageSize, total),
	}, nil
}

// =============================================================================
// helpers
// =============================================================================

func erpItemToProto(it *domain.Item) *financev1.CostErpItem {
	return &financev1.CostErpItem{
		ItemId: it.ItemID, ItemCode: it.ItemCode, ItemName: it.ItemName,
		ItemType: it.ItemType, IsActive: it.IsActive,
		SyncedAt: it.SyncedAt.Format(time.RFC3339),
	}
}

func paginationFromProto(p *commonv1.PaginationRequest) (int32, int32) {
	page := int32(1)
	pageSize := int32(20)
	if p != nil {
		if p.Page > 0 {
			page = p.Page
		}
		if p.PageSize > 0 {
			pageSize = p.PageSize
		}
	}
	return page, pageSize
}

func paginationResponse(page, pageSize int32, total int64) *commonv1.PaginationResponse {
	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = safeIntToInt32(int((total + int64(pageSize) - 1) / int64(pageSize)))
	}
	return &commonv1.PaginationResponse{
		CurrentPage: page, PageSize: pageSize,
		TotalItems: total, TotalPages: totalPages,
	}
}

func erpErrToBase(err error) *commonv1.BaseResponse {
	if errors.Is(err, domain.ErrNotFound) {
		return NotFoundResponse(err.Error())
	}
	return InternalErrorResponse(err.Error())
}
