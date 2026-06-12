package grpc

import (
	"context"
	"errors"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproducttype"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

// CostProductTypeHandler implements financev1.CostProductTypeServiceServer.
type CostProductTypeHandler struct {
	financev1.UnimplementedCostProductTypeServiceServer
	createHandler  *app.CreateHandler
	getHandler     *app.GetHandler
	updateHandler  *app.UpdateHandler
	listHandler    *app.ListHandler
	exportHandler  *app.ExportHandler
	importHandler  *app.ImportHandler
	templateHandler *app.TemplateHandler
	validation     *ValidationHelper
}

// NewCostProductTypeHandler constructs the handler.
func NewCostProductTypeHandler(repo domain.Repository) (*CostProductTypeHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &CostProductTypeHandler{
		createHandler:   app.NewCreateHandler(repo),
		getHandler:      app.NewGetHandler(repo),
		updateHandler:   app.NewUpdateHandler(repo),
		listHandler:     app.NewListHandler(repo),
		exportHandler:   app.NewExportHandler(repo),
		importHandler:   app.NewImportHandler(repo),
		templateHandler: app.NewTemplateHandler(),
		validation:      v,
	}, nil
}

// CreateCostProductType creates a new product type.
func (h *CostProductTypeHandler) CreateCostProductType(ctx context.Context, req *financev1.CreateCostProductTypeRequest) (*financev1.CreateCostProductTypeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.CreateCostProductTypeResponse{Base: baseResp}, nil
	}
	t, err := h.createHandler.Handle(ctx, app.CreateCommand{
		TypeCode: req.GetTypeCode(),
		TypeName: req.GetTypeName(),
	})
	if err != nil {
		return &financev1.CreateCostProductTypeResponse{Base: productTypeErrToBase(err)}, nil
	}
	return &financev1.CreateCostProductTypeResponse{
		Base: successResponse("Cost product type created"),
		Data: costProductTypeToProto(t),
	}, nil
}

// GetCostProductType returns by id.
func (h *CostProductTypeHandler) GetCostProductType(ctx context.Context, req *financev1.GetCostProductTypeRequest) (*financev1.GetCostProductTypeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.GetCostProductTypeResponse{Base: baseResp}, nil
	}
	t, err := h.getHandler.Handle(ctx, app.GetQuery{TypeID: req.GetTypeId()})
	if err != nil {
		return &financev1.GetCostProductTypeResponse{Base: productTypeErrToBase(err)}, nil
	}
	return &financev1.GetCostProductTypeResponse{
		Base: successResponse("OK"),
		Data: costProductTypeToProto(t),
	}, nil
}

// UpdateCostProductType updates name + active flag.
func (h *CostProductTypeHandler) UpdateCostProductType(ctx context.Context, req *financev1.UpdateCostProductTypeRequest) (*financev1.UpdateCostProductTypeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateCostProductTypeResponse{Base: baseResp}, nil
	}
	t, err := h.updateHandler.Handle(ctx, app.UpdateCommand{
		TypeID:   req.GetTypeId(),
		TypeName: req.GetTypeName(),
		IsActive: req.GetIsActive(),
	})
	if err != nil {
		return &financev1.UpdateCostProductTypeResponse{Base: productTypeErrToBase(err)}, nil
	}
	return &financev1.UpdateCostProductTypeResponse{
		Base: successResponse("Cost product type updated"),
		Data: costProductTypeToProto(t),
	}, nil
}

