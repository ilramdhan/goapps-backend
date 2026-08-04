// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// MBHeadHandler implements financev1.MBHeadServiceServer.
type MBHeadHandler struct {
	financev1.UnimplementedMBHeadServiceServer
	createHandler    *appmbhead.CreateHandler
	getHandler       *appmbhead.GetHandler
	listHandler      *appmbhead.ListHandler
	updateHandler    *appmbhead.UpdateHandler
	deleteHandler    *appmbhead.DeleteHandler
	submitHandler    *appmbhead.SubmitHandler
	approveHandler   *appmbhead.ApproveHandler
	validateHandler  *appmbhead.ValidateHandler
	unApproveHandler *appmbhead.UnApproveHandler
	revokeHandler    *appmbhead.RevokeHandler
	refreezeHandler  *appmbhead.RefreezeHandler
	exportHandler    *appmbhead.ExportHandler
	importHandler    *appmbhead.ImportHandler
	templateHandler  *appmbhead.TemplateHandler
	validation       *ValidationHelper
}

// NewMBHeadHandler constructs an MBHeadHandler.
func NewMBHeadHandler(repo mbhead.Repository, paramRepo mbparam.Repository) (*MBHeadHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MBHeadHandler{
		createHandler:    appmbhead.NewCreateHandler(repo),
		getHandler:       appmbhead.NewGetHandler(repo),
		listHandler:      appmbhead.NewListHandler(repo),
		updateHandler:    appmbhead.NewUpdateHandler(repo),
		deleteHandler:    appmbhead.NewDeleteHandler(repo),
		submitHandler:    appmbhead.NewSubmitHandler(repo),
		approveHandler:   appmbhead.NewApproveHandler(repo),
		validateHandler:  appmbhead.NewValidateHandler(repo, paramRepo),
		unApproveHandler: appmbhead.NewUnApproveHandler(repo),
		revokeHandler:    appmbhead.NewRevokeHandler(repo),
		refreezeHandler:  appmbhead.NewRefreezeHandler(repo, paramRepo),
		exportHandler:    appmbhead.NewExportHandler(repo),
		importHandler:    appmbhead.NewImportHandler(repo),
		templateHandler:  appmbhead.NewTemplateHandler(),
		validation:       v,
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

	machineID, badResp := parseOptionalMachineID(req.MbhMachineId)
	if badResp != nil {
		RecordMBHeadOperation("create", false)
		return &financev1.CreateMBHeadResponse{Base: badResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appmbhead.CreateCommand{
		MBCosting:       req.MbhMbCosting,
		OracleSysID:     req.MbhOracleSysId,
		MgtName:         req.MbhMgtName,
		Denier:          req.MbhDenier,
		Filament:        filament,
		Dozing:          req.MbhDozing,
		MBHCheckStatus:  req.MbhCheckStatus,
		MBHStatus:       req.MbhStatus,
		MBHLdrPrsn:      req.MbhLdrPrsn,
		MBHFinalProduct: req.MbhFinalProduct,
		MBHCode:         req.MbhCode,
		CreatedBy:       getUserFromContext(ctx),
		IsBoughtout:     req.MbhIsBoughtout,
		DevCode:         req.GetMbhDevCode(),
		ShadeCode:       req.GetMbhShadeCode(),
		ShadeName:       req.GetMbhShadeName(),
		CrossSection:    req.GetMbhCrossSection(),
		LustureCode:     req.GetMbhLustureCode(),
		MachineID:       machineID,
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

	machineID, badResp := parseOptionalMachineID(req.MbhMachineId)
	if badResp != nil {
		RecordMBHeadOperation("update", false)
		return &financev1.UpdateMBHeadResponse{Base: badResp}, nil
	}

	entity, err := h.updateHandler.Handle(ctx, appmbhead.UpdateCommand{
		ID:              id,
		MBCosting:       req.MbhMbCosting,
		MgtName:         req.MbhMgtName,
		Denier:          req.MbhDenier,
		Filament:        filament,
		Dozing:          req.MbhDozing,
		MBHCheckStatus:  req.MbhCheckStatus,
		MBHStatus:       req.MbhStatus,
		MBHLdrPrsn:      req.MbhLdrPrsn,
		MBHFinalProduct: req.MbhFinalProduct,
		MBHCode:         req.MbhCode,
		IsActive:        req.MbhIsActive,
		DevCode:         req.MbhDevCode,
		ShadeCode:       req.MbhShadeCode,
		ShadeName:       req.MbhShadeName,
		CrossSection:    req.MbhCrossSection,
		LustureCode:     req.MbhLustureCode,
		MachineID:       machineID,
		UpdatedBy:       getUserFromContext(ctx),
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

// SubmitMBHead submits an MB Head for approval (DRAFT -> SUBMITTED).
func (h *MBHeadHandler) SubmitMBHead(ctx context.Context, req *financev1.SubmitMBHeadRequest) (*financev1.SubmitMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("submit", false)
		return &financev1.SubmitMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("submit", false)
		return &financev1.SubmitMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.submitHandler.Handle(ctx, appmbhead.SubmitCommand{MbhID: id, ActorUserID: getUserFromContext(ctx)})
	if err != nil {
		RecordMBHeadOperation("submit", false)
		return &financev1.SubmitMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("submit", true)
	return &financev1.SubmitMBHeadResponse{
		Base: successResponse("MB head submitted successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// ApproveMBHead approves a submitted MB Head (SUBMITTED -> APPROVED).
func (h *MBHeadHandler) ApproveMBHead(ctx context.Context, req *financev1.ApproveMBHeadRequest) (*financev1.ApproveMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("approve", false)
		return &financev1.ApproveMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("approve", false)
		return &financev1.ApproveMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.approveHandler.Handle(ctx, appmbhead.ApproveCommand{MbhID: id, ActorUserID: getUserFromContext(ctx)})
	if err != nil {
		RecordMBHeadOperation("approve", false)
		return &financev1.ApproveMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("approve", true)
	return &financev1.ApproveMBHeadResponse{
		Base: successResponse("MB head approved successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// ValidateMBHead validates an approved MB Head, freezing its param snapshot and auto-generating
// its product (APPROVED -> VALIDATED, or DRAFT -> VALIDATED for boughtout MBs).
func (h *MBHeadHandler) ValidateMBHead(ctx context.Context, req *financev1.ValidateMBHeadRequest) (*financev1.ValidateMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("validate", false)
		return &financev1.ValidateMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("validate", false)
		return &financev1.ValidateMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.validateHandler.Handle(ctx, appmbhead.ValidateCommand{MbhID: id, ActorUserID: getUserFromContext(ctx)})
	if err != nil {
		RecordMBHeadOperation("validate", false)
		return &financev1.ValidateMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("validate", true)
	return &financev1.ValidateMBHeadResponse{
		Base: successResponse("MB head validated successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// UnApproveMBHead reverts an approved MB Head back to a prior state (APPROVED -> UN_APPROVED).
func (h *MBHeadHandler) UnApproveMBHead(ctx context.Context, req *financev1.UnApproveMBHeadRequest) (*financev1.UnApproveMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("unapprove", false)
		return &financev1.UnApproveMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("unapprove", false)
		return &financev1.UnApproveMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.unApproveHandler.Handle(ctx, appmbhead.UnApproveCommand{MbhID: id, Reason: req.Reason, ActorUserID: getUserFromContext(ctx)})
	if err != nil {
		RecordMBHeadOperation("unapprove", false)
		return &financev1.UnApproveMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("unapprove", true)
	return &financev1.UnApproveMBHeadResponse{
		Base: successResponse("MB head un-approved successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// RevokeMBHead revokes an MB Head, moving it to the terminal REVOKED state.
func (h *MBHeadHandler) RevokeMBHead(ctx context.Context, req *financev1.RevokeMBHeadRequest) (*financev1.RevokeMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("revoke", false)
		return &financev1.RevokeMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("revoke", false)
		return &financev1.RevokeMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.revokeHandler.Handle(ctx, appmbhead.RevokeCommand{MbhID: id, Reason: req.Reason, ActorUserID: getUserFromContext(ctx)})
	if err != nil {
		RecordMBHeadOperation("revoke", false)
		return &financev1.RevokeMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("revoke", true)
	return &financev1.RevokeMBHeadResponse{
		Base: successResponse("MB head revoked successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
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

// ExportMBHeads exports MB Heads to an Excel file.
func (h *MBHeadHandler) ExportMBHeads(ctx context.Context, req *financev1.ExportMBHeadsRequest) (*financev1.ExportMBHeadsResponse, error) {
	query := appmbhead.ExportQuery{}

	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		active := true
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		active := false
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
		// Export all - no filter
	}

	result, err := h.exportHandler.Handle(ctx, query)
	if err != nil {
		RecordMBHeadOperation("export", false)
		return &financev1.ExportMBHeadsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("export", true)

	return &financev1.ExportMBHeadsResponse{
		Base:        successResponse("MB Heads exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportMBHeads imports MB Heads from an Excel file.
func (h *MBHeadHandler) ImportMBHeads(ctx context.Context, req *financev1.ImportMBHeadsRequest) (*financev1.ImportMBHeadsResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("import", false)
		return &financev1.ImportMBHeadsResponse{Base: baseResp}, nil
	}

	cmd := appmbhead.ImportCommand{
		FileContent:     req.FileContent,
		FileName:        req.FileName,
		DuplicateAction: req.DuplicateAction,
		CreatedBy:       getUserFromContext(ctx),
	}

	result, err := h.importHandler.Handle(ctx, cmd)
	if err != nil {
		RecordMBHeadOperation("import", false)
		return &financev1.ImportMBHeadsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("import", true)

	importErrors := make([]*financev1.ImportError, len(result.Errors))
	for i, e := range result.Errors {
		importErrors[i] = &financev1.ImportError{
			RowNumber: e.RowNumber,
			Field:     e.Field,
			Message:   e.Message,
		}
	}

	return &financev1.ImportMBHeadsResponse{
		Base:         successResponse("Import completed"),
		SuccessCount: result.SuccessCount,
		SkippedCount: result.SkippedCount,
		FailedCount:  result.FailedCount,
		Errors:       importErrors,
	}, nil
}

// DownloadMBHeadTemplate downloads the Excel import template for MB Heads.
func (h *MBHeadHandler) DownloadMBHeadTemplate(_ context.Context, _ *financev1.DownloadMBHeadTemplateRequest) (*financev1.DownloadMBHeadTemplateResponse, error) {
	result, err := h.templateHandler.Handle()
	if err != nil {
		return &financev1.DownloadMBHeadTemplateResponse{Base: InternalErrorResponse(err.Error())}, nil
	}

	return &financev1.DownloadMBHeadTemplateResponse{
		Base:        successResponse("Template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
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
	p.MbhCheckStatus = e.MBHCheckStatus()
	p.MbhStatus = e.MBHStatus()
	p.MbhLdrPrsn = e.MBHLdrPrsn()
	p.MbhFinalProduct = e.MBHFinalProduct()
	p.MbhCode = e.MBHCode()
	p.IsBoughtout = e.IsBoughtout()
	p.DevCode = e.DevCode()
	p.ShadeCode = e.ShadeCode()
	p.ShadeName = e.ShadeName()
	p.CrossSection = e.CrossSection()
	p.LustureCode = e.LustureCode()
	p.EntryStatus = e.EntryStatus()
	p.CurrentVersion = e.CurrentVersion()
	if e.MachineFixedTotal() != nil {
		p.MachineFixedTotal = *e.MachineFixedTotal()
	}
	if e.MachineID() != nil {
		id := e.MachineID().String()
		p.MachineId = &id
	}
	p.StateReason = e.StateReason()
	p.CostProductId = e.CostProductID()
	if e.CostGeneratedAt() != nil {
		p.CostGeneratedAt = *e.CostGeneratedAt()
	}
	p.CostGeneratedBy = e.CostGeneratedBy()
	if e.ParamWaste() != nil {
		p.ParamWaste = *e.ParamWaste()
	}
	if e.ParamQualityLoss() != nil {
		p.ParamQualityLoss = *e.ParamQualityLoss()
	}
	if e.ParamEfficiency() != nil {
		p.ParamEfficiency = *e.ParamEfficiency()
	}
	if e.ParamDevExpense() != nil {
		p.ParamDevExpense = *e.ParamDevExpense()
	}
	if e.ParamPacking() != nil {
		p.ParamPacking = *e.ParamPacking()
	}
	if e.ParamMBProdPerDay() != nil {
		p.ParamMbProdPerDay = *e.ParamMBProdPerDay()
	}
	p.ParamThroughputPerHour = e.ParamThroughputPerHour()
	p.ParamNoOfProcess = e.ParamNoOfProcess()
	if e.UpdatedAt() != nil {
		p.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		p.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return p
}

// parseOptionalMachineID parses an optional *string mbh_machine_id into a *uuid.UUID.
// Returns (nil, nil) when the input is nil or empty. Returns (nil, baseResponse) when
// the input is non-empty but invalid.
func parseOptionalMachineID(raw *string) (*uuid.UUID, *commonv1.BaseResponse) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return nil, ErrorResponse("400", "invalid mbh_machine_id: "+err.Error())
	}
	return &id, nil
}
