// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"bytes"
	"context"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/cms"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/storage"
	"github.com/mutugading/goapps-backend/services/iam/pkg/safeconv"
)

// =============================================================================
// CMS PAGE HANDLER
// =============================================================================

// CMSPageHandler handles CMS page-related gRPC requests.
type CMSPageHandler struct {
	iamv1.UnimplementedCMSPageServiceServer
	repo             cms.PageRepository
	validationHelper *ValidationHelper
}

// NewCMSPageHandler creates a new CMSPageHandler.
func NewCMSPageHandler(repo cms.PageRepository, validationHelper *ValidationHelper) *CMSPageHandler {
	return &CMSPageHandler{repo: repo, validationHelper: validationHelper}
}

// CreateCMSPage handles the gRPC request to create a new CMS page.
func (h *CMSPageHandler) CreateCMSPage(ctx context.Context, req *iamv1.CreateCMSPageRequest) (*iamv1.CreateCMSPageResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateCMSPageResponse{Base: baseResp}, nil
	}

	exists, err := h.repo.ExistsBySlug(ctx, req.GetPageSlug())
	if err != nil {
		return &iamv1.CreateCMSPageResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to check existing page"),
		}, nil
	}
	if exists {
		return &iamv1.CreateCMSPageResponse{
			Base: ConflictResponse("Page slug already exists"),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	page, err := cms.NewPage(req.GetPageSlug(), req.GetPageTitle(), req.GetPageContent(),
		req.GetMetaDescription(), req.GetIsPublished(), int(req.GetSortOrder()), userID)
	if err != nil {
		return &iamv1.CreateCMSPageResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.repo.Create(ctx, page); err != nil {
		return &iamv1.CreateCMSPageResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to create page"),
		}, nil
	}

	return &iamv1.CreateCMSPageResponse{
		Base: &commonv1.BaseResponse{IsSuccess: true, StatusCode: "201", Message: "CMS page created successfully"},
		Data: toCMSPageProto(page),
	}, nil
}

// GetCMSPage handles the gRPC request to retrieve a CMS page by ID.
func (h *CMSPageHandler) GetCMSPage(ctx context.Context, req *iamv1.GetCMSPageRequest) (*iamv1.GetCMSPageResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetCMSPageResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetPageId())
	if err != nil {
		return &iamv1.GetCMSPageResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid page ID"),
		}, nil
	}

	page, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.GetCMSPageResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.GetCMSPageResponse{
		Base: SuccessResponse("CMS page retrieved successfully"),
		Data: toCMSPageProto(page),
	}, nil
}

// GetCMSPageBySlug handles the gRPC request to retrieve a CMS page by slug.
// This is a public endpoint — no authentication required.
func (h *CMSPageHandler) GetCMSPageBySlug(ctx context.Context, req *iamv1.GetCMSPageBySlugRequest) (*iamv1.GetCMSPageBySlugResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetCMSPageBySlugResponse{Base: baseResp}, nil
	}

	page, err := h.repo.GetBySlug(ctx, req.GetSlug())
	if err != nil {
		return &iamv1.GetCMSPageBySlugResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.GetCMSPageBySlugResponse{
		Base: SuccessResponse("CMS page retrieved successfully"),
		Data: toCMSPageProto(page),
	}, nil
}

// UpdateCMSPage handles the gRPC request to update a CMS page.
func (h *CMSPageHandler) UpdateCMSPage(ctx context.Context, req *iamv1.UpdateCMSPageRequest) (*iamv1.UpdateCMSPageResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateCMSPageResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetPageId())
	if err != nil {
		return &iamv1.UpdateCMSPageResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid page ID"),
		}, nil
	}

	page, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.UpdateCMSPageResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	var sortOrder *int
	if req.SortOrder != nil {
		so := int(*req.SortOrder)
		sortOrder = &so
	}
	if err := page.Update(req.PageTitle, req.PageContent, req.MetaDescription, req.IsPublished, sortOrder, userID); err != nil {
		return &iamv1.UpdateCMSPageResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.repo.Update(ctx, page); err != nil {
		return &iamv1.UpdateCMSPageResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to update page"),
		}, nil
	}

	return &iamv1.UpdateCMSPageResponse{
		Base: SuccessResponse("CMS page updated successfully"),
		Data: toCMSPageProto(page),
	}, nil
}

