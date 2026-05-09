// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appproduct "github.com/mutugading/goapps-backend/services/finance/internal/application/product"
	domainproduct "github.com/mutugading/goapps-backend/services/finance/internal/domain/product"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ProductGRPCHandler implements financev1.ProductServiceServer.
type ProductGRPCHandler struct {
	financev1.UnimplementedProductServiceServer
	createHandler    *appproduct.CreateHandler
	getHandler       *appproduct.GetHandler
	listHandler      *appproduct.ListHandler
	updateHandler    *appproduct.UpdateHandler
	deleteHandler    *appproduct.DeleteHandler
	duplicateHandler *appproduct.DuplicateHandler
	// productRepo is kept for the ListProductsByRequest RPC which needs
	// direct pagination-aware access to a request-scoped list of products.
	productRepo      domainproduct.Repository
	validationHelper *ValidationHelper
}

// NewProductGRPCHandler constructs a ProductGRPCHandler.
func NewProductGRPCHandler(
	create *appproduct.CreateHandler,
	get *appproduct.GetHandler,
	list *appproduct.ListHandler,
	update *appproduct.UpdateHandler,
	deleteH *appproduct.DeleteHandler,
	duplicate *appproduct.DuplicateHandler,
	repo domainproduct.Repository,
) (*ProductGRPCHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &ProductGRPCHandler{
		createHandler:    create,
		getHandler:       get,
		listHandler:      list,
		updateHandler:    update,
		deleteHandler:    deleteH,
		duplicateHandler: duplicate,
		productRepo:      repo,
		validationHelper: v,
	}, nil
}

