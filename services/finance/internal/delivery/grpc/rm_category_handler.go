// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/rmcategory"
	rmcategorydomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcategory"
)

// RMCategoryHandler implements the RMCategoryServiceServer interface.
type RMCategoryHandler struct {
	financev1.UnimplementedRMCategoryServiceServer
	createHandler    *rmcategory.CreateHandler
	getHandler       *rmcategory.GetHandler
	updateHandler    *rmcategory.UpdateHandler
	deleteHandler    *rmcategory.DeleteHandler
	listHandler      *rmcategory.ListHandler
	exportHandler    *rmcategory.ExportHandler
	importHandler    *rmcategory.ImportHandler
	templateHandler  *rmcategory.TemplateHandler
	validationHelper *ValidationHelper
}

// NewRMCategoryHandler creates a new RMCategory gRPC handler.
func NewRMCategoryHandler(
	repo rmcategorydomain.Repository,
) (*RMCategoryHandler, error) {
	validationHelper, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}

	return &RMCategoryHandler{
		createHandler:    rmcategory.NewCreateHandler(repo),
		getHandler:       rmcategory.NewGetHandler(repo),
		updateHandler:    rmcategory.NewUpdateHandler(repo),
		deleteHandler:    rmcategory.NewDeleteHandler(repo),
		listHandler:      rmcategory.NewListHandler(repo),
		exportHandler:    rmcategory.NewExportHandler(repo),
		importHandler:    rmcategory.NewImportHandler(repo),
		templateHandler:  rmcategory.NewTemplateHandler(),
		validationHelper: validationHelper,
	}, nil
}

// CreateRMCategory creates a new raw material category.
func (h *RMCategoryHandler) CreateRMCategory(ctx context.Context, req *financev1.CreateRMCategoryRequest) (*financev1.CreateRMCategoryResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMCategoryOperation("create", false)
		return &financev1.CreateRMCategoryResponse{Base: baseResp}, nil
	}

	cmd := rmcategory.CreateCommand{
		CategoryCode: req.CategoryCode,
		CategoryName: req.CategoryName,
		Description:  req.Description,
		CreatedBy:    getUserFromContext(ctx),
	}

	entity, err := h.createHandler.Handle(ctx, cmd)
	if err != nil {
		RecordRMCategoryOperation("create", false)
		return &financev1.CreateRMCategoryResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMCategoryOperation("create", true)

	return &financev1.CreateRMCategoryResponse{
		Base: successResponse("RM Category created successfully"),
		Data: rmCategoryEntityToProto(entity),
	}, nil
}

// GetRMCategory retrieves a raw material category by ID.
func (h *RMCategoryHandler) GetRMCategory(ctx context.Context, req *financev1.GetRMCategoryRequest) (*financev1.GetRMCategoryResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMCategoryOperation("get", false)
		return &financev1.GetRMCategoryResponse{Base: baseResp}, nil
	}

	query := rmcategory.GetQuery{RMCategoryID: req.RmCategoryId}
	entity, err := h.getHandler.Handle(ctx, query)
	if err != nil {
		RecordRMCategoryOperation("get", false)
		return &financev1.GetRMCategoryResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMCategoryOperation("get", true)

	return &financev1.GetRMCategoryResponse{
		Base: successResponse("RM Category retrieved successfully"),
		Data: rmCategoryEntityToProto(entity),
	}, nil
}

// UpdateRMCategory updates an existing raw material category.
func (h *RMCategoryHandler) UpdateRMCategory(ctx context.Context, req *financev1.UpdateRMCategoryRequest) (*financev1.UpdateRMCategoryResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMCategoryOperation("update", false)
		return &financev1.UpdateRMCategoryResponse{Base: baseResp}, nil
	}

	cmd := rmcategory.UpdateCommand{
		RMCategoryID: req.RmCategoryId,
		UpdatedBy:    getUserFromContext(ctx),
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
		RecordRMCategoryOperation("update", false)
		return &financev1.UpdateRMCategoryResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMCategoryOperation("update", true)

	return &financev1.UpdateRMCategoryResponse{
		Base: successResponse("RM Category updated successfully"),
		Data: rmCategoryEntityToProto(entity),
	}, nil
}