// DeleteCMSPage handles the gRPC request to delete a CMS page.
func (h *CMSPageHandler) DeleteCMSPage(ctx context.Context, req *iamv1.DeleteCMSPageRequest) (*iamv1.DeleteCMSPageResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteCMSPageResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetPageId())
	if err != nil {
		return &iamv1.DeleteCMSPageResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid page ID"),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	if err := h.repo.Delete(ctx, id, userID); err != nil {
		return &iamv1.DeleteCMSPageResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.DeleteCMSPageResponse{
		Base: SuccessResponse("CMS page deleted successfully"),
	}, nil
}

// ListCMSPages handles the gRPC request to list CMS pages.
func (h *CMSPageHandler) ListCMSPages(ctx context.Context, req *iamv1.ListCMSPagesRequest) (*iamv1.ListCMSPagesResponse, error) {
	page := int(req.GetPage())
	pageSize := int(req.GetPageSize())
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}

	params := cms.PageListParams{
		Page:        page,
		PageSize:    pageSize,
		Search:      req.GetSearch(),
		IsPublished: req.IsPublished,
		SortBy:      req.GetSortBy(),
		SortOrder:   req.GetSortOrder(),
	}

	pages, total, err := h.repo.List(ctx, params)
	if err != nil {
		return &iamv1.ListCMSPagesResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to list pages"),
		}, nil
	}

	totalPages := int32(0)
	if total > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &iamv1.ListCMSPagesResponse{
		Base:       SuccessResponse("CMS pages listed successfully"),
		Data:       toCMSPageProtos(pages),
		Pagination: &commonv1.PaginationResponse{CurrentPage: safeconv.IntToInt32(page), PageSize: safeconv.IntToInt32(pageSize), TotalItems: total, TotalPages: totalPages},
	}, nil
}

// =============================================================================
// CMS SECTION HANDLER
// =============================================================================

// CMSSectionHandler handles CMS section-related gRPC requests.
type CMSSectionHandler struct {
	iamv1.UnimplementedCMSSectionServiceServer
	sectionRepo      cms.SectionRepository
	settingRepo      cms.SettingRepository
	storageService   storage.Service
	validationHelper *ValidationHelper
}

// NewCMSSectionHandler creates a new CMSSectionHandler.
func NewCMSSectionHandler(sectionRepo cms.SectionRepository, settingRepo cms.SettingRepository, storageSvc storage.Service, validationHelper *ValidationHelper) *CMSSectionHandler {
	return &CMSSectionHandler{sectionRepo: sectionRepo, settingRepo: settingRepo, storageService: storageSvc, validationHelper: validationHelper}
}

// CreateCMSSection handles the gRPC request to create a new CMS section.
func (h *CMSSectionHandler) CreateCMSSection(ctx context.Context, req *iamv1.CreateCMSSectionRequest) (*iamv1.CreateCMSSectionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateCMSSectionResponse{Base: baseResp}, nil
	}

	exists, err := h.sectionRepo.ExistsByKey(ctx, req.GetSectionKey())
	if err != nil {
		return &iamv1.CreateCMSSectionResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to check existing section"),
		}, nil
	}
	if exists {
		return &iamv1.CreateCMSSectionResponse{
			Base: ConflictResponse("Section key already exists"),
		}, nil
	}

	sectionType := protoToSectionType(req.GetSectionType())
	userID := getUserFromCtx(ctx)

	section, err := cms.NewSection(sectionType, req.GetSectionKey(), req.GetTitle(),
		req.GetSubtitle(), req.GetContent(), req.GetIconName(), req.GetImageUrl(),
		req.GetButtonText(), req.GetButtonUrl(), int(req.GetSortOrder()),
		req.GetIsPublished(), req.GetMetadata(), userID)
	if err != nil {
		return &iamv1.CreateCMSSectionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.sectionRepo.Create(ctx, section); err != nil {
		return &iamv1.CreateCMSSectionResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to create section"),
		}, nil
	}

	return &iamv1.CreateCMSSectionResponse{
		Base: &commonv1.BaseResponse{IsSuccess: true, StatusCode: "201", Message: "CMS section created successfully"},
		Data: toCMSSectionProto(section),
	}, nil
}

