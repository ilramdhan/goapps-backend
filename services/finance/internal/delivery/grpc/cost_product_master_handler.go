package grpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costbulkimport"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
)

// CostProductMasterHandler implements financev1.CostProductMasterServiceServer.
type CostProductMasterHandler struct {
	financev1.UnimplementedCostProductMasterServiceServer
	createHandler     *app.CreateHandler
	getHandler        *app.GetHandler
	updateHandler     *app.UpdateHandler
	linkErpHandler    *app.LinkErpHandler
	deactivateHandler *app.DeactivateHandler
	listHandler       *app.ListHandler
	exportHandler     *app.ExportHandler
	templateHandler   *costbulkimport.TemplateHandler
	jobRepo           costimportjob.Repository
	storageSvc        storage.Service
	importPublisher   *rabbitmq.JobPublisherAdapter
	validation        *ValidationHelper
	auditRepo         costauditlog.Repository // optional; nil means no audit
}

// NewCostProductMasterHandler constructs the handler.
func NewCostProductMasterHandler(repo domain.Repository) (*CostProductMasterHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &CostProductMasterHandler{
		createHandler:     app.NewCreateHandler(repo),
		getHandler:        app.NewGetHandler(repo),
		updateHandler:     app.NewUpdateHandler(repo),
		linkErpHandler:    app.NewLinkErpHandler(repo),
		deactivateHandler: app.NewDeactivateHandler(repo),
		listHandler:       app.NewListHandler(repo),
		exportHandler:     app.NewExportHandler(repo),
		templateHandler:   costbulkimport.NewTemplateHandler(),
		validation:        v,
	}, nil
}

// WithImportSupport wires the job repo, storage, and publisher needed for async CPM imports.
func (h *CostProductMasterHandler) WithImportSupport(
	jobRepo costimportjob.Repository,
	storageSvc storage.Service,
	importPublisher *rabbitmq.JobPublisherAdapter,
) {
	h.jobRepo = jobRepo
	h.storageSvc = storageSvc
	h.importPublisher = importPublisher
}

// WithAuditSupport wires the audit repository for emit-on-mutate behavior.
func (h *CostProductMasterHandler) WithAuditSupport(r costauditlog.Repository) {
	h.auditRepo = r
}

// emitAudit fires an audit entry and silently drops errors (audit must not fail a mutation).
func (h *CostProductMasterHandler) emitAudit(ctx context.Context, op string, entityID int64, actor string) {
	if h.auditRepo == nil {
		return
	}
	if err := h.auditRepo.Emit(ctx, costauditlog.NewInput{
		EntityType: "cost_product_master",
		EntityID:   entityID,
		Operation:  op,
		UserID:     actor,
	}); err != nil {
		_ = err // best-effort: audit never blocks a mutation
	}
}

// CreateCostProductMaster creates a new product master with auto-generated code.
func (h *CostProductMasterHandler) CreateCostProductMaster(ctx context.Context, req *financev1.CreateCostProductMasterRequest) (*financev1.CreateCostProductMasterResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.CreateCostProductMasterResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	p, err := h.createHandler.Handle(ctx, app.CreateCommand{
		ProductTypeID: req.GetProductTypeId(),
		ProductName:   req.GetProductName(),
		ShadeCode:     req.GetShadeCode(),
		GradeCode:     req.GetGradeCode(),
		Description:   req.GetDescription(),
		Flex01:        req.GetFlex_01(),
		Flex02:        req.GetFlex_02(),
		Flex03:        req.GetFlex_03(),
		ActorUserID:   actor,
	})
	if err != nil {
		return &financev1.CreateCostProductMasterResponse{Base: productMasterErrToBase(err)}, nil
	}
	h.emitAudit(ctx, costauditlog.OpInsert, p.ProductSysID(), actor)
	return &financev1.CreateCostProductMasterResponse{
		Base: successResponse("Cost product master created"),
		Data: costProductMasterToProto(p),
	}, nil
}

// GetCostProductMaster returns by sys_id.
func (h *CostProductMasterHandler) GetCostProductMaster(ctx context.Context, req *financev1.GetCostProductMasterRequest) (*financev1.GetCostProductMasterResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.GetCostProductMasterResponse{Base: baseResp}, nil
	}
	p, err := h.getHandler.Handle(ctx, app.GetQuery{ProductSysID: req.GetProductSysId()})
	if err != nil {
		return &financev1.GetCostProductMasterResponse{Base: productMasterErrToBase(err)}, nil
	}
	return &financev1.GetCostProductMasterResponse{
		Base: successResponse("OK"),
		Data: costProductMasterToProto(p),
	}, nil
}

// GetCostProductMasterByCode returns by product_code.
func (h *CostProductMasterHandler) GetCostProductMasterByCode(ctx context.Context, req *financev1.GetCostProductMasterByCodeRequest) (*financev1.GetCostProductMasterByCodeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.GetCostProductMasterByCodeResponse{Base: baseResp}, nil
	}
	p, err := h.getHandler.Handle(ctx, app.GetQuery{ProductCode: req.GetProductCode()})
	if err != nil {
		return &financev1.GetCostProductMasterByCodeResponse{Base: productMasterErrToBase(err)}, nil
	}
	return &financev1.GetCostProductMasterByCodeResponse{
		Base: successResponse("OK"),
		Data: costProductMasterToProto(p),
	}, nil
}

