//nolint:dupl // UOMCategoryHandler mirrors RMCategoryHandler by design — different proto types prevent shared code.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/uomcategory"
	uomcategorydomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/uomcategory"
)

// UOMCategoryHandler implements the UOMCategoryServiceServer interface.
type UOMCategoryHandler struct {
	financev1.UnimplementedUOMCategoryServiceServer
	createHandler    *uomcategory.CreateHandler
	getHandler       *uomcategory.GetHandler
	updateHandler    *uomcategory.UpdateHandler
	deleteHandler    *uomcategory.DeleteHandler
	listHandler      *uomcategory.ListHandler
	exportHandler    *uomcategory.ExportHandler
	importHandler    *uomcategory.ImportHandler
	templateHandler  *uomcategory.TemplateHandler
	validationHelper *ValidationHelper
}

// NewUOMCategoryHandler creates a new UOMCategory gRPC handler.
func NewUOMCategoryHandler(
	repo uomcategorydomain.Repository,
) (*UOMCategoryHandler, error) {
	validationHelper, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}

	return &UOMCategoryHandler{
		createHandler:    uomcategory.NewCreateHandler(repo),
		getHandler:       uomcategory.NewGetHandler(repo),
		updateHandler:    uomcategory.NewUpdateHandler(repo),
		deleteHandler:    uomcategory.NewDeleteHandler(repo),
		listHandler:      uomcategory.NewListHandler(repo),
		exportHandler:    uomcategory.NewExportHandler(repo),
		importHandler:    uomcategory.NewImportHandler(repo),
		templateHandler:  uomcategory.NewTemplateHandler(),
		validationHelper: validationHelper,
	}, nil
}

// CreateUOMCategory creates a new UOM category.
func (h *UOMCategoryHandler) CreateUOMCategory(ctx context.Context, req *financev1.CreateUOMCategoryRequest) (*financev1.CreateUOMCategoryResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordUOMCategoryOperation("create", false)
		return &financev1.CreateUOMCategoryResponse{Base: baseResp}, nil
	}

	cmd := uomcategory.CreateCommand{
		CategoryCode: req.CategoryCode,
		CategoryName: req.CategoryName,
		Description:  req.Description,
		CreatedBy:    getUserFromContext(ctx),
	}

	entity, err := h.createHandler.Handle(ctx, cmd)
	if err != nil {
		RecordUOMCategoryOperation("create", false)
		return &financev1.CreateUOMCategoryResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMCategoryOperation("create", true)

	return &financev1.CreateUOMCategoryResponse{
		Base: successResponse("UOM Category created successfully"),
		Data: uomCategoryEntityToProto(entity),
	}, nil
}

// GetUOMCategory retrieves a UOM category by ID.
func (h *UOMCategoryHandler) GetUOMCategory(ctx context.Context, req *financev1.GetUOMCategoryRequest) (*financev1.GetUOMCategoryResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordUOMCategoryOperation("get", false)
		return &financev1.GetUOMCategoryResponse{Base: baseResp}, nil
	}

	query := uomcategory.GetQuery{UOMCategoryID: req.UomCategoryId}
	entity, err := h.getHandler.Handle(ctx, query)
	if err != nil {
		RecordUOMCategoryOperation("get", false)
		return &financev1.GetUOMCategoryResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMCategoryOperation("get", true)

	return &financev1.GetUOMCategoryResponse{
		Base: successResponse("UOM Category retrieved successfully"),
		Data: uomCategoryEntityToProto(entity),
	}, nil
}

// UpdateUOMCategory updates an existing UOM category.
func (h *UOMCategoryHandler) UpdateUOMCategory(ctx context.Context, req *financev1.UpdateUOMCategoryRequest) (*financev1.UpdateUOMCategoryResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordUOMCategoryOperation("update", false)
		return &financev1.UpdateUOMCategoryResponse{Base: baseResp}, nil
	}

	cmd := uomcategory.UpdateCommand{
		UOMCategoryID: req.UomCategoryId,
		UpdatedBy:     getUserFromContext(ctx),
	}

	if req.CategoryName != nil && *req.CategoryName != "" {
		cmd.CategoryName = req.CategoryName
	}
	if req.Description != nil {
		cmd.Description = req.Description
	}
	if req.IsActive != nil {
		cmd.IsActive = req.IsActive
	}

	entity, err := h.updateHandler.Handle(ctx, cmd)
	if err != nil {
		RecordUOMCategoryOperation("update", false)
		return &financev1.UpdateUOMCategoryResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMCategoryOperation("update", true)

	return &financev1.UpdateUOMCategoryResponse{
		Base: successResponse("UOM Category updated successfully"),
		Data: uomCategoryEntityToProto(entity),
	}, nil
}

