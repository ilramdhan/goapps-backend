// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	lookupapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/lookup"
	lookupdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/lookup"
)

// lookupHandler implements the PpcLookup RPCs of PPCService.
type lookupHandler struct {
	svc *lookupapp.Service
}

func newLookupHandler(svc *lookupapp.Service) *lookupHandler {
	return &lookupHandler{svc: svc}
}

// CreatePpcLookup creates a new lookup row.
func (h *lookupHandler) CreatePpcLookup(ctx context.Context, req *ppcv1.CreatePpcLookupRequest) (*ppcv1.CreatePpcLookupResponse, error) {
	entity, err := h.svc.Create(ctx, lookupapp.CreateCommand{
		Category:  req.GetCategory(),
		Code:      req.GetCode(),
		Label:     req.GetLabel(),
		SortOrder: req.GetSortOrder(),
		CreatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.CreatePpcLookupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreatePpcLookupResponse{
		Base: successResponse("Lookup created successfully"),
		Data: lookupToProto(entity),
	}, nil
}

// GetPpcLookup retrieves a lookup by ID.
func (h *lookupHandler) GetPpcLookup(ctx context.Context, req *ppcv1.GetPpcLookupRequest) (*ppcv1.GetPpcLookupResponse, error) {
	entity, err := h.svc.Get(ctx, req.GetLookupId())
	if err != nil {
		return &ppcv1.GetPpcLookupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetPpcLookupResponse{
		Base: successResponse("Lookup retrieved successfully"),
		Data: lookupToProto(entity),
	}, nil
}

// UpdatePpcLookup updates an existing lookup.
func (h *lookupHandler) UpdatePpcLookup(ctx context.Context, req *ppcv1.UpdatePpcLookupRequest) (*ppcv1.UpdatePpcLookupResponse, error) {
	entity, err := h.svc.Update(ctx, lookupapp.UpdateCommand{
		ID:        req.GetLookupId(),
		Label:     req.Label,
		SortOrder: req.SortOrder,
		IsActive:  req.IsActive,
		UpdatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.UpdatePpcLookupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdatePpcLookupResponse{
		Base: successResponse("Lookup updated successfully"),
		Data: lookupToProto(entity),
	}, nil
}

// DeletePpcLookup deletes a lookup.
func (h *lookupHandler) DeletePpcLookup(ctx context.Context, req *ppcv1.DeletePpcLookupRequest) (*ppcv1.DeletePpcLookupResponse, error) {
	if err := h.svc.Delete(ctx, req.GetLookupId()); err != nil {
		return &ppcv1.DeletePpcLookupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeletePpcLookupResponse{Base: successResponse("Lookup deleted successfully")}, nil
}

// ListPpcLookups lists lookups with filtering and pagination.
func (h *lookupHandler) ListPpcLookups(ctx context.Context, req *ppcv1.ListPpcLookupsRequest) (*ppcv1.ListPpcLookupsResponse, error) {
	result, err := h.svc.List(ctx, lookupapp.ListQuery{
		Page:     int(req.GetPage()),
		PageSize: int(req.GetPageSize()),
		Category: req.GetCategory(),
		Search:   req.GetSearch(),
		IsActive: activeFilterToBool(req.GetActiveFilter()),
	})
	if err != nil {
		return &ppcv1.ListPpcLookupsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.PpcLookup, len(result.Items))
	for i, entity := range result.Items {
		items[i] = lookupToProto(entity)
	}
	return &ppcv1.ListPpcLookupsResponse{
		Base: successResponse("Lookups retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

func lookupToProto(e *lookupdomain.Lookup) *ppcv1.PpcLookup {
	proto := &ppcv1.PpcLookup{
		LookupId:  e.ID(),
		Category:  e.Category(),
		Code:      e.Code(),
		Label:     e.Label(),
		SortOrder: e.SortOrder(),
		IsActive:  e.IsActive(),
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