// UpdateCostProductMaster updates editable fields.
func (h *CostProductMasterHandler) UpdateCostProductMaster(ctx context.Context, req *financev1.UpdateCostProductMasterRequest) (*financev1.UpdateCostProductMasterResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateCostProductMasterResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	p, err := h.updateHandler.Handle(ctx, app.UpdateCommand{
		ProductSysID: req.GetProductSysId(),
		ProductName:  req.GetProductName(),
		ShadeCode:    req.GetShadeCode(),
		GradeCode:    req.GetGradeCode(),
		Description:  req.GetDescription(),
		Flex01:       req.GetFlex_01(),
		Flex02:       req.GetFlex_02(),
		Flex03:       req.GetFlex_03(),
		ActorUserID:  actor,
	})
	if err != nil {
		return &financev1.UpdateCostProductMasterResponse{Base: productMasterErrToBase(err)}, nil
	}
	h.emitAudit(ctx, costauditlog.OpUpdate, p.ProductSysID(), actor)
	return &financev1.UpdateCostProductMasterResponse{
		Base: successResponse("Cost product master updated"),
		Data: costProductMasterToProto(p),
	}, nil
}

// UpdateCostProductMasterErpLinkage sets ERP linkage.
func (h *CostProductMasterHandler) UpdateCostProductMasterErpLinkage(ctx context.Context, req *financev1.UpdateCostProductMasterErpLinkageRequest) (*financev1.UpdateCostProductMasterErpLinkageResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateCostProductMasterErpLinkageResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	p, err := h.linkErpHandler.Handle(ctx, app.LinkErpCommand{
		ProductSysID:  req.GetProductSysId(),
		ErpItemCode:   req.GetErpItemCode(),
		ErpGradeCode1: req.GetErpGradeCode_1(),
		ErpGradeCode2: req.GetErpGradeCode_2(),
		ActorUserID:   actor,
	})
	if err != nil {
		return &financev1.UpdateCostProductMasterErpLinkageResponse{Base: productMasterErrToBase(err)}, nil
	}
	h.emitAudit(ctx, costauditlog.OpAssign, p.ProductSysID(), actor)
	return &financev1.UpdateCostProductMasterErpLinkageResponse{
		Base: successResponse("ERP linkage updated"),
		Data: costProductMasterToProto(p),
	}, nil
}

// DeactivateCostProductMaster flips is_active=false.
func (h *CostProductMasterHandler) DeactivateCostProductMaster(ctx context.Context, req *financev1.DeactivateCostProductMasterRequest) (*financev1.DeactivateCostProductMasterResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.DeactivateCostProductMasterResponse{Base: baseResp}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	p, err := h.deactivateHandler.Handle(ctx, app.DeactivateCommand{
		ProductSysID: req.GetProductSysId(),
		ActorUserID:  actor,
	})
	if err != nil {
		return &financev1.DeactivateCostProductMasterResponse{Base: productMasterErrToBase(err)}, nil
	}
	h.emitAudit(ctx, costauditlog.OpStatusChange, p.ProductSysID(), actor)
	return &financev1.DeactivateCostProductMasterResponse{
		Base: successResponse("Cost product master deactivated"),
	}, nil
}

// ListCostProductMasters paginates product masters.
func (h *CostProductMasterHandler) ListCostProductMasters(ctx context.Context, req *financev1.ListCostProductMastersRequest) (*financev1.ListCostProductMastersResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostProductMastersResponse{Base: baseResp}, nil
	}
	page, pageSize := paginationFromProto(req.Pagination)
	res, err := h.listHandler.Handle(ctx, app.ListQuery{
		Search:         req.GetSearch(),
		ProductTypeID:  req.GetProductTypeId(),
		ProductTypeIDs: req.GetProductTypeIds(),
		ShadeCode:      req.GetShadeCode(),
		ActiveFilter:   req.GetActiveFilter(),
		Page:           int(page),
		PageSize:       int(pageSize),
		SortBy:         req.GetSortBy(),
		SortOrder:      req.GetSortOrder(),
	})
	if err != nil {
		return &financev1.ListCostProductMastersResponse{Base: productMasterErrToBase(err)}, nil
	}
	items := make([]*financev1.CostProductMaster, 0, len(res.Items))
	for _, p := range res.Items {
		items = append(items, costProductMasterToProto(p))
	}
	return &financev1.ListCostProductMastersResponse{
		Base:       successResponse("OK"),
		Data:       items,
		Pagination: paginationResponse(page, pageSize, res.Total),
	}, nil
}