// DeleteUOMCategory soft deletes a UOM category.
func (h *UOMCategoryHandler) DeleteUOMCategory(ctx context.Context, req *financev1.DeleteUOMCategoryRequest) (*financev1.DeleteUOMCategoryResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordUOMCategoryOperation("delete", false)
		return &financev1.DeleteUOMCategoryResponse{Base: baseResp}, nil
	}

	cmd := uomcategory.DeleteCommand{
		UOMCategoryID: req.UomCategoryId,
		DeletedBy:     getUserFromContext(ctx),
	}

	if err := h.deleteHandler.Handle(ctx, cmd); err != nil {
		RecordUOMCategoryOperation("delete", false)
		return &financev1.DeleteUOMCategoryResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMCategoryOperation("delete", true)

	return &financev1.DeleteUOMCategoryResponse{
		Base: successResponse("UOM Category deleted successfully"),
	}, nil
}

// ListUOMCategories lists UOM categories with search, filter, and pagination.
func (h *UOMCategoryHandler) ListUOMCategories(ctx context.Context, req *financev1.ListUOMCategoriesRequest) (*financev1.ListUOMCategoriesResponse, error) {
	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := uomcategory.ListQuery{
		Page:      page,
		PageSize:  pageSize,
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	// Handle ActiveFilter enum
	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		active := true
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		active := false
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
		// Show all - no filter
	}

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		RecordUOMCategoryOperation("list", false)
		return &financev1.ListUOMCategoriesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMCategoryOperation("list", true)

	items := make([]*financev1.UOMCategory, len(result.Categories))
	for i, entity := range result.Categories {
		items[i] = uomCategoryEntityToProto(entity)
	}

	return &financev1.ListUOMCategoriesResponse{
		Base: successResponse("UOM Categories retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportUOMCategories exports UOM categories to Excel file.
func (h *UOMCategoryHandler) ExportUOMCategories(ctx context.Context, req *financev1.ExportUOMCategoriesRequest) (*financev1.ExportUOMCategoriesResponse, error) {
	query := uomcategory.ExportQuery{}

	// Handle ActiveFilter enum
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
		RecordUOMCategoryOperation("export", false)
		return &financev1.ExportUOMCategoriesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMCategoryOperation("export", true)

	return &financev1.ExportUOMCategoriesResponse{
		Base:        successResponse("UOM Categories exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportUOMCategories imports UOM categories from Excel file.
func (h *UOMCategoryHandler) ImportUOMCategories(ctx context.Context, req *financev1.ImportUOMCategoriesRequest) (*financev1.ImportUOMCategoriesResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordUOMCategoryOperation("import", false)
		return &financev1.ImportUOMCategoriesResponse{Base: baseResp}, nil
	}

	cmd := uomcategory.ImportCommand{
		FileContent:     req.FileContent,
		FileName:        req.FileName,
		DuplicateAction: req.DuplicateAction,
		CreatedBy:       getUserFromContext(ctx),
	}

	result, err := h.importHandler.Handle(ctx, cmd)
	if err != nil {
		RecordUOMCategoryOperation("import", false)
		return &financev1.ImportUOMCategoriesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordUOMCategoryOperation("import", true)

	importErrors := make([]*financev1.ImportError, len(result.Errors))
	for i, e := range result.Errors {
		importErrors[i] = &financev1.ImportError{
			RowNumber: e.RowNumber,
			Field:     e.Field,
			Message:   e.Message,
		}
	}

	return &financev1.ImportUOMCategoriesResponse{
		Base:         successResponse("Import completed"),
		SuccessCount: result.SuccessCount,
		SkippedCount: result.SkippedCount,
		UpdatedCount: result.UpdatedCount,
		FailedCount:  result.FailedCount,
		Errors:       importErrors,
	}, nil
}

// DownloadUOMCategoryTemplate downloads the Excel import template.
func (h *UOMCategoryHandler) DownloadUOMCategoryTemplate(_ context.Context, _ *financev1.DownloadUOMCategoryTemplateRequest) (*financev1.DownloadUOMCategoryTemplateResponse, error) {
	result, err := h.templateHandler.Handle()
	if err != nil {
		return &financev1.DownloadUOMCategoryTemplateResponse{Base: InternalErrorResponse(err.Error())}, nil
	}

	return &financev1.DownloadUOMCategoryTemplateResponse{
		Base:        successResponse("Template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// uomCategoryEntityToProto converts a domain Category entity to proto message.
func uomCategoryEntityToProto(entity *uomcategorydomain.Category) *financev1.UOMCategory {
	proto := &financev1.UOMCategory{
		UomCategoryId: entity.ID().String(),
		CategoryCode:  entity.Code().String(),
		CategoryName:  entity.Name(),
		Description:   entity.Description(),
		IsActive:      entity.IsActive(),
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

// RecordUOMCategoryOperation records a UOM Category operation metric.
func RecordUOMCategoryOperation(operation string, success bool) {
	uomCategoryOperationsTotal.WithLabelValues(operation, metricStatus(success)).Inc()
}
