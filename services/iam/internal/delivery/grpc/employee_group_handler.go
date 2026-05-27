// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"context"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	egapp "github.com/mutugading/goapps-backend/services/iam/internal/application/employeegroup"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeegroup"
)

// EmployeeGroupHandler implements the EmployeeGroupService gRPC service.
type EmployeeGroupHandler struct {
	iamv1.UnimplementedEmployeeGroupServiceServer
	createHandler    *egapp.CreateHandler
	getHandler       *egapp.GetHandler
	updateHandler    *egapp.UpdateHandler
	deleteHandler    *egapp.DeleteHandler
	listHandler      *egapp.ListHandler
	exportHandler    *egapp.ExportHandler
	importHandler    *egapp.ImportHandler
	templateHandler  *egapp.TemplateHandler
	validationHelper *ValidationHelper
}

// NewEmployeeGroupHandler creates a new EmployeeGroupHandler.
func NewEmployeeGroupHandler(repo employeegroup.Repository, validationHelper *ValidationHelper) *EmployeeGroupHandler {
	return &EmployeeGroupHandler{
		createHandler:    egapp.NewCreateHandler(repo),
		getHandler:       egapp.NewGetHandler(repo),
		updateHandler:    egapp.NewUpdateHandler(repo),
		deleteHandler:    egapp.NewDeleteHandler(repo),
		listHandler:      egapp.NewListHandler(repo),
		exportHandler:    egapp.NewExportHandler(repo),
		importHandler:    egapp.NewImportHandler(repo),
		templateHandler:  egapp.NewTemplateHandler(),
		validationHelper: validationHelper,
	}
}

