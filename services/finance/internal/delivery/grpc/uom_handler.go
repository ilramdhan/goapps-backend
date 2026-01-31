// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"strings"
	"time"

	commonv1 "github.com/ilramdhan/goapps-backend/gen/common/v1"
	financev1 "github.com/ilramdhan/goapps-backend/gen/finance/v1"
	"github.com/ilramdhan/goapps-backend/services/finance/internal/application/uom"
	uomdomain "github.com/ilramdhan/goapps-backend/services/finance/internal/domain/uom"
	"github.com/ilramdhan/goapps-backend/services/finance/internal/infrastructure/redis"
)

// UOMHandler implements the UOMServiceServer interface.
type UOMHandler struct {
	financev1.UnimplementedUOMServiceServer
	createHandler    *uom.CreateHandler
	getHandler       *uom.GetHandler
	updateHandler    *uom.UpdateHandler
	deleteHandler    *uom.DeleteHandler
	listHandler      *uom.ListHandler
	exportHandler    *uom.ExportHandler
	importHandler    *uom.ImportHandler
	templateHandler  *uom.TemplateHandler
	cache            *redis.UOMCache
	validationHelper *ValidationHelper
}

// NewUOMHandler creates a new UOM gRPC handler.
func NewUOMHandler(
	repo uomdomain.Repository,
	cache *redis.UOMCache,
) (*UOMHandler, error) {
	validationHelper, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}

	return &UOMHandler{
		createHandler:    uom.NewCreateHandler(repo),
		getHandler:       uom.NewGetHandler(repo),
		updateHandler:    uom.NewUpdateHandler(repo),
		deleteHandler:    uom.NewDeleteHandler(repo),
		listHandler:      uom.NewListHandler(repo),
		exportHandler:    uom.NewExportHandler(repo),
		importHandler:    uom.NewImportHandler(repo),
		templateHandler:  uom.NewTemplateHandler(),
		cache:            cache,
		validationHelper: validationHelper,
	}, nil
}

// CreateUOM creates a new UOM.
func (h *UOMHandler) CreateUOM(ctx context.Context, req *financev1.CreateUOMRequest) (*financev1.CreateUOMResponse, error) {
	// Validate request
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordUOMOperation("create", false)
		return &financev1.CreateUOMResponse{Base: baseResp}, nil
	}

	cmd := uom.CreateCommand{
		UOMCode:     req.UomCode,
		UOMName:     req.UomName,
		UOMCategory: protoToCategory(req.UomCategory),
		Description: req.Description,
		CreatedBy:   getUserFromContext(ctx),
	}

	entity, err := h.createHandler.Handle(ctx, cmd)
	if err != nil {
		RecordUOMOperation("create", false)
		return &financev1.CreateUOMResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMOperation("create", true)

	// Invalidate list cache
	if h.cache != nil {
		_ = h.cache.InvalidateList(ctx)
	}

	return &financev1.CreateUOMResponse{
		Base: successResponse("UOM created successfully"),
		Data: entityToProto(entity),
	}, nil
}

// GetUOM retrieves a UOM by ID.
func (h *UOMHandler) GetUOM(ctx context.Context, req *financev1.GetUOMRequest) (*financev1.GetUOMResponse, error) {
	// Validate request
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordUOMOperation("get", false)
		return &financev1.GetUOMResponse{Base: baseResp}, nil
	}

	query := uom.GetQuery{UOMID: req.UomId}
	entity, err := h.getHandler.Handle(ctx, query)
	if err != nil {
		RecordUOMOperation("get", false)
		return &financev1.GetUOMResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMOperation("get", true)

	return &financev1.GetUOMResponse{
		Base: successResponse("UOM retrieved successfully"),
		Data: entityToProto(entity),
	}, nil
}

