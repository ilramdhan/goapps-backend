// Package grpc provides gRPC server implementation for the finance service.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appproductgrade "github.com/mutugading/goapps-backend/services/finance/internal/application/productgrade"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
)

// ProductGradeHandler implements financev1.ProductGradeServiceServer.
type ProductGradeHandler struct {
	financev1.UnimplementedProductGradeServiceServer
	createHandler *appproductgrade.CreateHandler
	getHandler    *appproductgrade.GetHandler
	listHandler   *appproductgrade.ListHandler
	updateHandler *appproductgrade.UpdateHandler
	deleteHandler *appproductgrade.DeleteHandler
	validation    *ValidationHelper
}

// NewProductGradeHandler constructs a ProductGradeHandler.
func NewProductGradeHandler(repo productgrade.Repository) (*ProductGradeHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &ProductGradeHandler{
		createHandler: appproductgrade.NewCreateHandler(repo),
		getHandler:    appproductgrade.NewGetHandler(repo),
		listHandler:   appproductgrade.NewListHandler(repo),
		updateHandler: appproductgrade.NewUpdateHandler(repo),
		deleteHandler: appproductgrade.NewDeleteHandler(repo),
		validation:    v,
	}, nil
}

// CreateProductGrade creates a new product grade record.
func (h *ProductGradeHandler) CreateProductGrade(ctx context.Context, req *financev1.CreateProductGradeRequest) (*financev1.CreateProductGradeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordProductGradeOperation("create", false)
		return &financev1.CreateProductGradeResponse{Base: baseResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appproductgrade.CreateCommand{
		Code:            req.PgCode,
		Name:            req.PgName,
		Description:     req.PgDescription,
		BCPerc:          req.BcPerc,
		NonStdPerc:      req.NonStdPerc,
		BCRecoveryRate:  req.BcRecoveryRate,
		PgDetailProduct: req.PgDetailProduct,
		PgGradeLabel:    req.PgGradeLabel,
		StdSellingPrice: req.StdSellingPrice,
		SpValue:         req.SpValue,
		Notes:           req.Notes,
		CreatedBy:       getUserFromContext(ctx),
	})
	if err != nil {
		RecordProductGradeOperation("create", false)
		return &financev1.CreateProductGradeResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordProductGradeOperation("create", true)
	return &financev1.CreateProductGradeResponse{
		Base: successResponse("Product grade created successfully"),
		Data: productGradeEntityToProto(entity),
	}, nil
}

// GetProductGrade retrieves a product grade record by ID.
func (h *ProductGradeHandler) GetProductGrade(ctx context.Context, req *financev1.GetProductGradeRequest) (*financev1.GetProductGradeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordProductGradeOperation("get", false)
		return &financev1.GetProductGradeResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.PgId)
	if err != nil {
		RecordProductGradeOperation("get", false)
		return &financev1.GetProductGradeResponse{Base: invalidIDResponse("pg_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.getHandler.Handle(ctx, appproductgrade.GetQuery{ProductGradeID: id})
	if err != nil {
		RecordProductGradeOperation("get", false)
		return &financev1.GetProductGradeResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordProductGradeOperation("get", true)
	return &financev1.GetProductGradeResponse{
		Base: successResponse("Product grade retrieved successfully"),
		Data: productGradeEntityToProto(entity),
	}, nil
}

// UpdateProductGrade updates an existing product grade record.
func (h *ProductGradeHandler) UpdateProductGrade(ctx context.Context, req *financev1.UpdateProductGradeRequest) (*financev1.UpdateProductGradeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordProductGradeOperation("update", false)
		return &financev1.UpdateProductGradeResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.PgId)
	if err != nil {
		RecordProductGradeOperation("update", false)
		return &financev1.UpdateProductGradeResponse{Base: invalidIDResponse("pg_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.updateHandler.Handle(ctx, appproductgrade.UpdateCommand{
		ProductGradeID:  id,
		Name:            req.PgName,
		Description:     req.PgDescription,
		BCPerc:          req.BcPerc,
		NonStdPerc:      req.NonStdPerc,
		BCRecoveryRate:  req.BcRecoveryRate,
		PgDetailProduct: req.PgDetailProduct,
		PgGradeLabel:    req.PgGradeLabel,
		StdSellingPrice: req.StdSellingPrice,
		SpValue:         req.SpValue,
		Notes:           req.Notes,
		IsActive:        req.IsActive,
		UpdatedBy:       getUserFromContext(ctx),
	})
	if err != nil {
		RecordProductGradeOperation("update", false)
		return &financev1.UpdateProductGradeResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordProductGradeOperation("update", true)
	return &financev1.UpdateProductGradeResponse{
		Base: successResponse("Product grade updated successfully"),
		Data: productGradeEntityToProto(entity),
	}, nil
}

// DeleteProductGrade soft-deletes a product grade record.
func (h *ProductGradeHandler) DeleteProductGrade(ctx context.Context, req *financev1.DeleteProductGradeRequest) (*financev1.DeleteProductGradeResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordProductGradeOperation("delete", false)
		return &financev1.DeleteProductGradeResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.PgId)
	if err != nil {
		RecordProductGradeOperation("delete", false)
		return &financev1.DeleteProductGradeResponse{Base: invalidIDResponse("pg_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	if err := h.deleteHandler.Handle(ctx, appproductgrade.DeleteCommand{ProductGradeID: id, DeletedBy: getUserFromContext(ctx)}); err != nil {
		RecordProductGradeOperation("delete", false)
		return &financev1.DeleteProductGradeResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordProductGradeOperation("delete", true)
	return &financev1.DeleteProductGradeResponse{Base: successResponse("Product grade deleted successfully")}, nil
}

// ListProductGrades lists product grade records with search, filter, and pagination.
func (h *ProductGradeHandler) ListProductGrades(ctx context.Context, req *financev1.ListProductGradesRequest) (*financev1.ListProductGradesResponse, error) {
	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := appproductgrade.ListQuery{
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
		RecordProductGradeOperation("list", false)
		return &financev1.ListProductGradesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordProductGradeOperation("list", true)

	items := make([]*financev1.ProductGrade, len(result.Grades))
	for i, e := range result.Grades {
		items[i] = productGradeEntityToProto(e)
	}

	return &financev1.ListProductGradesResponse{
		Base: successResponse("Product grades retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportProductGrades is not yet implemented.
func (h *ProductGradeHandler) ExportProductGrades(_ context.Context, _ *financev1.ExportProductGradesRequest) (*financev1.ExportProductGradesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ExportProductGrades not implemented")
}

// ImportProductGrades is not yet implemented.
func (h *ProductGradeHandler) ImportProductGrades(_ context.Context, _ *financev1.ImportProductGradesRequest) (*financev1.ImportProductGradesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ImportProductGrades not implemented")
}

// DownloadProductGradeTemplate is not yet implemented.
func (h *ProductGradeHandler) DownloadProductGradeTemplate(_ context.Context, _ *financev1.DownloadProductGradeTemplateRequest) (*financev1.DownloadProductGradeTemplateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DownloadProductGradeTemplate not implemented")
}

// productGradeEntityToProto converts a domain ProductGrade entity to its proto representation.
func productGradeEntityToProto(e *productgrade.Entity) *financev1.ProductGrade {
	p := &financev1.ProductGrade{
		PgId:            e.ID().String(),
		PgCode:          e.Code(),
		PgName:          e.Name(),
		PgDescription:   e.Description(),
		BcPerc:          e.BCPerc(),
		NonStdPerc:      e.NonStdPerc(),
		BcRecoveryRate:  e.BCRecoveryRate(),
		PgDetailProduct: e.PgDetailProduct(),
		PgGradeLabel:    e.PgGradeLabel(),
		StdSellingPrice: e.StdSellingPrice(),
		SpValue:         e.SpValue(),
		IsActive:        e.IsActive(),
		Notes:           e.Notes(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.UpdatedAt() != nil {
		p.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		p.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return p
}