// GetCMSSection handles the gRPC request to retrieve a CMS section by ID.
func (h *CMSSectionHandler) GetCMSSection(ctx context.Context, req *iamv1.GetCMSSectionRequest) (*iamv1.GetCMSSectionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetCMSSectionResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetSectionId())
	if err != nil {
		return &iamv1.GetCMSSectionResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid section ID"),
		}, nil
	}

	section, err := h.sectionRepo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.GetCMSSectionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.GetCMSSectionResponse{
		Base: SuccessResponse("CMS section retrieved successfully"),
		Data: toCMSSectionProto(section),
	}, nil
}

// UpdateCMSSection handles the gRPC request to update a CMS section.
func (h *CMSSectionHandler) UpdateCMSSection(ctx context.Context, req *iamv1.UpdateCMSSectionRequest) (*iamv1.UpdateCMSSectionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateCMSSectionResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetSectionId())
	if err != nil {
		return &iamv1.UpdateCMSSectionResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid section ID"),
		}, nil
	}

	section, err := h.sectionRepo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.UpdateCMSSectionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	var sectionType *cms.SectionType
	if req.SectionType != nil {
		st := protoToSectionType(*req.SectionType)
		sectionType = &st
	}
	var sortOrder *int
	if req.SortOrder != nil {
		so := int(*req.SortOrder)
		sortOrder = &so
	}

	if err := section.Update(sectionType, req.Title, req.Subtitle, req.Content,
		req.IconName, req.ImageUrl, req.ButtonText, req.ButtonUrl,
		sortOrder, req.IsPublished, req.Metadata, userID); err != nil {
		return &iamv1.UpdateCMSSectionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.sectionRepo.Update(ctx, section); err != nil {
		return &iamv1.UpdateCMSSectionResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to update section"),
		}, nil
	}

	return &iamv1.UpdateCMSSectionResponse{
		Base: SuccessResponse("CMS section updated successfully"),
		Data: toCMSSectionProto(section),
	}, nil
}

// DeleteCMSSection handles the gRPC request to delete a CMS section.
func (h *CMSSectionHandler) DeleteCMSSection(ctx context.Context, req *iamv1.DeleteCMSSectionRequest) (*iamv1.DeleteCMSSectionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteCMSSectionResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetSectionId())
	if err != nil {
		return &iamv1.DeleteCMSSectionResponse{ //nolint:nilerr // error returned in response body
			Base: ErrorResponse("400", "Invalid section ID"),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	if err := h.sectionRepo.Delete(ctx, id, userID); err != nil {
		return &iamv1.DeleteCMSSectionResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.DeleteCMSSectionResponse{
		Base: SuccessResponse("CMS section deleted successfully"),
	}, nil
}

// ListCMSSections handles the gRPC request to list CMS sections.
func (h *CMSSectionHandler) ListCMSSections(ctx context.Context, req *iamv1.ListCMSSectionsRequest) (*iamv1.ListCMSSectionsResponse, error) {
	page := int(req.GetPage())
	pageSize := int(req.GetPageSize())
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}

	var sectionType *cms.SectionType
	if req.GetSectionType() != iamv1.CMSSectionType_CMS_SECTION_TYPE_UNSPECIFIED {
		st := protoToSectionType(req.GetSectionType())
		sectionType = &st
	}

	params := cms.SectionListParams{
		Page:        page,
		PageSize:    pageSize,
		Search:      req.GetSearch(),
		SectionType: sectionType,
		IsPublished: req.IsPublished,
		SortBy:      req.GetSortBy(),
		SortOrder:   req.GetSortOrder(),
	}

	sections, total, err := h.sectionRepo.List(ctx, params)
	if err != nil {
		return &iamv1.ListCMSSectionsResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to list sections"),
		}, nil
	}

	totalPages := int32(0)
	if total > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &iamv1.ListCMSSectionsResponse{
		Base:       SuccessResponse("CMS sections listed successfully"),
		Data:       toCMSSectionProtos(sections),
		Pagination: &commonv1.PaginationResponse{CurrentPage: safeconv.IntToInt32(page), PageSize: safeconv.IntToInt32(pageSize), TotalItems: total, TotalPages: totalPages},
	}, nil
}

