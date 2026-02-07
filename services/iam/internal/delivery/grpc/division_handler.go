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

// DivisionHandler handles division-related gRPC requests.
type DivisionHandler struct {
	iamv1.UnimplementedDivisionServiceServer
	repo             organization.DivisionRepository
	validationHelper *ValidationHelper
}

// NewDivisionHandler creates a new DivisionHandler.
func NewDivisionHandler(repo organization.DivisionRepository, validationHelper *ValidationHelper) *DivisionHandler {
	return &DivisionHandler{repo: repo, validationHelper: validationHelper}
}

// CreateDivision handles the gRPC request to create a new division.
func (h *DivisionHandler) CreateDivision(ctx context.Context, req *iamv1.CreateDivisionRequest) (*iamv1.CreateDivisionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateDivisionResponse{Base: baseResp}, nil
	}

	companyID, err := uuid.Parse(req.GetCompanyId())
	if err != nil {
		return &iamv1.CreateDivisionResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid company ID"),
		}, nil
	}

	exists, err := h.repo.ExistsByCode(ctx, req.GetDivisionCode())
	if err != nil {
		return &iamv1.CreateDivisionResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to check existing division"),
		}, nil
	}
	if exists {
		return &iamv1.CreateDivisionResponse{
			Base: ConflictResponse("Division code already exists"),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	division, err := organization.NewDivision(companyID, req.GetDivisionCode(), req.GetDivisionName(), req.GetDescription(), userID)
	if err != nil {
		return &iamv1.CreateDivisionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.repo.Create(ctx, division); err != nil {
		return &iamv1.CreateDivisionResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to create division"),
		}, nil
	}

	return &iamv1.CreateDivisionResponse{
		Base: &commonv1.BaseResponse{IsSuccess: true, StatusCode: "201", Message: "Division created successfully"},
		Data: toDivisionProto(division),
	}, nil
}

// GetDivision handles the gRPC request to retrieve a division by ID.
func (h *DivisionHandler) GetDivision(ctx context.Context, req *iamv1.GetDivisionRequest) (*iamv1.GetDivisionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetDivisionResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetDivisionId())
	if err != nil {
		return &iamv1.GetDivisionResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid division ID"),
		}, nil
	}

	division, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.GetDivisionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.GetDivisionResponse{
		Base: SuccessResponse("Division retrieved successfully"),
		Data: toDivisionProto(division),
	}, nil
}

// UpdateDivision handles the gRPC request to update an existing division.
func (h *DivisionHandler) UpdateDivision(ctx context.Context, req *iamv1.UpdateDivisionRequest) (*iamv1.UpdateDivisionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateDivisionResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetDivisionId())
	if err != nil {
		return &iamv1.UpdateDivisionResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid division ID"),
		}, nil
	}

	division, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.UpdateDivisionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	if err := division.Update(req.DivisionName, req.Description, req.IsActive, userID); err != nil {
		return &iamv1.UpdateDivisionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.repo.Update(ctx, division); err != nil {
		return &iamv1.UpdateDivisionResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to update division"),
		}, nil
	}

	return &iamv1.UpdateDivisionResponse{
		Base: SuccessResponse("Division updated successfully"),
		Data: toDivisionProto(division),
	}, nil
}

// DeleteDivision handles the gRPC request to delete a division.
func (h *DivisionHandler) DeleteDivision(ctx context.Context, req *iamv1.DeleteDivisionRequest) (*iamv1.DeleteDivisionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteDivisionResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetDivisionId())
	if err != nil {
		return &iamv1.DeleteDivisionResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid division ID"),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	if err := h.repo.Delete(ctx, id, userID); err != nil {
		return &iamv1.DeleteDivisionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.DeleteDivisionResponse{
		Base: SuccessResponse("Division deleted successfully"),
	}, nil
}

// ListDivisions handles the gRPC request to list divisions with pagination.
func (h *DivisionHandler) ListDivisions(ctx context.Context, req *iamv1.ListDivisionsRequest) (*iamv1.ListDivisionsResponse, error) {
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

	var companyID *uuid.UUID
	if req.GetCompanyId() != "" {
		cid, err := uuid.Parse(req.GetCompanyId())
		if err == nil {
			companyID = &cid
		}
	}

	params := organization.DivisionListParams{
		ListParams: organization.ListParams{
			Page:      page,
			PageSize:  pageSize,
			Search:    req.GetSearch(),
			IsActive:  isActive,
			SortBy:    req.GetSortBy(),
			SortOrder: req.GetSortOrder(),
		},
		CompanyID: companyID,
	}

	divisions, total, err := h.repo.List(ctx, params)
	if err != nil {
		return &iamv1.ListDivisionsResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to list divisions"),
		}, nil
	}

	totalPages := int32(0)
	if total > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &iamv1.ListDivisionsResponse{
		Base:       SuccessResponse("Divisions listed successfully"),
		Data:       toDivisionProtos(divisions),
		Pagination: &commonv1.PaginationResponse{CurrentPage: safeconv.IntToInt32(page), PageSize: safeconv.IntToInt32(pageSize), TotalItems: total, TotalPages: totalPages},
	}, nil
}

// ExportDivisions handles the gRPC request to export divisions.
func (h *DivisionHandler) ExportDivisions(_ context.Context, _ *iamv1.ExportDivisionsRequest) (*iamv1.ExportDivisionsResponse, error) {
	return &iamv1.ExportDivisionsResponse{
		Base: ErrorResponse("501", "Export not implemented"),
	}, nil
}

// ImportDivisions handles the gRPC request to import divisions.
func (h *DivisionHandler) ImportDivisions(_ context.Context, _ *iamv1.ImportDivisionsRequest) (*iamv1.ImportDivisionsResponse, error) {
	return &iamv1.ImportDivisionsResponse{
		Base: ErrorResponse("501", "Import not implemented"),
	}, nil
}

// DownloadDivisionTemplate handles the gRPC request to download the division import template.
func (h *DivisionHandler) DownloadDivisionTemplate(_ context.Context, _ *iamv1.DownloadDivisionTemplateRequest) (*iamv1.DownloadDivisionTemplateResponse, error) {
	return &iamv1.DownloadDivisionTemplateResponse{
		Base: ErrorResponse("501", "Template download not implemented"),
	}, nil
}

func toDivisionProto(d *organization.Division) *iamv1.Division {
	if d == nil {
		return nil
	}
	return &iamv1.Division{
		DivisionId:   d.ID().String(),
		CompanyId:    d.CompanyID().String(),
		DivisionCode: d.Code(),
		DivisionName: d.Name(),
		Description:  d.Description(),
		IsActive:     d.IsActive(),
		Audit:        toAuditProto(d.Audit()),
	}
}

func toDivisionProtos(divisions []*organization.Division) []*iamv1.Division {
	result := make([]*iamv1.Division, len(divisions))
	for i, d := range divisions {
		result[i] = toDivisionProto(d)
	}
	return result
}
