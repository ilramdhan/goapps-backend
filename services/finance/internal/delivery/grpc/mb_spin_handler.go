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
	appmbspin "github.com/mutugading/goapps-backend/services/finance/internal/application/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// MBSpinHandler implements financev1.MBSpinServiceServer.
type MBSpinHandler struct {
	financev1.UnimplementedMBSpinServiceServer
	createHandler *appmbspin.CreateHandler
	getHandler    *appmbspin.GetHandler
	listHandler   *appmbspin.ListHandler
	updateHandler *appmbspin.UpdateHandler
	deleteHandler *appmbspin.DeleteHandler
	validation    *ValidationHelper
}

// NewMBSpinHandler constructs an MBSpinHandler.
func NewMBSpinHandler(repo mbspin.Repository) (*MBSpinHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MBSpinHandler{
		createHandler: appmbspin.NewCreateHandler(repo),
		getHandler:    appmbspin.NewGetHandler(repo),
		listHandler:   appmbspin.NewListHandler(repo),
		updateHandler: appmbspin.NewUpdateHandler(repo),
		deleteHandler: appmbspin.NewDeleteHandler(repo),
		validation:    v,
	}, nil
}

// CreateMBSpin creates a new MB spin record.
func (h *MBSpinHandler) CreateMBSpin(ctx context.Context, req *financev1.CreateMBSpinRequest) (*financev1.CreateMBSpinResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBSpinOperation("create", false)
		return &financev1.CreateMBSpinResponse{Base: baseResp}, nil
	}

	headID, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBSpinOperation("create", false)
		return &financev1.CreateMBSpinResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	var filament *int
	if req.MbsFilament != nil {
		v := int(*req.MbsFilament)
		filament = &v
	}

	entity, err := h.createHandler.Handle(ctx, appmbspin.CreateCommand{
		HeadID:      headID,
		MgtName:     req.MbsMgtName,
		OracleSysID: req.MbsOracleSysId,
		Denier:      req.MbsDenier,
		Filament:    filament,
		Dozing:      req.MbsDozing,
		MBCosting:   req.MbsMbCosting,
		CreatedBy:   getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBSpinOperation("create", false)
		return &financev1.CreateMBSpinResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBSpinOperation("create", true)
	return &financev1.CreateMBSpinResponse{
		Base: successResponse("MB spin created successfully"),
		Data: mbSpinEntityToProto(entity),
	}, nil
}

// GetMBSpin retrieves an MB spin record by ID.
func (h *MBSpinHandler) GetMBSpin(ctx context.Context, req *financev1.GetMBSpinRequest) (*financev1.GetMBSpinResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBSpinOperation("get", false)
		return &financev1.GetMBSpinResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbsId)
	if err != nil {
		RecordMBSpinOperation("get", false)
		return &financev1.GetMBSpinResponse{Base: invalidIDResponse("mbs_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.getHandler.Handle(ctx, appmbspin.GetQuery{ID: id})
	if err != nil {
		RecordMBSpinOperation("get", false)
		return &financev1.GetMBSpinResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBSpinOperation("get", true)
	return &financev1.GetMBSpinResponse{
		Base: successResponse("MB spin retrieved successfully"),
		Data: mbSpinEntityToProto(entity),
	}, nil
}

// UpdateMBSpin updates an existing MB spin record.
func (h *MBSpinHandler) UpdateMBSpin(ctx context.Context, req *financev1.UpdateMBSpinRequest) (*financev1.UpdateMBSpinResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBSpinOperation("update", false)
		return &financev1.UpdateMBSpinResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbsId)
	if err != nil {
		RecordMBSpinOperation("update", false)
		return &financev1.UpdateMBSpinResponse{Base: invalidIDResponse("mbs_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	var filament *int
	if req.MbsFilament != nil {
		v := int(*req.MbsFilament)
		filament = &v
	}

	entity, err := h.updateHandler.Handle(ctx, appmbspin.UpdateCommand{
		ID:        id,
		MgtName:   req.MbsMgtName,
		MBCosting: req.MbsMbCosting,
		Denier:    req.MbsDenier,
		Filament:  filament,
		Dozing:    req.MbsDozing,
		IsActive:  req.MbsIsActive,
		UpdatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBSpinOperation("update", false)
		return &financev1.UpdateMBSpinResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBSpinOperation("update", true)
	return &financev1.UpdateMBSpinResponse{
		Base: successResponse("MB spin updated successfully"),
		Data: mbSpinEntityToProto(entity),
	}, nil
}

// DeleteMBSpin soft-deletes an MB spin record.
func (h *MBSpinHandler) DeleteMBSpin(ctx context.Context, req *financev1.DeleteMBSpinRequest) (*financev1.DeleteMBSpinResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBSpinOperation("delete", false)
		return &financev1.DeleteMBSpinResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbsId)
	if err != nil {
		RecordMBSpinOperation("delete", false)
		return &financev1.DeleteMBSpinResponse{Base: invalidIDResponse("mbs_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	if err := h.deleteHandler.Handle(ctx, appmbspin.DeleteCommand{ID: id, DeletedBy: getUserFromContext(ctx)}); err != nil {
		RecordMBSpinOperation("delete", false)
		return &financev1.DeleteMBSpinResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBSpinOperation("delete", true)
	return &financev1.DeleteMBSpinResponse{Base: successResponse("MB spin deleted successfully")}, nil
}

// ListMBSpins lists MB spin records for a given head with search, filter, and pagination.
func (h *MBSpinHandler) ListMBSpins(ctx context.Context, req *financev1.ListMBSpinsRequest) (*financev1.ListMBSpinsResponse, error) {
	headID, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBSpinOperation("list", false)
		return &financev1.ListMBSpinsResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := appmbspin.ListQuery{
		HeadID:    headID,
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
		RecordMBSpinOperation("list", false)
		return &financev1.ListMBSpinsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBSpinOperation("list", true)

	items := make([]*financev1.MBSpin, len(result.Items))
	for i, e := range result.Items {
		items[i] = mbSpinEntityToProto(e)
	}

	return &financev1.ListMBSpinsResponse{
		Base: successResponse("MB spins retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportMBSpins is not yet implemented.
func (h *MBSpinHandler) ExportMBSpins(_ context.Context, _ *financev1.ExportMBSpinsRequest) (*financev1.ExportMBSpinsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ExportMBSpins not implemented")
}

// ImportMBSpins is not yet implemented.
func (h *MBSpinHandler) ImportMBSpins(_ context.Context, _ *financev1.ImportMBSpinsRequest) (*financev1.ImportMBSpinsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ImportMBSpins not implemented")
}

// DownloadMBSpinTemplate is not yet implemented.
func (h *MBSpinHandler) DownloadMBSpinTemplate(_ context.Context, _ *financev1.DownloadMBSpinTemplateRequest) (*financev1.DownloadMBSpinTemplateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DownloadMBSpinTemplate not implemented")
}

// mbSpinEntityToProto converts a domain MBSpin entity to its proto representation.
func mbSpinEntityToProto(e *mbspin.Entity) *financev1.MBSpin {
	p := &financev1.MBSpin{
		MbsId:       e.ID().String(),
		MbsMbhId:    e.HeadID().String(),
		MbsMgtName:  e.MgtName(),
		MbsIsActive: e.IsActive(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.OracleSysID() != nil {
		p.MbsOracleSysId = *e.OracleSysID()
	}
	p.MbsDenier = e.Denier()
	if e.Filament() != nil {
		v := safeconv.IntToInt32(*e.Filament())
		p.MbsFilament = &v
	}
	p.MbsDozing = e.Dozing()
	if e.MBCosting() != nil {
		p.MbsMbCosting = *e.MBCosting()
	}
	if e.UpdatedAt() != nil {
		p.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		p.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return p
}