// CreateEmployeeGroup creates a new employee group.
func (h *EmployeeGroupHandler) CreateEmployeeGroup(ctx context.Context, req *iamv1.CreateEmployeeGroupRequest) (*iamv1.CreateEmployeeGroupResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateEmployeeGroupResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	entity, err := h.createHandler.Handle(ctx, egapp.CreateCommand{
		Code:      req.GetCode(),
		Name:      req.GetName(),
		CreatedBy: userID,
	})
	if err != nil {
		return &iamv1.CreateEmployeeGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.CreateEmployeeGroupResponse{
		Base: &commonv1.BaseResponse{
			IsSuccess:  true,
			StatusCode: "201",
			Message:    "Employee group created successfully",
		},
		Data: toEmployeeGroupProto(entity),
	}, nil
}

// GetEmployeeGroup retrieves an employee group by ID.
func (h *EmployeeGroupHandler) GetEmployeeGroup(ctx context.Context, req *iamv1.GetEmployeeGroupRequest) (*iamv1.GetEmployeeGroupResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetEmployeeGroupResponse{Base: baseResp}, nil
	}

	entity, err := h.getHandler.Handle(ctx, egapp.GetQuery{
		EmployeeGroupID: req.GetEmployeeGroupId(),
	})
	if err != nil {
		return &iamv1.GetEmployeeGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.GetEmployeeGroupResponse{
		Base: SuccessResponse("Employee group retrieved successfully"),
		Data: toEmployeeGroupProto(entity),
	}, nil
}

// UpdateEmployeeGroup updates an employee group.
func (h *EmployeeGroupHandler) UpdateEmployeeGroup(ctx context.Context, req *iamv1.UpdateEmployeeGroupRequest) (*iamv1.UpdateEmployeeGroupResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateEmployeeGroupResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	entity, err := h.updateHandler.Handle(ctx, egapp.UpdateCommand{
		EmployeeGroupID: req.GetEmployeeGroupId(),
		Name:            req.Name,
		IsActive:        req.IsActive,
		UpdatedBy:       userID,
	})
	if err != nil {
		return &iamv1.UpdateEmployeeGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.UpdateEmployeeGroupResponse{
		Base: SuccessResponse("Employee group updated successfully"),
		Data: toEmployeeGroupProto(entity),
	}, nil
}

// DeleteEmployeeGroup soft-deletes an employee group.
func (h *EmployeeGroupHandler) DeleteEmployeeGroup(ctx context.Context, req *iamv1.DeleteEmployeeGroupRequest) (*iamv1.DeleteEmployeeGroupResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteEmployeeGroupResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	if err := h.deleteHandler.Handle(ctx, egapp.DeleteCommand{
		EmployeeGroupID: req.GetEmployeeGroupId(),
		DeletedBy:       userID,
	}); err != nil {
		return &iamv1.DeleteEmployeeGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.DeleteEmployeeGroupResponse{
		Base: SuccessResponse("Employee group deleted successfully"),
	}, nil
}

// ListEmployeeGroups lists employee groups with pagination and filters.
func (h *EmployeeGroupHandler) ListEmployeeGroups(ctx context.Context, req *iamv1.ListEmployeeGroupsRequest) (*iamv1.ListEmployeeGroupsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ListEmployeeGroupsResponse{Base: baseResp}, nil
	}

	isActive := activeFilterToBool(req.GetActiveFilter())

	result, err := h.listHandler.Handle(ctx, egapp.ListQuery{
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		Search:    req.GetSearch(),
		IsActive:  isActive,
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &iamv1.ListEmployeeGroupsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	protoItems := make([]*iamv1.EmployeeGroup, len(result.Items))
	for i, e := range result.Items {
		protoItems[i] = toEmployeeGroupProto(e)
	}

	return &iamv1.ListEmployeeGroupsResponse{
		Base: SuccessResponse("Employee groups retrieved successfully"),
		Data: protoItems,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportEmployeeGroups exports employee groups to Excel.
func (h *EmployeeGroupHandler) ExportEmployeeGroups(ctx context.Context, req *iamv1.ExportEmployeeGroupsRequest) (*iamv1.ExportEmployeeGroupsResponse, error) {
	isActive := activeFilterToBool(req.GetActiveFilter())

	result, err := h.exportHandler.Handle(ctx, egapp.ExportQuery{
		IsActive: isActive,
	})
	if err != nil {
		return &iamv1.ExportEmployeeGroupsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.ExportEmployeeGroupsResponse{
		Base:        SuccessResponse("Employee groups exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportEmployeeGroups imports employee groups from Excel.
func (h *EmployeeGroupHandler) ImportEmployeeGroups(ctx context.Context, req *iamv1.ImportEmployeeGroupsRequest) (*iamv1.ImportEmployeeGroupsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ImportEmployeeGroupsResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	result, err := h.importHandler.Handle(ctx, egapp.ImportCommand{
		FileContent:     req.GetFileContent(),
		FileName:        req.GetFileName(),
		DuplicateAction: req.GetDuplicateAction(),
		CreatedBy:       userID,
	})
	if err != nil {
		return &iamv1.ImportEmployeeGroupsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	importErrors := make([]*iamv1.ImportError, len(result.Errors))
	for i, ie := range result.Errors {
		importErrors[i] = &iamv1.ImportError{
			RowNumber: ie.RowNumber,
			Field:     ie.Field,
			Message:   ie.Message,
		}
	}

	return &iamv1.ImportEmployeeGroupsResponse{
		Base:         SuccessResponse("Employee groups import completed"),
		SuccessCount: result.SuccessCount,
		SkippedCount: result.SkippedCount,
		UpdatedCount: result.UpdatedCount,
		FailedCount:  result.FailedCount,
		Errors:       importErrors,
	}, nil
}

// DownloadEmployeeGroupTemplate downloads an Excel import template.
func (h *EmployeeGroupHandler) DownloadEmployeeGroupTemplate(_ context.Context, _ *iamv1.DownloadEmployeeGroupTemplateRequest) (*iamv1.DownloadEmployeeGroupTemplateResponse, error) {
	result, err := h.templateHandler.Handle()
	if err != nil {
		return &iamv1.DownloadEmployeeGroupTemplateResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.DownloadEmployeeGroupTemplateResponse{
		Base:        SuccessResponse("Import template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// toEmployeeGroupProto converts a domain EmployeeGroup to the proto message.
func toEmployeeGroupProto(e *employeegroup.EmployeeGroup) *iamv1.EmployeeGroup {
	return &iamv1.EmployeeGroup{
		EmployeeGroupId: e.ID().String(),
		Code:            e.Code().String(),
		Name:            e.Name(),
		IsActive:        e.IsActive(),
		Audit:           toAuditProto(e.Audit()),
	}
}
