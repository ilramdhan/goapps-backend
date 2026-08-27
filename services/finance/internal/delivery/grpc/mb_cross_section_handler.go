package grpc

import (
	"context"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbcrosssection "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcrosssection"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
)

// MBCrossSectionHandler implements financev1.MbCrossSectionServiceServer.
type MBCrossSectionHandler struct {
	financev1.UnimplementedMbCrossSectionServiceServer
	createHandler *appmbcrosssection.CreateHandler
	getHandler    *appmbcrosssection.GetHandler
	listHandler   *appmbcrosssection.ListHandler
	updateHandler *appmbcrosssection.UpdateHandler
	deleteHandler *appmbcrosssection.DeleteHandler
	validation    *ValidationHelper
}

// NewMBCrossSectionHandler constructs an MBCrossSectionHandler.
func NewMBCrossSectionHandler(repo mbcrosssection.Repository) (*MBCrossSectionHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MBCrossSectionHandler{
		createHandler: appmbcrosssection.NewCreateHandler(repo),
		getHandler:    appmbcrosssection.NewGetHandler(repo),
		listHandler:   appmbcrosssection.NewListHandler(repo),
		updateHandler: appmbcrosssection.NewUpdateHandler(repo),
		deleteHandler: appmbcrosssection.NewDeleteHandler(repo),
		validation:    v,
	}, nil
}

// CreateMbCrossSection creates a new MB cross-section master record.
func (h *MBCrossSectionHandler) CreateMbCrossSection(ctx context.Context, req *financev1.CreateMbCrossSectionRequest) (*financev1.CreateMbCrossSectionResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBCrossSectionOperation("create", false)
		return &financev1.CreateMbCrossSectionResponse{Base: baseResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appmbcrosssection.CreateCommand{
		Code:         req.Code,
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		DisplayOrder: req.DisplayOrder,
		IsActive:     req.IsActive,
		CreatedBy:    getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBCrossSectionOperation("create", false)
		return &financev1.CreateMbCrossSectionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBCrossSectionOperation("create", true)
	return &financev1.CreateMbCrossSectionResponse{
		Base: successResponse("MB cross section created successfully"),
		Data: mbCrossSectionEntityToProto(entity),
	}, nil
}

// GetMbCrossSection retrieves an MB cross-section master record by ID.
func (h *MBCrossSectionHandler) GetMbCrossSection(ctx context.Context, req *financev1.GetMbCrossSectionRequest) (*financev1.GetMbCrossSectionResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBCrossSectionOperation("get", false)
		return &financev1.GetMbCrossSectionResponse{Base: baseResp}, nil
	}

	entity, err := h.getHandler.Handle(ctx, appmbcrosssection.GetQuery{ID: req.MbcsId})
	if err != nil {
		RecordMBCrossSectionOperation("get", false)
		return &financev1.GetMbCrossSectionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBCrossSectionOperation("get", true)
	return &financev1.GetMbCrossSectionResponse{
		Base: successResponse("MB cross section retrieved successfully"),
		Data: mbCrossSectionEntityToProto(entity),
	}, nil
}

// UpdateMbCrossSection updates an existing MB cross-section master record.
func (h *MBCrossSectionHandler) UpdateMbCrossSection(ctx context.Context, req *financev1.UpdateMbCrossSectionRequest) (*financev1.UpdateMbCrossSectionResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBCrossSectionOperation("update", false)
		return &financev1.UpdateMbCrossSectionResponse{Base: baseResp}, nil
	}

	entity, err := h.updateHandler.Handle(ctx, appmbcrosssection.UpdateCommand{
		ID:           req.MbcsId,
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		DisplayOrder: req.DisplayOrder,
		IsActive:     req.IsActive,
		UpdatedBy:    getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBCrossSectionOperation("update", false)
		return &financev1.UpdateMbCrossSectionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBCrossSectionOperation("update", true)
	return &financev1.UpdateMbCrossSectionResponse{
		Base: successResponse("MB cross section updated successfully"),
		Data: mbCrossSectionEntityToProto(entity),
	}, nil
}

// DeleteMbCrossSection soft-deletes an MB cross-section master record.
func (h *MBCrossSectionHandler) DeleteMbCrossSection(ctx context.Context, req *financev1.DeleteMbCrossSectionRequest) (*financev1.DeleteMbCrossSectionResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBCrossSectionOperation("delete", false)
		return &financev1.DeleteMbCrossSectionResponse{Base: baseResp}, nil
	}

	if err := h.deleteHandler.Handle(ctx, appmbcrosssection.DeleteCommand{
		ID:        req.MbcsId,
		DeletedBy: getUserFromContext(ctx),
	}); err != nil {
		RecordMBCrossSectionOperation("delete", false)
		return &financev1.DeleteMbCrossSectionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBCrossSectionOperation("delete", true)
	return &financev1.DeleteMbCrossSectionResponse{Base: successResponse("MB cross section deleted successfully")}, nil
}

// ListMbCrossSection lists MB cross-section master records with search, sort, filter and pagination.
func (h *MBCrossSectionHandler) ListMbCrossSection(ctx context.Context, req *financev1.ListMbCrossSectionRequest) (*financev1.ListMbCrossSectionResponse, error) {
	result, err := h.listHandler.Handle(ctx, appmbcrosssection.ListQuery{
		Page:      req.Page,
		PageSize:  req.PageSize,
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortDir,
		IsActive:  activeFilterToBoolPtr(req.ActiveFilter),
	})
	if err != nil {
		RecordMBCrossSectionOperation("list", false)
		return &financev1.ListMbCrossSectionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBCrossSectionOperation("list", true)

	items := make([]*financev1.MbCrossSection, len(result.Items))
	for i, e := range result.Items {
		items[i] = mbCrossSectionEntityToProto(e)
	}

	return &financev1.ListMbCrossSectionResponse{
		Base: successResponse("MB cross sections retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// activeFilterToBoolPtr maps the proto ActiveFilter enum to the optional bool used by
// the application layer. UNSPECIFIED means "no filter" and yields nil.
func activeFilterToBoolPtr(f financev1.ActiveFilter) *bool {
	switch f {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		t := true
		return &t
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		fa := false
		return &fa
	case financev1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
		return nil
	default:
		return nil
	}
}

// mbCrossSectionEntityToProto converts a domain mbcrosssection Entity to its proto representation.
func mbCrossSectionEntityToProto(e *mbcrosssection.Entity) *financev1.MbCrossSection {
	return &financev1.MbCrossSection{
		MbcsId:       e.ID(),
		Code:         e.Code(),
		DisplayName:  e.DisplayName(),
		Description:  e.Description(),
		IsActive:     e.IsActive(),
		DisplayOrder: e.DisplayOrder(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt(),
			CreatedBy: e.CreatedBy(),
			UpdatedAt: e.UpdatedAt(),
			UpdatedBy: e.UpdatedBy(),
		},
	}
}
