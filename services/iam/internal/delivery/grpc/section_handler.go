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

// SectionHandler handles section-related gRPC requests.
type SectionHandler struct {
	iamv1.UnimplementedSectionServiceServer
	repo             organization.SectionRepository
	validationHelper *ValidationHelper
}

// NewSectionHandler creates a new SectionHandler.
func NewSectionHandler(repo organization.SectionRepository, validationHelper *ValidationHelper) *SectionHandler {
	return &SectionHandler{repo: repo, validationHelper: validationHelper}
}

// CreateSection handles the gRPC request to create a new section.
func (h *SectionHandler) CreateSection(ctx context.Context, req *iamv1.CreateSectionRequest) (*iamv1.CreateSectionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateSectionResponse{Base: baseResp}, nil
	}

	departmentID, err := uuid.Parse(req.GetDepartmentId())
	if err != nil {
		return &iamv1.CreateSectionResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid department ID"),
		}, nil
	}

	exists, err := h.repo.ExistsByCode(ctx, req.GetSectionCode())
	if err != nil {
		return &iamv1.CreateSectionResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to check existing section"),
		}, nil
	}
	if exists {
		return &iamv1.CreateSectionResponse{
			Base: ConflictResponse("Section code already exists"),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	section, err := organization.NewSection(departmentID, req.GetSectionCode(), req.GetSectionName(), req.GetDescription(), userID)
	if err != nil {
		return &iamv1.CreateSectionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.repo.Create(ctx, section); err != nil {
		return &iamv1.CreateSectionResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to create section"),
		}, nil
	}

	return &iamv1.CreateSectionResponse{
		Base: &commonv1.BaseResponse{IsSuccess: true, StatusCode: "201", Message: "Section created successfully"},
		Data: toSectionProto(section),
	}, nil
}

// GetSection handles the gRPC request to retrieve a section by ID.
func (h *SectionHandler) GetSection(ctx context.Context, req *iamv1.GetSectionRequest) (*iamv1.GetSectionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetSectionResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetSectionId())
	if err != nil {
		return &iamv1.GetSectionResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid section ID"),
		}, nil
	}

	section, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.GetSectionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.GetSectionResponse{
		Base: SuccessResponse("Section retrieved successfully"),
		Data: toSectionProto(section),
	}, nil
}

// UpdateSection handles the gRPC request to update an existing section.
func (h *SectionHandler) UpdateSection(ctx context.Context, req *iamv1.UpdateSectionRequest) (*iamv1.UpdateSectionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateSectionResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetSectionId())
	if err != nil {
		return &iamv1.UpdateSectionResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid section ID"),
		}, nil
	}

	section, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.UpdateSectionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	if err := section.Update(req.SectionName, req.Description, req.IsActive, userID); err != nil {
		return &iamv1.UpdateSectionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.repo.Update(ctx, section); err != nil {
		return &iamv1.UpdateSectionResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to update section"),
		}, nil
	}

	return &iamv1.UpdateSectionResponse{
		Base: SuccessResponse("Section updated successfully"),
		Data: toSectionProto(section),
	}, nil
}

// DeleteSection handles the gRPC request to delete a section.
func (h *SectionHandler) DeleteSection(ctx context.Context, req *iamv1.DeleteSectionRequest) (*iamv1.DeleteSectionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteSectionResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetSectionId())
	if err != nil {
		return &iamv1.DeleteSectionResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid section ID"),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	if err := h.repo.Delete(ctx, id, userID); err != nil {
		return &iamv1.DeleteSectionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.DeleteSectionResponse{
		Base: SuccessResponse("Section deleted successfully"),
	}, nil
}

// ListSections handles the gRPC request to list sections with pagination.
func (h *SectionHandler) ListSections(ctx context.Context, req *iamv1.ListSectionsRequest) (*iamv1.ListSectionsResponse, error) {
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
		// No filter — return all.
	}

	var departmentID, divisionID, companyID *uuid.UUID
	if req.GetDepartmentId() != "" {
		did, err := uuid.Parse(req.GetDepartmentId())
		if err == nil {
			departmentID = &did
		}
	}
	if req.GetDivisionId() != "" {
		did, err := uuid.Parse(req.GetDivisionId())
		if err == nil {
			divisionID = &did
		}
	}
	if req.GetCompanyId() != "" {
		cid, err := uuid.Parse(req.GetCompanyId())
		if err == nil {
			companyID = &cid
		}
	}

	params := organization.SectionListParams{
		ListParams: organization.ListParams{
			Page:      page,
			PageSize:  pageSize,
			Search:    req.GetSearch(),
			IsActive:  isActive,
			SortBy:    req.GetSortBy(),
			SortOrder: req.GetSortOrder(),
		},
		DepartmentID: departmentID,
		DivisionID:   divisionID,
		CompanyID:    companyID,
	}

	sections, total, err := h.repo.List(ctx, params)
	if err != nil {
		return &iamv1.ListSectionsResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to list sections"),
		}, nil
	}

	totalPages := int32(0)
	if total > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &iamv1.ListSectionsResponse{
		Base:       SuccessResponse("Sections listed successfully"),
		Data:       toSectionProtos(sections),
		Pagination: &commonv1.PaginationResponse{CurrentPage: safeconv.IntToInt32(page), PageSize: safeconv.IntToInt32(pageSize), TotalItems: total, TotalPages: totalPages},
	}, nil
}

// ExportSections handles the gRPC request to export sections.
func (h *SectionHandler) ExportSections(_ context.Context, _ *iamv1.ExportSectionsRequest) (*iamv1.ExportSectionsResponse, error) {
	return &iamv1.ExportSectionsResponse{
		Base: ErrorResponse("501", "Export not implemented"),
	}, nil
}

// ImportSections handles the gRPC request to import sections.
func (h *SectionHandler) ImportSections(_ context.Context, _ *iamv1.ImportSectionsRequest) (*iamv1.ImportSectionsResponse, error) {
	return &iamv1.ImportSectionsResponse{
		Base: ErrorResponse("501", "Import not implemented"),
	}, nil
}

// DownloadSectionTemplate handles the gRPC request to download the section import template.
func (h *SectionHandler) DownloadSectionTemplate(_ context.Context, _ *iamv1.DownloadSectionTemplateRequest) (*iamv1.DownloadSectionTemplateResponse, error) {
	return &iamv1.DownloadSectionTemplateResponse{
		Base: ErrorResponse("501", "Template download not implemented"),
	}, nil
}

func toSectionProto(s *organization.Section) *iamv1.Section {
	if s == nil {
		return nil
	}
	return &iamv1.Section{
		SectionId:    s.ID().String(),
		DepartmentId: s.DepartmentID().String(),
		SectionCode:  s.Code(),
		SectionName:  s.Name(),
		Description:  s.Description(),
		IsActive:     s.IsActive(),
		Audit:        toAuditProto(s.Audit()),
	}
}

func toSectionProtos(sections []*organization.Section) []*iamv1.Section {
	result := make([]*iamv1.Section, len(sections))
	for i, s := range sections {
		result[i] = toSectionProto(s)
	}
	return result
}