// GetPublicLandingContent handles the gRPC request to fetch all published landing page content.
// This is a public endpoint — no authentication required.
func (h *CMSSectionHandler) GetPublicLandingContent(ctx context.Context, _ *iamv1.GetPublicLandingContentRequest) (*iamv1.GetPublicLandingContentResponse, error) {
	sections, err := h.sectionRepo.ListPublished(ctx)
	if err != nil {
		return &iamv1.GetPublicLandingContentResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to fetch landing content"),
		}, nil
	}

	settings, err := h.settingRepo.ListAll(ctx)
	if err != nil {
		return &iamv1.GetPublicLandingContentResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to fetch site settings"),
		}, nil
	}

	return &iamv1.GetPublicLandingContentResponse{
		Base:     SuccessResponse("Landing content retrieved successfully"),
		Sections: toCMSSectionProtos(sections),
		Settings: toCMSSettingProtos(settings),
	}, nil
}

// UploadCMSImage handles the gRPC request to upload a CMS image to MinIO.
func (h *CMSSectionHandler) UploadCMSImage(ctx context.Context, req *iamv1.UploadCMSImageRequest) (*iamv1.UploadCMSImageResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UploadCMSImageResponse{Base: baseResp}, nil
	}

	if h.storageService == nil {
		return &iamv1.UploadCMSImageResponse{
			Base: InternalErrorResponse("Storage service not configured"),
		}, nil
	}

	reader := bytes.NewReader(req.GetFileData())
	imageURL, err := h.storageService.UploadCMSImage(
		ctx, req.GetFolder(), req.GetFileName(), reader,
		int64(len(req.GetFileData())), req.GetContentType(),
	)
	if err != nil {
		return &iamv1.UploadCMSImageResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to upload image"),
		}, nil
	}

	return &iamv1.UploadCMSImageResponse{
		Base:     SuccessResponse("Image uploaded successfully"),
		ImageUrl: imageURL,
	}, nil
}

// =============================================================================
// CMS SETTING HANDLER
// =============================================================================

// CMSSettingHandler handles CMS setting-related gRPC requests.
type CMSSettingHandler struct {
	iamv1.UnimplementedCMSSettingServiceServer
	repo             cms.SettingRepository
	validationHelper *ValidationHelper
}

// NewCMSSettingHandler creates a new CMSSettingHandler.
func NewCMSSettingHandler(repo cms.SettingRepository, validationHelper *ValidationHelper) *CMSSettingHandler {
	return &CMSSettingHandler{repo: repo, validationHelper: validationHelper}
}

// GetCMSSetting handles the gRPC request to retrieve a CMS setting by key.
func (h *CMSSettingHandler) GetCMSSetting(ctx context.Context, req *iamv1.GetCMSSettingRequest) (*iamv1.GetCMSSettingResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetCMSSettingResponse{Base: baseResp}, nil
	}

	setting, err := h.repo.GetByKey(ctx, req.GetSettingKey())
	if err != nil {
		return &iamv1.GetCMSSettingResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	return &iamv1.GetCMSSettingResponse{
		Base: SuccessResponse("CMS setting retrieved successfully"),
		Data: toCMSSettingProto(setting),
	}, nil
}

// UpdateCMSSetting handles the gRPC request to update a CMS setting.
func (h *CMSSettingHandler) UpdateCMSSetting(ctx context.Context, req *iamv1.UpdateCMSSettingRequest) (*iamv1.UpdateCMSSettingResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateCMSSettingResponse{Base: baseResp}, nil
	}

	setting, err := h.repo.GetByKey(ctx, req.GetSettingKey())
	if err != nil {
		return &iamv1.UpdateCMSSettingResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	userID := getUserFromCtx(ctx)
	if err := setting.UpdateValue(req.GetSettingValue(), userID); err != nil {
		return &iamv1.UpdateCMSSettingResponse{ //nolint:nilerr // error returned in response body
			Base: domainErrorToBaseResponse(err),
		}, nil
	}

	if err := h.repo.Update(ctx, setting); err != nil {
		return &iamv1.UpdateCMSSettingResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to update setting"),
		}, nil
	}

	return &iamv1.UpdateCMSSettingResponse{
		Base: SuccessResponse("CMS setting updated successfully"),
		Data: toCMSSettingProto(setting),
	}, nil
}

