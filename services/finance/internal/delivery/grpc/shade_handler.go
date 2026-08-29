// Package grpc provides gRPC server implementation for the finance service.
package grpc

import (
	"context"
	"errors"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appshade "github.com/mutugading/goapps-backend/services/finance/internal/application/shade"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// ShadeHandler implements financev1.ShadeServiceServer.
type ShadeHandler struct {
	financev1.UnimplementedShadeServiceServer
	createHandler *appshade.CreateHandler
	getHandler    *appshade.GetHandler
	listHandler   *appshade.ListHandler
	updateHandler *appshade.UpdateHandler
	syncHandler   *appshade.SyncHandler
	validation    *ValidationHelper
}

// NewShadeHandler constructs a ShadeHandler. syncHandler's oracle source may be
// nil (Oracle unconfigured); SyncShades then reports shade.ErrSyncNotConfigured
// through the normal error-response path instead of crashing.
func NewShadeHandler(
	createHandler *appshade.CreateHandler,
	getHandler *appshade.GetHandler,
	listHandler *appshade.ListHandler,
	updateHandler *appshade.UpdateHandler,
	syncHandler *appshade.SyncHandler,
) (*ShadeHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &ShadeHandler{
		createHandler: createHandler,
		getHandler:    getHandler,
		listHandler:   listHandler,
		updateHandler: updateHandler,
		syncHandler:   syncHandler,
		validation:    v,
	}, nil
}

// CreateShade creates a hand-authored (MANUAL) shade.
func (h *ShadeHandler) CreateShade(ctx context.Context, req *financev1.CreateShadeRequest) (*financev1.CreateShadeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.CreateShadeResponse{Base: baseResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appshade.CreateCommand{
		Code:      req.GetShadeCode(),
		Name:      req.GetShadeName(),
		ShortName: optionalString(req.GetShadeShortName()),
		CreatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.CreateShadeResponse{Base: shadeErrToBase(err)}, nil
	}

	return &financev1.CreateShadeResponse{
		Base: successResponse("Shade created successfully"),
		Data: shadeEntityToProto(entity),
	}, nil
}

// GetShade retrieves a Shade by id.
func (h *ShadeHandler) GetShade(ctx context.Context, req *financev1.GetShadeRequest) (*financev1.GetShadeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.GetShadeResponse{Base: baseResp}, nil
	}

	entity, err := h.getHandler.HandleByID(ctx, req.GetShadeId())
	if err != nil {
		return &financev1.GetShadeResponse{Base: shadeErrToBase(err)}, nil
	}

	return &financev1.GetShadeResponse{
		Base: successResponse("Shade retrieved successfully"),
		Data: shadeEntityToProto(entity),
	}, nil
}

// UpdateShade updates a Shade's name, short name, and/or active status.
// Passing is_active=false here is also the manual deactivate path.
func (h *ShadeHandler) UpdateShade(ctx context.Context, req *financev1.UpdateShadeRequest) (*financev1.UpdateShadeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateShadeResponse{Base: baseResp}, nil
	}

	entity, err := h.updateHandler.Handle(ctx, appshade.UpdateCommand{
		ID:        req.GetShadeId(),
		Name:      req.ShadeName,
		ShortName: req.ShadeShortName,
		IsActive:  req.IsActive,
		UpdatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.UpdateShadeResponse{Base: shadeErrToBase(err)}, nil
	}

	return &financev1.UpdateShadeResponse{
		Base: successResponse("Shade updated successfully"),
		Data: shadeEntityToProto(entity),
	}, nil
}

// DeactivateShade soft-deactivates a Shade (ces_is_active = false). It is a
// thin convenience wrapper over UpdateShade(is_active=false) — not a delete.
func (h *ShadeHandler) DeactivateShade(ctx context.Context, req *financev1.DeactivateShadeRequest) (*financev1.DeactivateShadeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.DeactivateShadeResponse{Base: baseResp}, nil
	}

	inactive := false
	entity, err := h.updateHandler.Handle(ctx, appshade.UpdateCommand{
		ID:        req.GetShadeId(),
		IsActive:  &inactive,
		UpdatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.DeactivateShadeResponse{Base: shadeErrToBase(err)}, nil
	}

	return &financev1.DeactivateShadeResponse{
		Base: successResponse("Shade deactivated successfully"),
		Data: shadeEntityToProto(entity),
	}, nil
}

// ListShades lists shades with search, filter, and pagination.
func (h *ShadeHandler) ListShades(ctx context.Context, req *financev1.ListShadesRequest) (*financev1.ListShadesResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListShadesResponse{Base: baseResp}, nil
	}

	page := int(req.GetPage())
	if page == 0 {
		page = 1
	}
	pageSize := int(req.GetPageSize())
	if pageSize == 0 {
		pageSize = 10
	}

	query := appshade.ListQuery{
		Page:         page,
		PageSize:     pageSize,
		Search:       req.GetSearch(),
		SourceFilter: req.GetSourceFilter(),
		SortBy:       req.GetSortBy(),
		SortOrder:    req.GetSortOrder(),
	}

	switch req.GetActiveFilter() {
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
		return &financev1.ListShadesResponse{Base: shadeErrToBase(err)}, nil
	}

	items := make([]*financev1.Shade, len(result.Shades))
	for i, e := range result.Shades {
		items[i] = shadeEntityToProto(e)
	}

	return &financev1.ListShadesResponse{
		Base: successResponse("Shades retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// SyncShades pulls the latest shade master from Oracle and upserts it into
// cost_erp_shade. Returns shade.ErrSyncNotConfigured (mapped to a normal error
// response, not a panic/crash) when no Oracle source was wired at startup —
// see NewShadeHandler and the Oracle-optional wiring in cmd/server/main.go.
func (h *ShadeHandler) SyncShades(ctx context.Context, _ *financev1.SyncShadesRequest) (*financev1.SyncShadesResponse, error) {
	result, err := h.syncHandler.Execute(ctx)
	if err != nil {
		return &financev1.SyncShadesResponse{Base: shadeErrToBase(err)}, nil
	}

	return &financev1.SyncShadesResponse{
		Base:       successResponse("Shade sync completed"),
		TotalRows:  safeIntToInt32(result.TotalRows),
		Inserted:   safeIntToInt32(result.Inserted),
		Updated:    safeIntToInt32(result.Updated),
		Skipped:    safeIntToInt32(result.Skipped),
		DurationMs: result.Duration.Milliseconds(),
	}, nil
}

// shadeErrToBase maps a shade domain error to a BaseResponse. Sync-not-configured
// is reported as a normal (non-5xx-worthy) client-facing conflict, not a crash.
func shadeErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, shade.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, shade.ErrAlreadyExists):
		return ConflictResponse(err.Error())
	case errors.Is(err, shade.ErrSyncNotConfigured):
		return ErrorResponse("409", err.Error())
	default:
		return domainErrorToBaseResponse(err)
	}
}