// DeleteRMCategory soft deletes a raw material category.
func (h *RMCategoryHandler) DeleteRMCategory(ctx context.Context, req *financev1.DeleteRMCategoryRequest) (*financev1.DeleteRMCategoryResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMCategoryOperation("delete", false)
		return &financev1.DeleteRMCategoryResponse{Base: baseResp}, nil
	}

	cmd := rmcategory.DeleteCommand{
		RMCategoryID: req.RmCategoryId,
		DeletedBy:    getUserFromContext(ctx),
	}

	if err := h.deleteHandler.Handle(ctx, cmd); err != nil {
		RecordRMCategoryOperation("delete", false)
		return &financev1.DeleteRMCategoryResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMCategoryOperation("delete", true)

	return &financev1.DeleteRMCategoryResponse{
		Base: successResponse("RM Category deleted successfully"),
	}, nil
}

// ListRMCategories lists raw material categories with search, filter, and pagination.
func (h *RMCategoryHandler) ListRMCategories(ctx context.Context, req *financev1.ListRMCategoriesRequest) (*financev1.ListRMCategoriesResponse, error) {
	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := rmcategory.ListQuery{
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
		RecordRMCategoryOperation("list", false)
		return &financev1.ListRMCategoriesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMCategoryOperation("list", true)

	items := make([]*financev1.RMCategory, len(result.Categories))
	for i, entity := range result.Categories {
		items[i] = rmCategoryEntityToProto(entity)
	}

	return &financev1.ListRMCategoriesResponse{
		Base: successResponse("RM Categories retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportRMCategories exports raw material categories to Excel file.
func (h *RMCategoryHandler) ExportRMCategories(ctx context.Context, req *financev1.ExportRMCategoriesRequest) (*financev1.ExportRMCategoriesResponse, error) {
	query := rmcategory.ExportQuery{}

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
		RecordRMCategoryOperation("export", false)
		return &financev1.ExportRMCategoriesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMCategoryOperation("export", true)

	return &financev1.ExportRMCategoriesResponse{
		Base:        successResponse("RM Categories exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportRMCategories imports raw material categories from Excel file.
func (h *RMCategoryHandler) ImportRMCategories(ctx context.Context, req *financev1.ImportRMCategoriesRequest) (*financev1.ImportRMCategoriesResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMCategoryOperation("import", false)
		return &financev1.ImportRMCategoriesResponse{Base: baseResp}, nil
	}

	cmd := rmcategory.ImportCommand{
		FileContent:     req.FileContent,
		FileName:        req.FileName,
		DuplicateAction: req.DuplicateAction,
		CreatedBy:       getUserFromContext(ctx),
	}

	result, err := h.importHandler.Handle(ctx, cmd)
	if err != nil {
		RecordRMCategoryOperation("import", false)
		return &financev1.ImportRMCategoriesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMCategoryOperation("import", true)

	importErrors := make([]*financev1.ImportError, len(result.Errors))
	for i, e := range result.Errors {
		importErrors[i] = &financev1.ImportError{
			RowNumber: e.RowNumber,
			Field:     e.Field,
			Message:   e.Message,
		}
	}

	return &financev1.ImportRMCategoriesResponse{
		Base:         successResponse("Import completed"),
		SuccessCount: result.SuccessCount,
		SkippedCount: result.SkippedCount,
		UpdatedCount: result.UpdatedCount,
		FailedCount:  result.FailedCount,
		Errors:       importErrors,
	}, nil
}

// DownloadRMCategoryTemplate downloads the Excel import template.
func (h *RMCategoryHandler) DownloadRMCategoryTemplate(_ context.Context, _ *financev1.DownloadRMCategoryTemplateRequest) (*financev1.DownloadRMCategoryTemplateResponse, error) {
	result, err := h.templateHandler.Handle()
	if err != nil {
		return &financev1.DownloadRMCategoryTemplateResponse{Base: InternalErrorResponse(err.Error())}, nil
	}

	return &financev1.DownloadRMCategoryTemplateResponse{
		Base:        successResponse("Template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// rmCategoryEntityToProto converts a domain RMCategory entity to proto message.
func rmCategoryEntityToProto(entity *rmcategorydomain.RMCategory) *financev1.RMCategory {
	proto := &financev1.RMCategory{
		RmCategoryId: entity.ID().String(),
		CategoryCode: entity.Code().String(),
		CategoryName: entity.Name(),
		Description:  entity.Description(),
		IsActive:     entity.IsActive(),
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

// RecordRMCategoryOperation records an RM Category operation metric.
func RecordRMCategoryOperation(operation string, success bool) {
	opStatus := "success"
	if !success {
		opStatus = "failure"
	}
	rmCategoryOperationsTotal.WithLabelValues(operation, opStatus).Inc()
}