// ListCMSSettings handles the gRPC request to list CMS settings.
func (h *CMSSettingHandler) ListCMSSettings(ctx context.Context, req *iamv1.ListCMSSettingsRequest) (*iamv1.ListCMSSettingsResponse, error) {
	var settings []*cms.Setting
	var err error

	if req.GetSettingGroup() != "" {
		settings, err = h.repo.List(ctx, req.GetSettingGroup())
	} else {
		settings, err = h.repo.ListAll(ctx)
	}
	if err != nil {
		return &iamv1.ListCMSSettingsResponse{ //nolint:nilerr // error returned in response body
			Base: InternalErrorResponse("Failed to list settings"),
		}, nil
	}

	return &iamv1.ListCMSSettingsResponse{
		Base: SuccessResponse("CMS settings listed successfully"),
		Data: toCMSSettingProtos(settings),
	}, nil
}

// BulkUpdateCMSSettings handles the gRPC request to update multiple settings.
func (h *CMSSettingHandler) BulkUpdateCMSSettings(ctx context.Context, req *iamv1.BulkUpdateCMSSettingsRequest) (*iamv1.BulkUpdateCMSSettingsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.BulkUpdateCMSSettingsResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)
	updated := make([]*cms.Setting, 0, len(req.GetSettings()))

	for _, update := range req.GetSettings() {
		setting, err := h.repo.GetByKey(ctx, update.GetSettingKey())
		if err != nil {
			return &iamv1.BulkUpdateCMSSettingsResponse{ //nolint:nilerr // error returned in response body
				Base: domainErrorToBaseResponse(err),
			}, nil
		}

		if err := setting.UpdateValue(update.GetSettingValue(), userID); err != nil {
			return &iamv1.BulkUpdateCMSSettingsResponse{ //nolint:nilerr // error returned in response body
				Base: domainErrorToBaseResponse(err),
			}, nil
		}

		if err := h.repo.Update(ctx, setting); err != nil {
			return &iamv1.BulkUpdateCMSSettingsResponse{ //nolint:nilerr // error returned in response body
				Base: InternalErrorResponse("Failed to update setting: " + update.GetSettingKey()),
			}, nil
		}

		updated = append(updated, setting)
	}

	return &iamv1.BulkUpdateCMSSettingsResponse{
		Base: SuccessResponse("CMS settings updated successfully"),
		Data: toCMSSettingProtos(updated),
	}, nil
}

// =============================================================================
// PROTO CONVERSION HELPERS
// =============================================================================

func toCMSPageProto(p *cms.Page) *iamv1.CMSPage {
	if p == nil {
		return nil
	}
	proto := &iamv1.CMSPage{
		PageId:          p.ID().String(),
		PageSlug:        p.Slug(),
		PageTitle:       p.Title(),
		PageContent:     p.Content(),
		MetaDescription: p.MetaDescription(),
		IsPublished:     p.IsPublished(),
		SortOrder:       safeconv.IntToInt32(p.SortOrder()),
		Audit:           toAuditProto(p.Audit()),
	}
	if p.PublishedAt() != nil {
		proto.PublishedAt = p.PublishedAt().Format("2006-01-02T15:04:05Z07:00")
	}
	return proto
}

func toCMSPageProtos(pages []*cms.Page) []*iamv1.CMSPage {
	result := make([]*iamv1.CMSPage, len(pages))
	for i, p := range pages {
		result[i] = toCMSPageProto(p)
	}
	return result
}

func toCMSSectionProto(s *cms.Section) *iamv1.CMSSection {
	if s == nil {
		return nil
	}
	return &iamv1.CMSSection{
		SectionId:   s.ID().String(),
		SectionType: sectionTypeToProto(s.Type()),
		SectionKey:  s.Key(),
		Title:       s.Title(),
		Subtitle:    s.Subtitle(),
		Content:     s.Content(),
		IconName:    s.IconName(),
		ImageUrl:    s.ImageURL(),
		ButtonText:  s.ButtonText(),
		ButtonUrl:   s.ButtonURL(),
		SortOrder:   safeconv.IntToInt32(s.SortOrder()),
		IsPublished: s.IsPublished(),
		Metadata:    s.Metadata(),
		Audit:       toAuditProto(s.Audit()),
	}
}

