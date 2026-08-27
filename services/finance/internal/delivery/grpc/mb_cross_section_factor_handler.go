package grpc

import (
	"context"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbcrosssection "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcrosssection"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
)

// MBCrossSectionFactorHandler implements financev1.MbCrossSectionFactorServiceServer.
type MBCrossSectionFactorHandler struct {
	financev1.UnimplementedMbCrossSectionFactorServiceServer
	createHandler *appmbcrosssection.FactorCreateHandler
	getHandler    *appmbcrosssection.FactorGetHandler
	listHandler   *appmbcrosssection.FactorListHandler
	updateHandler *appmbcrosssection.FactorUpdateHandler
	deleteHandler *appmbcrosssection.FactorDeleteHandler
	validation    *ValidationHelper
}

// NewMBCrossSectionFactorHandler constructs an MBCrossSectionFactorHandler.
func NewMBCrossSectionFactorHandler(repo mbcrosssection.FactorRepository) (*MBCrossSectionFactorHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MBCrossSectionFactorHandler{
		createHandler: appmbcrosssection.NewFactorCreateHandler(repo),
		getHandler:    appmbcrosssection.NewFactorGetHandler(repo),
		listHandler:   appmbcrosssection.NewFactorListHandler(repo),
		updateHandler: appmbcrosssection.NewFactorUpdateHandler(repo),
		deleteHandler: appmbcrosssection.NewFactorDeleteHandler(repo),
		validation:    v,
	}, nil
}

// CreateMbCrossSectionFactor creates a new cross-section conversion factor.
func (h *MBCrossSectionFactorHandler) CreateMbCrossSectionFactor(ctx context.Context, req *financev1.CreateMbCrossSectionFactorRequest) (*financev1.CreateMbCrossSectionFactorResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBCrossSectionFactorOperation("create", false)
		return &financev1.CreateMbCrossSectionFactorResponse{Base: baseResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appmbcrosssection.FactorCreateCommand{
		FromCode:  req.FromCode,
		ToCode:    req.ToCode,
		Factor:    req.Factor,
		Operation: req.Operation,
		Note:      req.Note,
		IsActive:  req.IsActive,
		CreatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBCrossSectionFactorOperation("create", false)
		return &financev1.CreateMbCrossSectionFactorResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBCrossSectionFactorOperation("create", true)
	return &financev1.CreateMbCrossSectionFactorResponse{
		Base: successResponse("MB cross section factor created successfully"),
		Data: mbCrossSectionFactorEntityToProto(entity),
	}, nil
}

// GetMbCrossSectionFactor retrieves a cross-section conversion factor by ID.
func (h *MBCrossSectionFactorHandler) GetMbCrossSectionFactor(ctx context.Context, req *financev1.GetMbCrossSectionFactorRequest) (*financev1.GetMbCrossSectionFactorResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBCrossSectionFactorOperation("get", false)
		return &financev1.GetMbCrossSectionFactorResponse{Base: baseResp}, nil
	}

	entity, err := h.getHandler.Handle(ctx, appmbcrosssection.FactorGetQuery{ID: req.MbcfId})
	if err != nil {
		RecordMBCrossSectionFactorOperation("get", false)
		return &financev1.GetMbCrossSectionFactorResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBCrossSectionFactorOperation("get", true)
	return &financev1.GetMbCrossSectionFactorResponse{
		Base: successResponse("MB cross section factor retrieved successfully"),
		Data: mbCrossSectionFactorEntityToProto(entity),
	}, nil
}

// UpdateMbCrossSectionFactor updates an existing cross-section conversion factor.
func (h *MBCrossSectionFactorHandler) UpdateMbCrossSectionFactor(ctx context.Context, req *financev1.UpdateMbCrossSectionFactorRequest) (*financev1.UpdateMbCrossSectionFactorResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBCrossSectionFactorOperation("update", false)
		return &financev1.UpdateMbCrossSectionFactorResponse{Base: baseResp}, nil
	}

	entity, err := h.updateHandler.Handle(ctx, appmbcrosssection.FactorUpdateCommand{
		ID:        req.MbcfId,
		Factor:    req.Factor,
		Operation: req.Operation,
		Note:      req.Note,
		IsActive:  req.IsActive,
		UpdatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBCrossSectionFactorOperation("update", false)
		return &financev1.UpdateMbCrossSectionFactorResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBCrossSectionFactorOperation("update", true)
	return &financev1.UpdateMbCrossSectionFactorResponse{
		Base: successResponse("MB cross section factor updated successfully"),
		Data: mbCrossSectionFactorEntityToProto(entity),
	}, nil
}

// DeleteMbCrossSectionFactor soft-deletes a cross-section conversion factor.
func (h *MBCrossSectionFactorHandler) DeleteMbCrossSectionFactor(ctx context.Context, req *financev1.DeleteMbCrossSectionFactorRequest) (*financev1.DeleteMbCrossSectionFactorResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBCrossSectionFactorOperation("delete", false)
		return &financev1.DeleteMbCrossSectionFactorResponse{Base: baseResp}, nil
	}

	if err := h.deleteHandler.Handle(ctx, appmbcrosssection.FactorDeleteCommand{
		ID:        req.MbcfId,
		DeletedBy: getUserFromContext(ctx),
	}); err != nil {
		RecordMBCrossSectionFactorOperation("delete", false)
		return &financev1.DeleteMbCrossSectionFactorResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBCrossSectionFactorOperation("delete", true)
	return &financev1.DeleteMbCrossSectionFactorResponse{Base: successResponse("MB cross section factor deleted successfully")}, nil
}

// ListMbCrossSectionFactor lists cross-section conversion factors with search, sort, filter and pagination.
func (h *MBCrossSectionFactorHandler) ListMbCrossSectionFactor(ctx context.Context, req *financev1.ListMbCrossSectionFactorRequest) (*financev1.ListMbCrossSectionFactorResponse, error) {
	result, err := h.listHandler.Handle(ctx, appmbcrosssection.FactorListQuery{
		Page:      req.Page,
		PageSize:  req.PageSize,
		Search:    req.Search,
		FromCode:  req.FromCode,
		ToCode:    req.ToCode,
		SortBy:    req.SortBy,
		SortOrder: req.SortDir,
		IsActive:  activeFilterToBoolPtr(req.ActiveFilter),
	})
	if err != nil {
		RecordMBCrossSectionFactorOperation("list", false)
		return &financev1.ListMbCrossSectionFactorResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBCrossSectionFactorOperation("list", true)

	items := make([]*financev1.MbCrossSectionFactor, len(result.Items))
	for i, e := range result.Items {
		items[i] = mbCrossSectionFactorEntityToProto(e)
	}

	return &financev1.ListMbCrossSectionFactorResponse{
		Base: successResponse("MB cross section factors retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// mbCrossSectionFactorEntityToProto converts a domain FactorEntity to its proto representation.
func mbCrossSectionFactorEntityToProto(e *mbcrosssection.FactorEntity) *financev1.MbCrossSectionFactor {
	return &financev1.MbCrossSectionFactor{
		MbcfId:    e.ID(),
		FromCode:  e.FromCode(),
		ToCode:    e.ToCode(),
		Factor:    e.Factor(),
		Operation: e.Operation(),
		Note:      e.Note(),
		IsActive:  e.IsActive(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt(),
			CreatedBy: e.CreatedBy(),
			UpdatedAt: e.UpdatedAt(),
			UpdatedBy: e.UpdatedBy(),
		},
	}
}
