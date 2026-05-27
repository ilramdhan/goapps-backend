// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"context"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	cmapp "github.com/mutugading/goapps-backend/services/iam/internal/application/companymapping"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/companymapping"
)

// CompanyMappingHandler implements the CompanyMappingService gRPC service.
type CompanyMappingHandler struct {
	iamv1.UnimplementedCompanyMappingServiceServer
	createHandler    *cmapp.CreateHandler
	getHandler       *cmapp.GetHandler
	updateHandler    *cmapp.UpdateHandler
	deleteHandler    *cmapp.DeleteHandler
	listHandler      *cmapp.ListHandler
	validationHelper *ValidationHelper
}

// NewCompanyMappingHandler creates a new CompanyMappingHandler.
func NewCompanyMappingHandler(repo companymapping.Repository, validationHelper *ValidationHelper) *CompanyMappingHandler {
	return &CompanyMappingHandler{
		createHandler:    cmapp.NewCreateHandler(repo),
		getHandler:       cmapp.NewGetHandler(repo),
		updateHandler:    cmapp.NewUpdateHandler(repo),
		deleteHandler:    cmapp.NewDeleteHandler(repo),
		listHandler:      cmapp.NewListHandler(repo),
		validationHelper: validationHelper,
	}
}

// CreateCompanyMapping creates a new company mapping.
func (h *CompanyMappingHandler) CreateCompanyMapping(ctx context.Context, req *iamv1.CreateCompanyMappingRequest) (*iamv1.CreateCompanyMappingResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateCompanyMappingResponse{Base: baseResp}, nil
	}

	cmd := cmapp.CreateCommand{
		Code:         req.GetCode(),
		Name:         req.GetName(),
		CompanyID:    req.GetCompanyId(),
		DivisionID:   req.GetDivisionId(),
		DepartmentID: req.GetDepartmentId(),
		CreatedBy:    getUserFromCtx(ctx),
	}
	if req.SectionId != nil {
		cmd.SectionID = req.SectionId
	}

	entity, err := h.createHandler.Handle(ctx, cmd)
	if err != nil {
		return &iamv1.CreateCompanyMappingResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.CreateCompanyMappingResponse{
		Base: &commonv1.BaseResponse{
			IsSuccess:  true,
			StatusCode: "201",
			Message:    "Company mapping created successfully",
		},
		Data: toCompanyMappingProto(entity),
	}, nil
}

// GetCompanyMapping retrieves a mapping by ID.
func (h *CompanyMappingHandler) GetCompanyMapping(ctx context.Context, req *iamv1.GetCompanyMappingRequest) (*iamv1.GetCompanyMappingResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetCompanyMappingResponse{Base: baseResp}, nil
	}

	entity, err := h.getHandler.Handle(ctx, cmapp.GetQuery{CompanyMappingID: req.GetCompanyMappingId()})
	if err != nil {
		return &iamv1.GetCompanyMappingResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &iamv1.GetCompanyMappingResponse{
		Base: SuccessResponse("Company mapping retrieved successfully"),
		Data: toCompanyMappingProto(entity),
	}, nil
}

// UpdateCompanyMapping updates an existing mapping.
func (h *CompanyMappingHandler) UpdateCompanyMapping(ctx context.Context, req *iamv1.UpdateCompanyMappingRequest) (*iamv1.UpdateCompanyMappingResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateCompanyMappingResponse{Base: baseResp}, nil
	}

	cmd := cmapp.UpdateCommand{
		CompanyMappingID: req.GetCompanyMappingId(),
		Name:             req.Name,
		CompanyID:        req.CompanyId,
		DivisionID:       req.DivisionId,
		DepartmentID:     req.DepartmentId,
		IsActive:         req.IsActive,
		UpdatedBy:        getUserFromCtx(ctx),
	}
	// section_id rules: if proto field is unset → no change; if set to empty → clear; if set to UUID → assign.
	if req.SectionId != nil {
		if *req.SectionId == "" {
			cmd.ClearSection = true
		} else {
			cmd.SectionID = req.SectionId
		}
	}

	entity, err := h.updateHandler.Handle(ctx, cmd)
	if err != nil {
		return &iamv1.UpdateCompanyMappingResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &iamv1.UpdateCompanyMappingResponse{
		Base: SuccessResponse("Company mapping updated successfully"),
		Data: toCompanyMappingProto(entity),
	}, nil
}

// DeleteCompanyMapping soft-deletes a mapping.
func (h *CompanyMappingHandler) DeleteCompanyMapping(ctx context.Context, req *iamv1.DeleteCompanyMappingRequest) (*iamv1.DeleteCompanyMappingResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteCompanyMappingResponse{Base: baseResp}, nil
	}
	if err := h.deleteHandler.Handle(ctx, cmapp.DeleteCommand{
		CompanyMappingID: req.GetCompanyMappingId(),
		DeletedBy:        getUserFromCtx(ctx),
	}); err != nil {
		return &iamv1.DeleteCompanyMappingResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &iamv1.DeleteCompanyMappingResponse{
		Base: SuccessResponse("Company mapping deleted successfully"),
	}, nil
}

// ListCompanyMappings lists mappings.
func (h *CompanyMappingHandler) ListCompanyMappings(ctx context.Context, req *iamv1.ListCompanyMappingsRequest) (*iamv1.ListCompanyMappingsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ListCompanyMappingsResponse{Base: baseResp}, nil
	}

	query := cmapp.ListQuery{
		Page:         int(req.GetPage()),
		PageSize:     int(req.GetPageSize()),
		Search:       req.GetSearch(),
		CompanyID:    parseOptionalUUID(req.CompanyId),
		DivisionID:   parseOptionalUUID(req.DivisionId),
		DepartmentID: parseOptionalUUID(req.DepartmentId),
		SectionID:    parseOptionalUUID(req.SectionId),
		IsActive:     activeFilterToBool(req.GetActiveFilter()),
		SortBy:       req.GetSortBy(),
		SortOrder:    req.GetSortOrder(),
	}

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		return &iamv1.ListCompanyMappingsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	protoItems := make([]*iamv1.CompanyMapping, len(result.Items))
	for i, m := range result.Items {
		protoItems[i] = toCompanyMappingProto(m)
	}

	return &iamv1.ListCompanyMappingsResponse{
		Base: SuccessResponse("Company mappings retrieved successfully"),
		Data: protoItems,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// toCompanyMappingProto converts the domain entity to its proto representation.
func toCompanyMappingProto(m *companymapping.CompanyMapping) *iamv1.CompanyMapping {
	h := m.Hierarchy()
	proto := &iamv1.CompanyMapping{
		CompanyMappingId: m.ID().String(),
		Code:             m.Code().String(),
		Name:             m.Name().String(),
		CompanyId:        h.CompanyID.String(),
		CompanyCode:      h.CompanyCode,
		CompanyName:      h.CompanyName,
		DivisionId:       h.DivisionID.String(),
		DivisionCode:     h.DivisionCode,
		DivisionName:     h.DivisionName,
		DepartmentId:     h.DepartmentID.String(),
		DepartmentCode:   h.DepartmentCode,
		DepartmentName:   h.DepartmentName,
		SectionCode:      h.SectionCode,
		SectionName:      h.SectionName,
		IsActive:         m.IsActive(),
		Audit:            toAuditProto(m.Audit()),
	}
	if h.SectionID != nil {
		s := h.SectionID.String()
		proto.SectionId = &s
	}
	return proto
}