func toCMSSectionProtos(sections []*cms.Section) []*iamv1.CMSSection {
	result := make([]*iamv1.CMSSection, len(sections))
	for i, s := range sections {
		result[i] = toCMSSectionProto(s)
	}
	return result
}

func toCMSSettingProto(s *cms.Setting) *iamv1.CMSSetting {
	if s == nil {
		return nil
	}
	return &iamv1.CMSSetting{
		SettingId:    s.ID().String(),
		SettingKey:   s.Key(),
		SettingValue: s.Value(),
		SettingType:  settingTypeToProto(s.Type()),
		SettingGroup: s.Group(),
		Description:  s.Description(),
		IsEditable:   s.IsEditable(),
		Audit:        toAuditProto(s.Audit()),
	}
}

func toCMSSettingProtos(settings []*cms.Setting) []*iamv1.CMSSetting {
	result := make([]*iamv1.CMSSetting, len(settings))
	for i, s := range settings {
		result[i] = toCMSSettingProto(s)
	}
	return result
}

// protoToSectionType converts proto enum to domain type.
func protoToSectionType(pt iamv1.CMSSectionType) cms.SectionType {
	switch pt {
	case iamv1.CMSSectionType_CMS_SECTION_TYPE_HERO:
		return cms.SectionTypeHero
	case iamv1.CMSSectionType_CMS_SECTION_TYPE_FEATURE:
		return cms.SectionTypeFeature
	case iamv1.CMSSectionType_CMS_SECTION_TYPE_FAQ:
		return cms.SectionTypeFAQ
	case iamv1.CMSSectionType_CMS_SECTION_TYPE_TESTIMONIAL:
		return cms.SectionTypeTestimonial
	case iamv1.CMSSectionType_CMS_SECTION_TYPE_CTA:
		return cms.SectionTypeCTA
	default:
		return cms.SectionTypeCustom
	}
}

// sectionTypeToProto converts domain type to proto enum.
func sectionTypeToProto(st cms.SectionType) iamv1.CMSSectionType {
	switch st {
	case cms.SectionTypeHero:
		return iamv1.CMSSectionType_CMS_SECTION_TYPE_HERO
	case cms.SectionTypeFeature:
		return iamv1.CMSSectionType_CMS_SECTION_TYPE_FEATURE
	case cms.SectionTypeFAQ:
		return iamv1.CMSSectionType_CMS_SECTION_TYPE_FAQ
	case cms.SectionTypeTestimonial:
		return iamv1.CMSSectionType_CMS_SECTION_TYPE_TESTIMONIAL
	case cms.SectionTypeCTA:
		return iamv1.CMSSectionType_CMS_SECTION_TYPE_CTA
	case cms.SectionTypeCustom:
		return iamv1.CMSSectionType_CMS_SECTION_TYPE_CUSTOM
	default:
		return iamv1.CMSSectionType_CMS_SECTION_TYPE_UNSPECIFIED
	}
}

// settingTypeToProto converts domain type to proto enum.
func settingTypeToProto(st cms.SettingType) iamv1.CMSSettingType {
	switch st {
	case cms.SettingTypeText:
		return iamv1.CMSSettingType_CMS_SETTING_TYPE_TEXT
	case cms.SettingTypeRichText:
		return iamv1.CMSSettingType_CMS_SETTING_TYPE_RICH_TEXT
	case cms.SettingTypeImage:
		return iamv1.CMSSettingType_CMS_SETTING_TYPE_IMAGE
	case cms.SettingTypeURL:
		return iamv1.CMSSettingType_CMS_SETTING_TYPE_URL
	case cms.SettingTypeJSON:
		return iamv1.CMSSettingType_CMS_SETTING_TYPE_JSON
	default:
		return iamv1.CMSSettingType_CMS_SETTING_TYPE_UNSPECIFIED
	}
}
