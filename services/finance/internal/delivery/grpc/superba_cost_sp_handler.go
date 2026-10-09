package grpc

import (
	"context"
	"errors"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appsp "github.com/mutugading/goapps-backend/services/finance/internal/application/superbacostsp"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/superbacostsp"
)

// SuperbaCostSpHandler implements financev1.SuperbaCostSpServiceServer.
type SuperbaCostSpHandler struct {
	financev1.UnimplementedSuperbaCostSpServiceServer
	createHandler *appsp.CreateHandler
	getHandler    *appsp.GetHandler
	listHandler   *appsp.ListHandler
	updateHandler *appsp.UpdateHandler
	deleteHandler *appsp.DeleteHandler
	syncHandler   *appsp.SyncHandler
	validation    *ValidationHelper
}

// NewSuperbaCostSpHandler constructs the handler. The sync handler's source may
// be nil; SyncSuperbaCostSps then reports ErrSyncNotConfigured as a normal
// error response (the Oracle source arrives in a later phase).
func NewSuperbaCostSpHandler(
	createHandler *appsp.CreateHandler,
	getHandler *appsp.GetHandler,
	listHandler *appsp.ListHandler,
	updateHandler *appsp.UpdateHandler,
	deleteHandler *appsp.DeleteHandler,
	syncHandler *appsp.SyncHandler,
) (*SuperbaCostSpHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &SuperbaCostSpHandler{
		createHandler: createHandler, getHandler: getHandler, listHandler: listHandler,
		updateHandler: updateHandler, deleteHandler: deleteHandler, syncHandler: syncHandler,
		validation: v,
	}, nil
}

