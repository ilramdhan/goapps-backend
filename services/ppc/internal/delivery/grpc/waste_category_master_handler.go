// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	wastecategoryapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/wastecategory"
	wastedomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/wastecategory"
)

// wasteCategoryHandler implements the waste-category RPCs of PPCService.
type wasteCategoryHandler struct {
	svc *wastecategoryapp.Service
}

func newWasteCategoryHandler(svc *wastecategoryapp.Service) *wasteCategoryHandler {
	return &wasteCategoryHandler{svc: svc}
}

// CreateWasteCategoryMaster creates a new waste category.
func (h *wasteCategoryHandler) CreateWasteCategoryMaster(ctx context.Context, req *ppcv1.CreateWasteCategoryMasterRequest) (*ppcv1.CreateWasteCategoryMasterResponse, error) {
	entity, err := h.svc.Create(ctx, wastecategoryapp.CreateCommand{
		Area:        areaCodeToString(req.GetArea()),
		Type:        req.GetType(),
		Code:        req.GetCode(),
		Name:        req.GetName(),
		GradeTarget: optionalString(req.GetGradeTarget()),
		SortOrder:   req.GetSortOrder(),
		CreatedBy:   getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.CreateWasteCategoryMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateWasteCategoryMasterResponse{
		Base: successResponse("Waste category created successfully"),
		Data: wasteCategoryToProto(entity),
	}, nil
}

// GetWasteCategoryMaster retrieves a waste category by ID.
func (h *wasteCategoryHandler) GetWasteCategoryMaster(ctx context.Context, req *ppcv1.GetWasteCategoryMasterRequest) (*ppcv1.GetWasteCategoryMasterResponse, error) {
	entity, err := h.svc.Get(ctx, req.GetCategoryId())
	if err != nil {
		return &ppcv1.GetWasteCategoryMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetWasteCategoryMasterResponse{
		Base: successResponse("Waste category retrieved successfully"),
		Data: wasteCategoryToProto(entity),
	}, nil
}

// UpdateWasteCategoryMaster updates an existing waste category.
func (h *wasteCategoryHandler) UpdateWasteCategoryMaster(ctx context.Context, req *ppcv1.UpdateWasteCategoryMasterRequest) (*ppcv1.UpdateWasteCategoryMasterResponse, error) {
	entity, err := h.svc.Update(ctx, wastecategoryapp.UpdateCommand{
		ID:          req.GetCategoryId(),
		Name:        req.Name,
		GradeTarget: req.GradeTarget,
		IsActive:    req.IsActive,
		SortOrder:   req.SortOrder,
		UpdatedBy:   getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.UpdateWasteCategoryMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateWasteCategoryMasterResponse{
		Base: successResponse("Waste category updated successfully"),
		Data: wasteCategoryToProto(entity),
	}, nil
}

// DeleteWasteCategoryMaster deletes a waste category.
func (h *wasteCategoryHandler) DeleteWasteCategoryMaster(ctx context.Context, req *ppcv1.DeleteWasteCategoryMasterRequest) (*ppcv1.DeleteWasteCategoryMasterResponse, error) {
	if err := h.svc.Delete(ctx, req.GetCategoryId()); err != nil {
		return &ppcv1.DeleteWasteCategoryMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeleteWasteCategoryMasterResponse{Base: successResponse("Waste category deleted successfully")}, nil
}

// ListWasteCategoryMasters lists waste categories with filtering and pagination.
func (h *wasteCategoryHandler) ListWasteCategoryMasters(ctx context.Context, req *ppcv1.ListWasteCategoryMastersRequest) (*ppcv1.ListWasteCategoryMastersResponse, error) {
	result, err := h.svc.List(ctx, wastecategoryapp.ListQuery{
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		Search:    req.GetSearch(),
		Area:      areaCodeToString(req.GetArea()),
		Type:      req.GetType(),
		IsActive:  activeFilterToBool(req.GetActiveFilter()),
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListWasteCategoryMastersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.WasteCategoryMaster, len(result.Items))
	for i, entity := range result.Items {
		items[i] = wasteCategoryToProto(entity)
	}
	return &ppcv1.ListWasteCategoryMastersResponse{
		Base: successResponse("Waste categories retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// optionalString maps an empty proto string to nil, otherwise a pointer to it.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func wasteCategoryToProto(e *wastedomain.Category) *ppcv1.WasteCategoryMaster {
	gradeTarget := ""
	if e.GradeTarget() != nil {
		gradeTarget = *e.GradeTarget()
	}
	proto := &ppcv1.WasteCategoryMaster{
		CategoryId:  e.ID(),
		Area:        stringToAreaCode(e.Area().String()),
		Type:        e.Type(),
		Code:        e.Code(),
		Name:        e.Name(),
		GradeTarget: gradeTarget,
		IsActive:    e.IsActive(),
		SortOrder:   e.SortOrder(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.UpdatedAt() != nil {
		proto.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		proto.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return proto
}