// UpdateUOM updates an existing UOM.
func (h *UOMHandler) UpdateUOM(ctx context.Context, req *financev1.UpdateUOMRequest) (*financev1.UpdateUOMResponse, error) {
	// Validate request
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordUOMOperation("update", false)
		return &financev1.UpdateUOMResponse{Base: baseResp}, nil
	}

	cmd := uom.UpdateCommand{
		UOMID:     req.UomId,
		UpdatedBy: getUserFromContext(ctx),
	}

	if req.UomName != nil && *req.UomName != "" {
		cmd.UOMName = req.UomName
	}
	if req.UomCategory != nil && *req.UomCategory != financev1.UOMCategory_UOM_CATEGORY_UNSPECIFIED {
		cat := protoToCategory(*req.UomCategory)
		cmd.UOMCategory = &cat
	}
	if req.Description != nil {
		cmd.Description = req.Description
	}
	if req.IsActive != nil {
		cmd.IsActive = req.IsActive
	}

	entity, err := h.updateHandler.Handle(ctx, cmd)
	if err != nil {
		RecordUOMOperation("update", false)
		return &financev1.UpdateUOMResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMOperation("update", true)

	// Invalidate cache
	if h.cache != nil {
		_ = h.cache.InvalidateList(ctx)
	}

	return &financev1.UpdateUOMResponse{
		Base: successResponse("UOM updated successfully"),
		Data: entityToProto(entity),
	}, nil
}

// DeleteUOM soft deletes a UOM.
func (h *UOMHandler) DeleteUOM(ctx context.Context, req *financev1.DeleteUOMRequest) (*financev1.DeleteUOMResponse, error) {
	// Validate request
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordUOMOperation("delete", false)
		return &financev1.DeleteUOMResponse{Base: baseResp}, nil
	}

	cmd := uom.DeleteCommand{
		UOMID:     req.UomId,
		DeletedBy: getUserFromContext(ctx),
	}

	if err := h.deleteHandler.Handle(ctx, cmd); err != nil {
		RecordUOMOperation("delete", false)
		return &financev1.DeleteUOMResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMOperation("delete", true)

	// Invalidate cache
	if h.cache != nil {
		_ = h.cache.InvalidateList(ctx)
	}

	return &financev1.DeleteUOMResponse{
		Base: successResponse("UOM deleted successfully"),
	}, nil
}

