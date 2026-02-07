// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"context"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/organization"
	"github.com/mutugading/goapps-backend/services/iam/pkg/safeconv"
)

// getUserFromCtx extracts the user ID from context, falling back to "system".
func getUserFromCtx(ctx context.Context) string {
	if id, err := getUserIDFromContext(ctx); err == nil {
		return id.String()
	}
	return "system"
}

// CompanyHandler handles company-related gRPC requests.
type CompanyHandler struct {
	iamv1.UnimplementedCompanyServiceServer
	repo             organization.CompanyRepository
	validationHelper *ValidationHelper
}

// NewCompanyHandler creates a new CompanyHandler.
func NewCompanyHandler(repo organization.CompanyRepository, validationHelper *ValidationHelper) *CompanyHandler {
	return &CompanyHandler{repo: repo, validationHelper: validationHelper}
}

// CreateCompany handles the gRPC request to create a new company.
func (h *CompanyHandler) CreateCompany(ctx context.Context, req *iamv1.CreateCompanyRequest) (*iamv1.CreateCompanyResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateCompanyResponse{Base: baseResp}, nil
	}

	// Check for existing code
	exists, err := h.repo.ExistsByCode(ctx, req.GetCompanyCode())
	if err != nil {
		return &iamv1.CreateCompanyResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to check existing company"),
		}, nil
	}
	if exists {
		return &iamv1.CreateCompanyResponse{
			Base: ConflictResponse("Company code already exists"),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	company, err := organization.NewCompany(req.GetCompanyCode(), req.GetCompanyName(), req.GetDescription(), userID)
	if err != nil {
		return &iamv1.CreateCompanyResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.repo.Create(ctx, company); err != nil {
		return &iamv1.CreateCompanyResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to create company"),
		}, nil
	}

	return &iamv1.CreateCompanyResponse{
		Base: &commonv1.BaseResponse{IsSuccess: true, StatusCode: "201", Message: "Company created successfully"},
		Data: toCompanyProto(company),
	}, nil
}

// GetCompany handles the gRPC request to retrieve a company by ID.
func (h *CompanyHandler) GetCompany(ctx context.Context, req *iamv1.GetCompanyRequest) (*iamv1.GetCompanyResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetCompanyResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetCompanyId())
	if err != nil {
		return &iamv1.GetCompanyResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid company ID"),
		}, nil
	}

	company, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.GetCompanyResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.GetCompanyResponse{
		Base: SuccessResponse("Company retrieved successfully"),
		Data: toCompanyProto(company),
	}, nil
}

// UpdateCompany handles the gRPC request to update an existing company.
func (h *CompanyHandler) UpdateCompany(ctx context.Context, req *iamv1.UpdateCompanyRequest) (*iamv1.UpdateCompanyResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateCompanyResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetCompanyId())
	if err != nil {
		return &iamv1.UpdateCompanyResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid company ID"),
		}, nil
	}

	company, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.UpdateCompanyResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	if err := company.Update(req.CompanyName, req.Description, req.IsActive, userID); err != nil {
		return &iamv1.UpdateCompanyResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.repo.Update(ctx, company); err != nil {
		return &iamv1.UpdateCompanyResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to update company"),
		}, nil
	}

	return &iamv1.UpdateCompanyResponse{
		Base: SuccessResponse("Company updated successfully"),
		Data: toCompanyProto(company),
	}, nil
}

// DeleteCompany handles the gRPC request to delete a company.
func (h *CompanyHandler) DeleteCompany(ctx context.Context, req *iamv1.DeleteCompanyRequest) (*iamv1.DeleteCompanyResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteCompanyResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetCompanyId())
	if err != nil {
		return &iamv1.DeleteCompanyResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid company ID"),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	if err := h.repo.Delete(ctx, id, userID); err != nil {
		return &iamv1.DeleteCompanyResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.DeleteCompanyResponse{
		Base: SuccessResponse("Company deleted successfully"),
	}, nil
}

// ListCompanies handles the gRPC request to list companies with pagination.
func (h *CompanyHandler) ListCompanies(ctx context.Context, req *iamv1.ListCompaniesRequest) (*iamv1.ListCompaniesResponse, error) {
	page := int(req.GetPage())
	pageSize := int(req.GetPageSize())
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}

	var isActive *bool
	switch req.GetActiveFilter() {
	case iamv1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		active := true
		isActive = &active
	case iamv1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		inactive := false
		isActive = &inactive
	case iamv1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
		// No filter — return all
	}

	params := organization.ListParams{
		Page:      page,
		PageSize:  pageSize,
		Search:    req.GetSearch(),
		IsActive:  isActive,
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	}

	companies, total, err := h.repo.List(ctx, params)
	if err != nil {
		return &iamv1.ListCompaniesResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to list companies"),
		}, nil
	}

	totalPages := int32(0)
	if total > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &iamv1.ListCompaniesResponse{
		Base:       SuccessResponse("Companies listed successfully"),
		Data:       toCompanyProtos(companies),
		Pagination: &commonv1.PaginationResponse{CurrentPage: safeconv.IntToInt32(page), PageSize: safeconv.IntToInt32(pageSize), TotalItems: total, TotalPages: totalPages},
	}, nil
}

// ExportCompanies handles the gRPC request to export companies.
func (h *CompanyHandler) ExportCompanies(_ context.Context, _ *iamv1.ExportCompaniesRequest) (*iamv1.ExportCompaniesResponse, error) {
	// TODO: Implement Excel export.
	return &iamv1.ExportCompaniesResponse{
		Base: ErrorResponse("501", "Export not implemented"),
	}, nil
}

// ImportCompanies handles the gRPC request to import companies.
func (h *CompanyHandler) ImportCompanies(_ context.Context, _ *iamv1.ImportCompaniesRequest) (*iamv1.ImportCompaniesResponse, error) {
	// TODO: Implement Excel import.
	return &iamv1.ImportCompaniesResponse{
		Base: ErrorResponse("501", "Import not implemented"),
	}, nil
}

// DownloadCompanyTemplate handles the gRPC request to download the company import template.
func (h *CompanyHandler) DownloadCompanyTemplate(_ context.Context, _ *iamv1.DownloadCompanyTemplateRequest) (*iamv1.DownloadCompanyTemplateResponse, error) {
	// TODO: Implement template download.
	return &iamv1.DownloadCompanyTemplateResponse{
		Base: ErrorResponse("501", "Template download not implemented"),
	}, nil
}

// Helper functions

func toCompanyProto(c *organization.Company) *iamv1.Company {
	if c == nil {
		return nil
	}
	return &iamv1.Company{
		CompanyId:   c.ID().String(),
		CompanyCode: c.Code(),
		CompanyName: c.Name(),
		Description: c.Description(),
		IsActive:    c.IsActive(),
		Audit:       toAuditProto(c.Audit()),
	}
}

func toCompanyProtos(companies []*organization.Company) []*iamv1.Company {
	result := make([]*iamv1.Company, len(companies))
	for i, c := range companies {
		result[i] = toCompanyProto(c)
	}
	return result
}