// CreateSuperbaCostSp creates a MANUAL row.
func (h *SuperbaCostSpHandler) CreateSuperbaCostSp(ctx context.Context, req *financev1.CreateSuperbaCostSpRequest) (*financev1.CreateSuperbaCostSpResponse, error) {
	if base := h.validation.ValidateRequest(req); base != nil {
		return &financev1.CreateSuperbaCostSpResponse{Base: base}, nil
	}
	entity, err := h.createHandler.Handle(ctx, appsp.CreateCommand{
		LegacySysID: req.GetLegacySysId(),
		ShadeCode:   req.GetShadeCode(),
		ColourName:  optionalString(req.GetColourName()),
		OldValue:    req.GetOldValue(),
		NewValue:    req.NewValue,
		IsActive:    req.GetIsActive(),
		CreatedBy:   getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.CreateSuperbaCostSpResponse{Base: superbaCostSpErrToBase(err)}, nil
	}
	return &financev1.CreateSuperbaCostSpResponse{
		Base: successResponse("Superba cost SP created successfully"),
		Data: superbaCostSpToProto(entity),
	}, nil
}

// GetSuperbaCostSp retrieves a row by id.
func (h *SuperbaCostSpHandler) GetSuperbaCostSp(ctx context.Context, req *financev1.GetSuperbaCostSpRequest) (*financev1.GetSuperbaCostSpResponse, error) {
	if base := h.validation.ValidateRequest(req); base != nil {
		return &financev1.GetSuperbaCostSpResponse{Base: base}, nil
	}
	entity, err := h.getHandler.Handle(ctx, req.GetId())
	if err != nil {
		return &financev1.GetSuperbaCostSpResponse{Base: superbaCostSpErrToBase(err)}, nil
	}
	return &financev1.GetSuperbaCostSpResponse{
		Base: successResponse("Superba cost SP retrieved successfully"),
		Data: superbaCostSpToProto(entity),
	}, nil
}

// UpdateSuperbaCostSp updates a row.
func (h *SuperbaCostSpHandler) UpdateSuperbaCostSp(ctx context.Context, req *financev1.UpdateSuperbaCostSpRequest) (*financev1.UpdateSuperbaCostSpResponse, error) {
	if base := h.validation.ValidateRequest(req); base != nil {
		return &financev1.UpdateSuperbaCostSpResponse{Base: base}, nil
	}
	entity, err := h.updateHandler.Handle(ctx, appsp.UpdateCommand{
		ID: req.GetId(), ShadeCode: req.ShadeCode, ColourName: req.ColourName,
		OldValue: req.OldValue, NewValue: req.NewValue, IsActive: req.IsActive,
		UpdatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.UpdateSuperbaCostSpResponse{Base: superbaCostSpErrToBase(err)}, nil
	}
	return &financev1.UpdateSuperbaCostSpResponse{
		Base: successResponse("Superba cost SP updated successfully"),
		Data: superbaCostSpToProto(entity),
	}, nil
}

// DeleteSuperbaCostSp soft-deletes a row.
func (h *SuperbaCostSpHandler) DeleteSuperbaCostSp(ctx context.Context, req *financev1.DeleteSuperbaCostSpRequest) (*financev1.DeleteSuperbaCostSpResponse, error) {
	if base := h.validation.ValidateRequest(req); base != nil {
		return &financev1.DeleteSuperbaCostSpResponse{Base: base}, nil
	}
	if err := h.deleteHandler.Handle(ctx, req.GetId(), getUserFromContext(ctx)); err != nil {
		return &financev1.DeleteSuperbaCostSpResponse{Base: superbaCostSpErrToBase(err)}, nil
	}
	return &financev1.DeleteSuperbaCostSpResponse{Base: successResponse("Superba cost SP deleted successfully")}, nil
}

// ListSuperbaCostSps lists rows.
func (h *SuperbaCostSpHandler) ListSuperbaCostSps(ctx context.Context, req *financev1.ListSuperbaCostSpsRequest) (*financev1.ListSuperbaCostSpsResponse, error) {
	if base := h.validation.ValidateRequest(req); base != nil {
		return &financev1.ListSuperbaCostSpsResponse{Base: base}, nil
	}
	query := appsp.ListQuery{
		Page: int(req.GetPage()), PageSize: int(req.GetPageSize()), Search: req.GetSearch(),
		SourceFilter: req.GetSourceFilter(), SortBy: req.GetSortBy(), SortOrder: req.GetSortOrder(),
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
		return &financev1.ListSuperbaCostSpsResponse{Base: superbaCostSpErrToBase(err)}, nil
	}
	items := make([]*financev1.SuperbaCostSp, len(result.Items))
	for i, e := range result.Items {
		items[i] = superbaCostSpToProto(e)
	}
	resp := &financev1.ListSuperbaCostSpsResponse{
		Base: successResponse("Superba cost SPs retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage, PageSize: result.PageSize,
			TotalItems: result.TotalItems, TotalPages: result.TotalPages,
		},
	}
	if result.LastSyncedAt != nil {
		resp.LastSyncedAt = result.LastSyncedAt.Format(time.RFC3339)
	}
	return resp, nil
}

// SyncSuperbaCostSps pulls the master from Oracle. Until the Oracle source is
// wired (phase P4) this returns a normal "not configured" error response.
func (h *SuperbaCostSpHandler) SyncSuperbaCostSps(ctx context.Context, _ *financev1.SyncSuperbaCostSpsRequest) (*financev1.SyncSuperbaCostSpsResponse, error) {
	result, err := h.syncHandler.Execute(ctx)
	if err != nil {
		return &financev1.SyncSuperbaCostSpsResponse{Base: superbaCostSpErrToBase(err)}, nil
	}
	return &financev1.SyncSuperbaCostSpsResponse{
		Base:       successResponse("Superba cost SP sync completed"),
		TotalRows:  safeIntToInt32(result.TotalRows),
		Inserted:   safeIntToInt32(result.Inserted),
		Updated:    safeIntToInt32(result.Updated),
		Unchanged:  safeIntToInt32(result.Unchanged),
		Skipped:    safeIntToInt32(result.Skipped),
		DurationMs: result.Duration.Milliseconds(),
	}, nil
}

func superbaCostSpErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, domain.ErrDuplicateLegacySysID):
		return ConflictResponse(err.Error())
	case errors.Is(err, domain.ErrSyncNotConfigured):
		return ErrorResponse("409", err.Error())
	case errors.Is(err, domain.ErrInvalidLegacySysID), errors.Is(err, domain.ErrEmptyShadeCode),
		errors.Is(err, domain.ErrShadeCodeTooLong), errors.Is(err, domain.ErrColourNameTooLong),
		errors.Is(err, domain.ErrNegativeValue):
		return ErrorResponse("400", err.Error())
	default:
		return domainErrorToBaseResponse(err)
	}
}

func superbaCostSpToProto(e *domain.Entry) *financev1.SuperbaCostSp {
	p := &financev1.SuperbaCostSp{
		Id: e.ID(), LegacySysId: e.LegacySysID(), ShadeCode: e.ShadeCode(),
		OldValue: e.OldValue(), NewValue: e.NewValue(), Source: e.Source(),
		IsActive: e.IsActive(), Effective: e.Effective(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.ColourName() != nil {
		p.ColourName = *e.ColourName()
	}
	if e.UpdatedAt() != nil {
		p.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		p.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return p
}
