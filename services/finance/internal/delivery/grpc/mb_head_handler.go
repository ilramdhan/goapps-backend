// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// MBHeadHandler implements financev1.MBHeadServiceServer.
type MBHeadHandler struct {
	financev1.UnimplementedMBHeadServiceServer
	createHandler *appmbhead.CreateHandler
	getHandler    *appmbhead.GetHandler
	listHandler   *appmbhead.ListHandler
	updateHandler *appmbhead.UpdateHandler
	deleteHandler *appmbhead.DeleteHandler
	validation    *ValidationHelper
}

// NewMBHeadHandler constructs an MBHeadHandler.
func NewMBHeadHandler(repo mbhead.Repository) (*MBHeadHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MBHeadHandler{
		createHandler: appmbhead.NewCreateHandler(repo),
		getHandler:    appmbhead.NewGetHandler(repo),
		listHandler:   appmbhead.NewListHandler(repo),
		updateHandler: appmbhead.NewUpdateHandler(repo),
		deleteHandler: appmbhead.NewDeleteHandler(repo),
		validation:    v,
	}, nil
}

// CreateMBHead creates a new MB head record.
func (h *MBHeadHandler) CreateMBHead(ctx context.Context, req *financev1.CreateMBHeadRequest) (*financev1.CreateMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("create", false)
		return &financev1.CreateMBHeadResponse{Base: baseResp}, nil
	}

	var filament *int
	if req.MbhFilament != nil {
		v := int(*req.MbhFilament)
		filament = &v
	}

	entity, err := h.createHandler.Handle(ctx, appmbhead.CreateCommand{
		MBCosting:   req.MbhMbCosting,
		OracleSysID: req.MbhOracleSysId,
		MgtName:     req.MbhMgtName,
		Denier:      req.MbhDenier,
		Filament:    filament,
		Dozing:      req.MbhDozing,
		CreatedBy:   getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBHeadOperation("create", false)
		return &financev1.CreateMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("create", true)
	return &financev1.CreateMBHeadResponse{
		Base: successResponse("MB head created successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// GetMBHead retrieves an MB head record by ID.
func (h *MBHeadHandler) GetMBHead(ctx context.Context, req *financev1.GetMBHeadRequest) (*financev1.GetMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("get", false)
		return &financev1.GetMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("get", false)
		return &financev1.GetMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.getHandler.Handle(ctx, appmbhead.GetQuery{ID: id})
	if err != nil {
		RecordMBHeadOperation("get", false)
		return &financev1.GetMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("get", true)
	return &financev1.GetMBHeadResponse{
		Base: successResponse("MB head retrieved successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// UpdateMBHead updates an existing MB head record.
func (h *MBHeadHandler) UpdateMBHead(ctx context.Context, req *financev1.UpdateMBHeadRequest) (*financev1.UpdateMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("update", false)
		return &financev1.UpdateMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("update", false)
		return &financev1.UpdateMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	var filament *int
	if req.MbhFilament != nil {
		v := int(*req.MbhFilament)
		filament = &v
	}

	entity, err := h.updateHandler.Handle(ctx, appmbhead.UpdateCommand{
		ID:        id,
		MBCosting: req.MbhMbCosting,
		MgtName:   req.MbhMgtName,
		Denier:    req.MbhDenier,
		Filament:  filament,
		Dozing:    req.MbhDozing,
		IsActive:  req.MbhIsActive,
		UpdatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBHeadOperation("update", false)
		return &financev1.UpdateMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("update", true)
	return &financev1.UpdateMBHeadResponse{
		Base: successResponse("MB head updated successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// DeleteMBHead soft-deletes an MB head record.
func (h *MBHeadHandler) DeleteMBHead(ctx context.Context, req *financev1.DeleteMBHeadRequest) (*financev1.DeleteMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("delete", false)
		return &financev1.DeleteMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("delete", false)
		return &financev1.DeleteMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	if err := h.deleteHandler.Handle(ctx, appmbhead.DeleteCommand{ID: id, DeletedBy: getUserFromContext(ctx)}); err != nil {
		RecordMBHeadOperation("delete", false)
		return &financev1.DeleteMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("delete", true)
	return &financev1.DeleteMBHeadResponse{Base: successResponse("MB head deleted successfully")}, nil
}

// ListMBHeads lists MB head records with search, filter, and pagination.
func (h *MBHeadHandler) ListMBHeads(ctx context.Context, req *financev1.ListMBHeadsRequest) (*financev1.ListMBHeadsResponse, error) {
	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := appmbhead.ListQuery{
		Page:      page,
		PageSize:  pageSize,
		Search:    req.Search,
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
		RecordMBHeadOperation("list", false)
		return &financev1.ListMBHeadsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("list", true)

	items := make([]*financev1.MBHead, len(result.Items))
	for i, e := range result.Items {
		items[i] = mbHeadEntityToProto(e)
	}

	return &financev1.ListMBHeadsResponse{
		Base: successResponse("MB heads retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportMBHeads is not yet implemented.
func (h *MBHeadHandler) ExportMBHeads(_ context.Context, _ *financev1.ExportMBHeadsRequest) (*financev1.ExportMBHeadsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ExportMBHeads not implemented")
}

// ImportMBHeads is not yet implemented.
func (h *MBHeadHandler) ImportMBHeads(_ context.Context, _ *financev1.ImportMBHeadsRequest) (*financev1.ImportMBHeadsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ImportMBHeads not implemented")
}

// DownloadMBHeadTemplate is not yet implemented.
func (h *MBHeadHandler) DownloadMBHeadTemplate(_ context.Context, _ *financev1.DownloadMBHeadTemplateRequest) (*financev1.DownloadMBHeadTemplateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DownloadMBHeadTemplate not implemented")
}

// mbHeadEntityToProto converts a domain MBHead entity to its proto representation.
func mbHeadEntityToProto(e *mbhead.Entity) *financev1.MBHead {
	p := &financev1.MBHead{
		MbhId:        e.ID().String(),
		MbhMbCosting: e.MBCosting(),
		MbhIsActive:  e.IsActive(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.OracleSysID() != nil {
		p.MbhOracleSysId = *e.OracleSysID()
	}
	if e.MgtName() != nil {
		p.MbhMgtName = *e.MgtName()
	}
	p.MbhDenier = e.Denier()
	if e.Filament() != nil {
		v := safeconv.IntToInt32(*e.Filament())
		p.MbhFilament = &v
	}
	p.MbhDozing = e.Dozing()
	if e.UpdatedAt() != nil {
		p.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		p.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return p
}