// ListUOMs lists UOMs with search, filter, and pagination.
func (h *UOMHandler) ListUOMs(ctx context.Context, req *financev1.ListUOMsRequest) (*financev1.ListUOMsResponse, error) {
	// Note: ListUOMs typically doesn't have strict validation requirements

	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := uom.ListQuery{
		Page:      page,
		PageSize:  pageSize,
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	if req.Category != financev1.UOMCategory_UOM_CATEGORY_UNSPECIFIED {
		cat := protoToCategory(req.Category)
		query.Category = &cat
	}

	// Handle ActiveFilter enum
	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		active := true
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		active := false
		query.IsActive = &active
		// ACTIVE_FILTER_UNSPECIFIED (0) means show all - no filter
	}

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		RecordUOMOperation("list", false)
		return &financev1.ListUOMsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMOperation("list", true)

	items := make([]*financev1.UOM, len(result.UOMs))
	for i, entity := range result.UOMs {
		items[i] = entityToProto(entity)
	}

	return &financev1.ListUOMsResponse{
		Base: successResponse("UOMs retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportUOMs exports UOMs to Excel file.
func (h *UOMHandler) ExportUOMs(ctx context.Context, req *financev1.ExportUOMsRequest) (*financev1.ExportUOMsResponse, error) {
	query := uom.ExportQuery{}

	if req.Category != financev1.UOMCategory_UOM_CATEGORY_UNSPECIFIED {
		cat := protoToCategory(req.Category)
		query.Category = &cat
	}

	// Handle ActiveFilter enum
	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		active := true
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		active := false
		query.IsActive = &active
		// ACTIVE_FILTER_UNSPECIFIED (0) means export all - no filter
	}

	result, err := h.exportHandler.Handle(ctx, query)
	if err != nil {
		RecordUOMOperation("export", false)
		return &financev1.ExportUOMsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMOperation("export", true)

	return &financev1.ExportUOMsResponse{
		Base:        successResponse("UOMs exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportUOMs imports UOMs from Excel file.
func (h *UOMHandler) ImportUOMs(ctx context.Context, req *financev1.ImportUOMsRequest) (*financev1.ImportUOMsResponse, error) {
	// Validate request
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordUOMOperation("import", false)
		return &financev1.ImportUOMsResponse{Base: baseResp}, nil
	}

	cmd := uom.ImportCommand{
		FileContent:     req.FileContent,
		FileName:        req.FileName,
		DuplicateAction: req.DuplicateAction,
		CreatedBy:       getUserFromContext(ctx),
	}

	result, err := h.importHandler.Handle(ctx, cmd)
	if err != nil {
		RecordUOMOperation("import", false)
		return &financev1.ImportUOMsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMOperation("import", true)

	// Invalidate all cache after import
	if h.cache != nil {
		_ = h.cache.InvalidateAll(ctx)
	}

	errors := make([]*financev1.ImportError, len(result.Errors))
	for i, e := range result.Errors {
		errors[i] = &financev1.ImportError{
			RowNumber: e.RowNumber,
			Field:     e.Field,
			Message:   e.Message,
		}
	}

	return &financev1.ImportUOMsResponse{
		Base:         successResponse("Import completed"),
		SuccessCount: result.SuccessCount,
		SkippedCount: result.SkippedCount,
		UpdatedCount: result.UpdatedCount,
		FailedCount:  result.FailedCount,
		Errors:       errors,
	}, nil
}

// DownloadTemplate downloads the Excel import template.
func (h *UOMHandler) DownloadTemplate(ctx context.Context, req *financev1.DownloadTemplateRequest) (*financev1.DownloadTemplateResponse, error) {
	result, err := h.templateHandler.Handle()
	if err != nil {
		return &financev1.DownloadTemplateResponse{Base: InternalErrorResponse(err.Error())}, nil
	}

	return &financev1.DownloadTemplateResponse{
		Base:        successResponse("Template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// Helper functions

func successResponse(message string) *commonv1.BaseResponse {
	return &commonv1.BaseResponse{
		IsSuccess:  true,
		StatusCode: "200",
		Message:    message,
	}
}

func domainErrorToBaseResponse(err error) *commonv1.BaseResponse {
	if err == nil {
		return successResponse("")
	}

	errMsg := err.Error()

	switch {
	case strings.Contains(errMsg, "not found"):
		return NotFoundResponse(errMsg)
	case strings.Contains(errMsg, "already exists"):
		return ConflictResponse(errMsg)
	case strings.Contains(errMsg, "invalid"):
		return ErrorResponse("400", errMsg)
	default:
		return InternalErrorResponse(errMsg)
	}
}

func protoToCategory(cat financev1.UOMCategory) string {
	switch cat {
	case financev1.UOMCategory_UOM_CATEGORY_WEIGHT:
		return "WEIGHT"
	case financev1.UOMCategory_UOM_CATEGORY_LENGTH:
		return "LENGTH"
	case financev1.UOMCategory_UOM_CATEGORY_VOLUME:
		return "VOLUME"
	case financev1.UOMCategory_UOM_CATEGORY_QUANTITY:
		return "QUANTITY"
	default:
		return ""
	}
}

func categoryToProto(cat string) financev1.UOMCategory {
	switch cat {
	case "WEIGHT":
		return financev1.UOMCategory_UOM_CATEGORY_WEIGHT
	case "LENGTH":
		return financev1.UOMCategory_UOM_CATEGORY_LENGTH
	case "VOLUME":
		return financev1.UOMCategory_UOM_CATEGORY_VOLUME
	case "QUANTITY":
		return financev1.UOMCategory_UOM_CATEGORY_QUANTITY
	default:
		return financev1.UOMCategory_UOM_CATEGORY_UNSPECIFIED
	}
}

func entityToProto(entity *uomdomain.UOM) *financev1.UOM {
	proto := &financev1.UOM{
		UomId:       entity.ID().String(),
		UomCode:     entity.Code().String(),
		UomName:     entity.Name(),
		UomCategory: categoryToProto(entity.Category().String()),
		Description: entity.Description(),
		IsActive:    entity.IsActive(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: entity.CreatedAt().Format(time.RFC3339),
			CreatedBy: entity.CreatedBy(),
		},
	}

	if entity.UpdatedAt() != nil {
		proto.Audit.UpdatedAt = entity.UpdatedAt().Format(time.RFC3339)
	}
	if entity.UpdatedBy() != nil {
		proto.Audit.UpdatedBy = *entity.UpdatedBy()
	}

	return proto
}

func getUserFromContext(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok && userID != "" {
		return userID
	}
	return "system" // Default for now, will be from JWT in IAM service
}