// CreateProduct creates a new product.
func (h *ProductGRPCHandler) CreateProduct(ctx context.Context, req *financev1.CreateProductRequest) (*financev1.CreateProductResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.CreateProductResponse{Base: baseResp}, nil
	}

	deptID, err := uuid.Parse(req.GetCreatedByDeptId())
	if err != nil {
		return &financev1.CreateProductResponse{Base: BadRequestResponse("invalid created_by_dept_id: "+err.Error())}, nil
	}

	var requestID uuid.UUID
	if rid := req.GetCurrentRequestId(); rid != "" {
		requestID, err = uuid.Parse(rid)
		if err != nil {
			return &financev1.CreateProductResponse{Base: BadRequestResponse("invalid current_request_id: "+err.Error())}, nil
		}
	}

	p, err := h.createHandler.Handle(ctx, appproduct.CreateCommand{
		Code:             req.GetProductCode(),
		Name:             req.GetProductName(),
		ItemCode:         req.GetProductItemCode(),
		ShadeCode:        req.GetProductShadeCode(),
		ShadeName:        req.GetProductShadeName(),
		DeptID:           deptID,
		DeptCode:         "",
		Purpose:          req.GetPurpose(),
		CurrentRequestID: requestID,
		CreatedBy:        getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.CreateProductResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &financev1.CreateProductResponse{
		Base: successResponse("product created successfully"),
		Data: productToProto(p),
	}, nil
}

// GetProduct retrieves a product by ID.
func (h *ProductGRPCHandler) GetProduct(ctx context.Context, req *financev1.GetProductRequest) (*financev1.GetProductResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.GetProductResponse{Base: baseResp}, nil
	}

	productID, err := uuid.Parse(req.GetProductId())
	if err != nil {
		return &financev1.GetProductResponse{Base: BadRequestResponse("invalid product_id: "+err.Error())}, nil
	}

	p, err := h.getHandler.Handle(ctx, appproduct.GetCommand{ID: productID})
	if err != nil {
		return &financev1.GetProductResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &financev1.GetProductResponse{
		Base: successResponse("product retrieved successfully"),
		Data: productToProto(p),
	}, nil
}

// ListProducts lists products with filtering and pagination.
func (h *ProductGRPCHandler) ListProducts(ctx context.Context, req *financev1.ListProductsRequest) (*financev1.ListProductsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.ListProductsResponse{Base: baseResp}, nil
	}

	page, pageSize := paginationFromProto(req.GetPagination())

	query := appproduct.ListQuery{
		Search:         req.GetSearch(),
		WorkflowStatus: req.GetWorkflowStatus(),
		ProductStatus:  req.GetProductStatus(),
		Purpose:        req.GetPurpose(),
		Page:           page,
		PageSize:       pageSize,
	}

	if deptID := req.GetCreatedByDeptId(); deptID != "" {
		id, err := uuid.Parse(deptID)
		if err != nil {
			return &financev1.ListProductsResponse{Base: BadRequestResponse("invalid created_by_dept_id: "+err.Error())}, nil
		}
		query.CreatedByDeptID = &id
	}

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		return &financev1.ListProductsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*financev1.Product, len(result.Products))
	for i, prod := range result.Products {
		items[i] = productToProto(prod)
	}

	return &financev1.ListProductsResponse{
		Base: successResponse("products retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  int64(result.TotalItems),
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// UpdateProduct updates an existing product.
func (h *ProductGRPCHandler) UpdateProduct(ctx context.Context, req *financev1.UpdateProductRequest) (*financev1.UpdateProductResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateProductResponse{Base: baseResp}, nil
	}

	productID, err := uuid.Parse(req.GetProductId())
	if err != nil {
		return &financev1.UpdateProductResponse{Base: BadRequestResponse("invalid product_id: "+err.Error())}, nil
	}

	p, err := h.updateHandler.Handle(ctx, appproduct.UpdateCommand{
		ID:        productID,
		Name:      req.GetProductName(),
		ShadeCode: req.GetProductShadeCode(),
		ShadeName: req.GetProductShadeName(),
		Purpose:   req.GetPurpose(),
		UpdatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.UpdateProductResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &financev1.UpdateProductResponse{
		Base: successResponse("product updated successfully"),
		Data: productToProto(p),
	}, nil
}

// DeleteProduct soft-deletes a product.
func (h *ProductGRPCHandler) DeleteProduct(ctx context.Context, req *financev1.DeleteProductRequest) (*financev1.DeleteProductResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.DeleteProductResponse{Base: baseResp}, nil
	}

	productID, err := uuid.Parse(req.GetProductId())
	if err != nil {
		return &financev1.DeleteProductResponse{Base: BadRequestResponse("invalid product_id: "+err.Error())}, nil
	}

	if err := h.deleteHandler.Handle(ctx, appproduct.DeleteCommand{
		ID:        productID,
		DeletedBy: getUserFromContext(ctx),
	}); err != nil {
		return &financev1.DeleteProductResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &financev1.DeleteProductResponse{
		Base: successResponse("product deleted successfully"),
	}, nil
}

// DuplicateProduct creates a new product by cloning an existing one.
func (h *ProductGRPCHandler) DuplicateProduct(ctx context.Context, req *financev1.DuplicateProductRequest) (*financev1.DuplicateProductResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.DuplicateProductResponse{Base: baseResp}, nil
	}

	sourceID, err := uuid.Parse(req.GetSourceProductId())
	if err != nil {
		return &financev1.DuplicateProductResponse{Base: BadRequestResponse("invalid source_product_id: "+err.Error())}, nil
	}

	var requestID uuid.UUID
	if rid := req.GetCurrentRequestId(); rid != "" {
		requestID, err = uuid.Parse(rid)
		if err != nil {
			return &financev1.DuplicateProductResponse{Base: BadRequestResponse("invalid current_request_id: "+err.Error())}, nil
		}
	}

	p, err := h.duplicateHandler.Handle(ctx, appproduct.DuplicateCommand{
		SourceID:         sourceID,
		NewCode:          req.GetProductCode(),
		NewName:          req.GetProductName(),
		DuplicationNote:  req.GetDuplicationNote(),
		Options:          copyOptionsFromProto(req.GetOptions()),
		CurrentRequestID: requestID,
		CreatedBy:        getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.DuplicateProductResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &financev1.DuplicateProductResponse{
		Base: successResponse("product duplicated successfully"),
		Data: productToProto(p),
	}, nil
}

// ListProductsByRequest lists products linked to a specific product request UUID.
// This is a thin pass-through to the repository's ListByRequestID method, with pagination.
func (h *ProductGRPCHandler) ListProductsByRequest(ctx context.Context, req *financev1.ListProductsByRequestRequest) (*financev1.ListProductsByRequestResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.ListProductsByRequestResponse{Base: baseResp}, nil
	}

	requestID, err := uuid.Parse(req.GetRequestId())
	if err != nil {
		return &financev1.ListProductsByRequestResponse{Base: BadRequestResponse("invalid request_id: "+err.Error())}, nil
	}

	page, pageSize := paginationFromProto(req.GetPagination())

	items, total, err := h.productRepo.ListByRequestID(ctx, requestID, page, pageSize)
	if err != nil {
		return &financev1.ListProductsByRequestResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	protoItems := make([]*financev1.Product, len(items))
	for i, prod := range items {
		protoItems[i] = productToProto(prod)
	}

	totalPages := int32(0)
	if pageSize > 0 && total > 0 {
		totalPages = safeconv.IntToInt32((total + pageSize - 1) / pageSize)
	}

	return &financev1.ListProductsByRequestResponse{
		Base: successResponse("products retrieved successfully"),
		Data: protoItems,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: safeconv.IntToInt32(page),
			PageSize:    safeconv.IntToInt32(pageSize),
			TotalItems:  int64(total),
			TotalPages:  totalPages,
		},
	}, nil
}

// =============================================================================
// Mapping helpers
// =============================================================================

// productToProto converts a domain product entity to its proto representation.
func productToProto(p *domainproduct.Product) *financev1.Product {
	proto := &financev1.Product{
		ProductId:         p.ID().String(),
		ProductCode:       p.Code().String(),
		ProductName:       p.Name().String(),
		ProductItemCode:   p.ItemCode().String(),
		ProductShadeCode:  p.ShadeCode().String(),
		ProductShadeName:  p.ShadeName().String(),
		ProductStatus:     string(p.ProductStatus()),
		WorkflowStatus:    string(p.WorkflowStatus()),
		CreatedByDeptId:   p.CreatedByDeptID().String(),
		CreatedByDeptCode: p.CreatedByDeptCode(),
		Purpose:           string(p.Purpose()),
		DuplicatedFromId:  p.DuplicatedFromID().String(),
		DuplicationNote:   p.DuplicationNote(),
		CopiedWithOptions: copyOptionsToProto(p.CopiedWithOptions()),
		Audit: &commonv1.AuditInfo{
			CreatedAt: p.CreatedAt().Format(time.RFC3339),
			CreatedBy: p.CreatedBy(),
		},
	}

	// Omit zero UUID fields in proto output.
	if p.DuplicatedFromID() == [16]byte{} {
		proto.DuplicatedFromId = ""
	}

	if p.CurrentRequestID() != [16]byte{} {
		proto.CurrentRequestId = p.CurrentRequestID().String()
	}

	if p.UpdatedAt() != nil {
		proto.Audit.UpdatedAt = p.UpdatedAt().Format(time.RFC3339)
	}
	if p.UpdatedBy() != "" {
		proto.Audit.UpdatedBy = p.UpdatedBy()
	}

	return proto
}

// copyOptionsToProto converts domain CopyOptions to proto CopyOptions. Nil-safe.
func copyOptionsToProto(opts *domainproduct.CopyOptions) *financev1.CopyOptions {
	if opts == nil {
		return nil
	}
	return &financev1.CopyOptions{
		IncludeValues:      opts.IncludeValues,
		IncludeRouting:     opts.IncludeRouting,
		IncludeRm:          opts.IncludeRM,
		IncludeAttachments: opts.IncludeAttachments,
	}
}

// copyOptionsFromProto converts proto CopyOptions to domain CopyOptions. Nil-safe.
func copyOptionsFromProto(opts *financev1.CopyOptions) domainproduct.CopyOptions {
	if opts == nil {
		return domainproduct.CopyOptions{}
	}
	return domainproduct.CopyOptions{
		IncludeValues:      opts.GetIncludeValues(),
		IncludeRouting:     opts.GetIncludeRouting(),
		IncludeRM:          opts.GetIncludeRm(),
		IncludeAttachments: opts.GetIncludeAttachments(),
	}
}

// paginationFromProto extracts page and pageSize from a proto PaginationRequest.
// Returns sensible defaults when the message is nil or fields are zero.
func paginationFromProto(p *commonv1.PaginationRequest) (page, pageSize int) {
	page = 1
	pageSize = 10
	if p == nil {
		return
	}
	if p.GetPage() > 0 {
		page = int(p.GetPage())
	}
	if p.GetPageSize() > 0 {
		pageSize = int(p.GetPageSize())
	}
	return
}
