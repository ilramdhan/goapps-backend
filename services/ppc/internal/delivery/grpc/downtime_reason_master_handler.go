// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	downtimereasonapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/downtimereason"
	downtimedomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/downtimereason"
)

// downtimeReasonHandler implements the downtime-reason RPCs of PPCService.
type downtimeReasonHandler struct {
	svc *downtimereasonapp.Service
}

func newDowntimeReasonHandler(svc *downtimereasonapp.Service) *downtimeReasonHandler {
	return &downtimeReasonHandler{svc: svc}
}

// CreateDowntimeReasonMaster creates a new downtime reason.
func (h *downtimeReasonHandler) CreateDowntimeReasonMaster(ctx context.Context, req *ppcv1.CreateDowntimeReasonMasterRequest) (*ppcv1.CreateDowntimeReasonMasterResponse, error) {
	entity, err := h.svc.Create(ctx, downtimereasonapp.CreateCommand{
		Area:             areaCodeToString(req.GetArea()),
		Code:             req.GetCode(),
		Name:             req.GetName(),
		Category:         req.GetCategory(),
		IsExcludeFromEff: req.GetIsExcludeFromEff(),
		SortOrder:        req.GetSortOrder(),
		CreatedBy:        getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.CreateDowntimeReasonMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateDowntimeReasonMasterResponse{
		Base: successResponse("Downtime reason created successfully"),
		Data: downtimeReasonToProto(entity),
	}, nil
}

// GetDowntimeReasonMaster retrieves a downtime reason by ID.
func (h *downtimeReasonHandler) GetDowntimeReasonMaster(ctx context.Context, req *ppcv1.GetDowntimeReasonMasterRequest) (*ppcv1.GetDowntimeReasonMasterResponse, error) {
	entity, err := h.svc.Get(ctx, req.GetReasonId())
	if err != nil {
		return &ppcv1.GetDowntimeReasonMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetDowntimeReasonMasterResponse{
		Base: successResponse("Downtime reason retrieved successfully"),
		Data: downtimeReasonToProto(entity),
	}, nil
}

// UpdateDowntimeReasonMaster updates an existing downtime reason.
func (h *downtimeReasonHandler) UpdateDowntimeReasonMaster(ctx context.Context, req *ppcv1.UpdateDowntimeReasonMasterRequest) (*ppcv1.UpdateDowntimeReasonMasterResponse, error) {
	entity, err := h.svc.Update(ctx, downtimereasonapp.UpdateCommand{
		ID:               req.GetReasonId(),
		Name:             req.Name,
		Category:         req.Category,
		IsExcludeFromEff: req.IsExcludeFromEff,
		IsActive:         req.IsActive,
		SortOrder:        req.SortOrder,
		UpdatedBy:        getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.UpdateDowntimeReasonMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateDowntimeReasonMasterResponse{
		Base: successResponse("Downtime reason updated successfully"),
		Data: downtimeReasonToProto(entity),
	}, nil
}

// DeleteDowntimeReasonMaster deletes a downtime reason.
func (h *downtimeReasonHandler) DeleteDowntimeReasonMaster(ctx context.Context, req *ppcv1.DeleteDowntimeReasonMasterRequest) (*ppcv1.DeleteDowntimeReasonMasterResponse, error) {
	if err := h.svc.Delete(ctx, req.GetReasonId()); err != nil {
		return &ppcv1.DeleteDowntimeReasonMasterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeleteDowntimeReasonMasterResponse{Base: successResponse("Downtime reason deleted successfully")}, nil
}

// ListDowntimeReasonMasters lists downtime reasons with filtering and pagination.
func (h *downtimeReasonHandler) ListDowntimeReasonMasters(ctx context.Context, req *ppcv1.ListDowntimeReasonMastersRequest) (*ppcv1.ListDowntimeReasonMastersResponse, error) {
	result, err := h.svc.List(ctx, downtimereasonapp.ListQuery{
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		Search:    req.GetSearch(),
		Area:      areaCodeToString(req.GetArea()),
		IsActive:  activeFilterToBool(req.GetActiveFilter()),
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListDowntimeReasonMastersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.DowntimeReasonMaster, len(result.Items))
	for i, entity := range result.Items {
		items[i] = downtimeReasonToProto(entity)
	}
	return &ppcv1.ListDowntimeReasonMastersResponse{
		Base: successResponse("Downtime reasons retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

func downtimeReasonToProto(e *downtimedomain.Reason) *ppcv1.DowntimeReasonMaster {
	proto := &ppcv1.DowntimeReasonMaster{
		ReasonId:         e.ID(),
		Area:             stringToAreaCode(e.Area().String()),
		Code:             e.Code(),
		Name:             e.Name(),
		Category:         e.Category(),
		IsExcludeFromEff: e.IsExcludeFromEff(),
		IsActive:         e.IsActive(),
		SortOrder:        e.SortOrder(),
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