// ListCostProductTypes paginates types.
func (h *CostProductTypeHandler) ListCostProductTypes(ctx context.Context, req *financev1.ListCostProductTypesRequest) (*financev1.ListCostProductTypesResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		return &financev1.ListCostProductTypesResponse{Base: baseResp}, nil
	}
	page := int32(1)
	pageSize := int32(20)
	if req.Pagination != nil {
		if req.Pagination.Page > 0 {
			page = req.Pagination.Page
		}
		if req.Pagination.PageSize > 0 {
			pageSize = req.Pagination.PageSize
		}
	}
	res, err := h.listHandler.Handle(ctx, app.ListQuery{
		Search: req.GetSearch(), ActiveFilter: req.GetActiveFilter(),
		Page: int(page), PageSize: int(pageSize),
		SortBy: req.GetSortBy(), SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &financev1.ListCostProductTypesResponse{Base: productTypeErrToBase(err)}, nil
	}
	items := make([]*financev1.CostProductType, 0, len(res.Items))
	for _, t := range res.Items {
		items = append(items, costProductTypeToProto(t))
	}
	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = safeIntToInt32(int((res.Total + int64(pageSize) - 1) / int64(pageSize)))
	}
	return &financev1.ListCostProductTypesResponse{
		Base: successResponse("OK"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: page,
			PageSize:    pageSize,
			TotalItems:  res.Total,
			TotalPages:  totalPages,
		},
	}, nil
}

// =============================================================================
// mappers
// =============================================================================

func costProductTypeToProto(t *domain.CostProductType) *financev1.CostProductType {
	return &financev1.CostProductType{
		TypeId:   t.TypeID(),
		TypeCode: t.TypeCode(),
		TypeName: t.TypeName(),
		IsActive: t.IsActive(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: t.CreatedAt().Format(time.RFC3339),
			UpdatedAt: t.UpdatedAt().Format(time.RFC3339),
		},
	}
}

func productTypeErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return ConflictResponse(err.Error())
	case errors.Is(err, domain.ErrInvalidTypeCode), errors.Is(err, domain.ErrInvalidTypeName):
		return ErrorResponse("400", err.Error())
	default:
		return InternalErrorResponse(err.Error())
	}
}

// ExportCostProductTypes exports CostProductTypes to Excel.
func (h *CostProductTypeHandler) ExportCostProductTypes(ctx context.Context, req *financev1.ExportCostProductTypesRequest) (*financev1.ExportCostProductTypesResponse, error) {
	result, err := h.exportHandler.Handle(ctx, app.ExportQuery{ActiveFilter: req.GetActiveFilter()})
	if err != nil {
		return &financev1.ExportCostProductTypesResponse{Base: productTypeErrToBase(err)}, nil
	}
	return &financev1.ExportCostProductTypesResponse{
		Base:        successResponse("Cost product types exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportCostProductTypes imports CostProductTypes from Excel.
func (h *CostProductTypeHandler) ImportCostProductTypes(ctx context.Context, req *financev1.ImportCostProductTypesRequest) (*financev1.ImportCostProductTypesResponse, error) {
	result, err := h.importHandler.Handle(ctx, app.ImportCommand{
		FileContent:     req.GetFileContent(),
		FileName:        req.GetFileName(),
		DuplicateAction: req.GetDuplicateAction(),
	})
	if err != nil {
		return &financev1.ImportCostProductTypesResponse{Base: productTypeErrToBase(err)}, nil
	}

	importErrors := make([]*financev1.ImportError, len(result.Errors))
	for i, e := range result.Errors {
		importErrors[i] = &financev1.ImportError{
			RowNumber: e.RowNumber,
			Field:     e.Field,
			Message:   e.Message,
		}
	}

	return &financev1.ImportCostProductTypesResponse{
		Base:         successResponse("Import completed"),
		SuccessCount: result.SuccessCount,
		SkippedCount: result.SkippedCount,
		UpdatedCount: result.UpdatedCount,
		FailedCount:  result.FailedCount,
		Errors:       importErrors,
	}, nil
}

// DownloadCostProductTypeTemplate downloads the Excel import template.
func (h *CostProductTypeHandler) DownloadCostProductTypeTemplate(_ context.Context, _ *financev1.DownloadCostProductTypeTemplateRequest) (*financev1.DownloadCostProductTypeTemplateResponse, error) {
	result, err := h.templateHandler.Handle()
	if err != nil {
		return &financev1.DownloadCostProductTypeTemplateResponse{Base: InternalErrorResponse(err.Error())}, nil
	}
	return &financev1.DownloadCostProductTypeTemplateResponse{
		Base:        successResponse("Template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}