// shadeEntityToProto converts a domain Shade entity to its proto representation.
func shadeEntityToProto(e *shade.Shade) *financev1.Shade {
	p := &financev1.Shade{
		ShadeId:     e.ID(),
		ShadeCode:   e.Code(),
		ShadeName:   e.Name(),
		ShadeSource: e.Source(),
		IsActive:    e.IsActive(),
		UsageCount:  e.UsageCount(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.ShortName() != nil {
		p.ShadeShortName = *e.ShortName()
	}
	if e.SourceCreatedAt() != nil {
		p.SourceCreatedAt = e.SourceCreatedAt().Format(time.RFC3339)
	}
	if e.SourceUpdatedAt() != nil {
		p.SourceUpdatedAt = e.SourceUpdatedAt().Format(time.RFC3339)
	}
	if e.SourceCreatedBy() != nil {
		p.SourceCreatedBy = *e.SourceCreatedBy()
	}
	if e.SourceUpdatedBy() != nil {
		p.SourceUpdatedBy = *e.SourceUpdatedBy()
	}
	if e.SyncedAt() != nil {
		p.SyncedAt = e.SyncedAt().Format(time.RFC3339)
	}
	if e.UpdatedAt() != nil {
		p.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		p.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return p
}

// optionalString converts an empty string to nil so "not provided" and
// "explicitly cleared" are distinguished the same way the domain layer expects.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