// ExportCostProductMasters exports product masters to Excel.
func (h *CostProductMasterHandler) ExportCostProductMasters(ctx context.Context, req *financev1.ExportCostProductMastersRequest) (*financev1.ExportCostProductMastersResponse, error) {
	result, err := h.exportHandler.Handle(ctx, app.ExportQuery{
		Search:       req.GetSearch(),
		ActiveFilter: req.GetActiveFilter(),
	})
	if err != nil {
		return &financev1.ExportCostProductMastersResponse{Base: productMasterErrToBase(err)}, nil
	}
	return &financev1.ExportCostProductMastersResponse{
		Base:        successResponse("Cost product masters exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportCostProductMasters uploads file to MinIO, creates a PENDING job, and publishes to RabbitMQ.
func (h *CostProductMasterHandler) ImportCostProductMasters(ctx context.Context, req *financev1.ImportCostProductMastersRequest) (*financev1.ImportCostProductMastersResponse, error) {
	if h.jobRepo == nil {
		return &financev1.ImportCostProductMastersResponse{Base: InternalErrorResponse("import not configured")}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	actor := getUserFromContext(ctx)
	requestingUserID, _ := GetUserIDFromCtx(ctx)
	fileContent := req.GetFileContent()
	fileKey := fmt.Sprintf("imports/%s/%s_%d.xlsx", costimportjob.EntityProductMaster, actor, time.Now().UnixNano())

	if h.storageSvc != nil {
		if err := h.storageSvc.PutObject(ctx, fileKey, bytes.NewReader(fileContent), int64(len(fileContent)),
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"); err != nil {
			return &financev1.ImportCostProductMastersResponse{Base: InternalErrorResponse(fmt.Sprintf("upload file: %s", err.Error()))}, nil //nolint:nilerr // intentional BaseResponse pattern
		}
	}

	job := costimportjob.NewJob(costimportjob.EntityProductMaster, fileKey, actor, requestingUserID)
	if err := h.jobRepo.Create(ctx, job); err != nil {
		return &financev1.ImportCostProductMastersResponse{Base: InternalErrorResponse(fmt.Sprintf("create import job: %s", err.Error()))}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	if h.importPublisher != nil {
		if err := h.importPublisher.PublishImportJob(ctx, job.JobID(), costimportjob.EntityProductMaster, requestingUserID); err != nil {
			return &financev1.ImportCostProductMastersResponse{Base: InternalErrorResponse(fmt.Sprintf("publish import job: %s", err.Error()))}, nil //nolint:nilerr // intentional BaseResponse pattern
		}
	}

	return &financev1.ImportCostProductMastersResponse{
		Base:  successResponse("Product master import queued"),
		JobId: job.JobID(),
	}, nil
}

// DownloadCostProductMasterTemplate downloads the multi-sheet bulk import template.
func (h *CostProductMasterHandler) DownloadCostProductMasterTemplate(ctx context.Context, _ *financev1.DownloadCostProductMasterTemplateRequest) (*financev1.DownloadCostProductMasterTemplateResponse, error) {
	data, err := h.templateHandler.Handle(ctx)
	if err != nil {
		return &financev1.DownloadCostProductMasterTemplateResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}
	return &financev1.DownloadCostProductMasterTemplateResponse{
		Base:        successResponse("Template generated successfully"),
		FileContent: data,
		FileName:    "cost_product_master_bulk_import_template.xlsx",
	}, nil
}

// =============================================================================
// mappers
// =============================================================================

func costProductMasterToProto(p *domain.CostProductMaster) *financev1.CostProductMaster {
	erpLinkedAt := ""
	if t := p.ErpLinkedAt(); t != nil {
		erpLinkedAt = t.Format(time.RFC3339)
	}
	return &financev1.CostProductMaster{
		ProductSysId:   p.ProductSysID(),
		ProductCode:    p.ProductCode(),
		ProductTypeId:  p.ProductTypeID(),
		ProductName:    p.ProductName(),
		ShadeCode:      p.ShadeCode(),
		ShadeName:      p.ShadeName(),
		GradeCode:      p.GradeCode(),
		Description:    p.Description(),
		Flex_01:        p.Flex01(),
		Flex_02:        p.Flex02(),
		Flex_03:        p.Flex03(),
		ErpItemCode:    p.ErpItemCode(),
		ErpGradeCode_1: p.ErpGradeCode1(),
		ErpGradeCode_2: p.ErpGradeCode2(),
		ErpLinkedAt:    erpLinkedAt,
		ErpLinkedBy:    p.ErpLinkedBy(),
		IsActive:       p.IsActive(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: p.CreatedAt().Format(time.RFC3339),
			CreatedBy: p.CreatedBy(),
			UpdatedAt: p.UpdatedAt().Format(time.RFC3339),
			UpdatedBy: p.UpdatedBy(),
		},
	}
}

func productMasterErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return ConflictResponse(err.Error())
	case errors.Is(err, domain.ErrInvalidProductName),
		errors.Is(err, domain.ErrInvalidGradeCode),
		errors.Is(err, domain.ErrInactive):
		return ErrorResponse("400", err.Error())
	default:
		return InternalErrorResponse(err.Error())
	}
}
