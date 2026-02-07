// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"context"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	permapp "github.com/mutugading/goapps-backend/services/iam/internal/application/permission"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
)

// PermissionHandler implements the PermissionService gRPC service.
type PermissionHandler struct {
	iamv1.UnimplementedPermissionServiceServer
	createHandler    *permapp.CreateHandler
	getHandler       *permapp.GetHandler
	updateHandler    *permapp.UpdateHandler
	deleteHandler    *permapp.DeleteHandler
	listHandler      *permapp.ListHandler
	permRepo         role.PermissionRepository
	validationHelper *ValidationHelper
}

// NewPermissionHandler creates a new PermissionHandler.
func NewPermissionHandler(permRepo role.PermissionRepository, validationHelper *ValidationHelper) *PermissionHandler {
	return &PermissionHandler{
		createHandler:    permapp.NewCreateHandler(permRepo),
		getHandler:       permapp.NewGetHandler(permRepo),
		updateHandler:    permapp.NewUpdateHandler(permRepo),
		deleteHandler:    permapp.NewDeleteHandler(permRepo),
		listHandler:      permapp.NewListHandler(permRepo),
		permRepo:         permRepo,
		validationHelper: validationHelper,
	}
}

// CreatePermission creates a new permission.
func (h *PermissionHandler) CreatePermission(ctx context.Context, req *iamv1.CreatePermissionRequest) (*iamv1.CreatePermissionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreatePermissionResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	entity, err := h.createHandler.Handle(ctx, permapp.CreateCommand{
		Code:        req.GetPermissionCode(),
		Name:        req.GetPermissionName(),
		Description: req.GetDescription(),
		ServiceName: req.GetServiceName(),
		ModuleName:  req.GetModuleName(),
		ActionType:  req.GetActionType(),
		CreatedBy:   userID,
	})
	if err != nil {
		return &iamv1.CreatePermissionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.CreatePermissionResponse{
		Base: &commonv1.BaseResponse{
			IsSuccess:  true,
			StatusCode: "201",
			Message:    "Permission created successfully",
		},
		Data: permissionToDetailProto(entity),
	}, nil
}

// GetPermission retrieves a permission by ID.
func (h *PermissionHandler) GetPermission(ctx context.Context, req *iamv1.GetPermissionRequest) (*iamv1.GetPermissionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetPermissionResponse{Base: baseResp}, nil
	}

	entity, err := h.getHandler.Handle(ctx, permapp.GetQuery{
		PermissionID: req.GetPermissionId(),
	})
	if err != nil {
		return &iamv1.GetPermissionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.GetPermissionResponse{
		Base: SuccessResponse("Permission retrieved successfully"),
		Data: permissionToDetailProto(entity),
	}, nil
}

// UpdatePermission updates a permission.
func (h *PermissionHandler) UpdatePermission(ctx context.Context, req *iamv1.UpdatePermissionRequest) (*iamv1.UpdatePermissionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdatePermissionResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	entity, err := h.updateHandler.Handle(ctx, permapp.UpdateCommand{
		PermissionID: req.GetPermissionId(),
		Name:         req.PermissionName,
		Description:  req.Description,
		IsActive:     req.IsActive,
		UpdatedBy:    userID,
	})
	if err != nil {
		return &iamv1.UpdatePermissionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.UpdatePermissionResponse{
		Base: SuccessResponse("Permission updated successfully"),
		Data: permissionToDetailProto(entity),
	}, nil
}

// DeletePermission deletes a permission.
func (h *PermissionHandler) DeletePermission(ctx context.Context, req *iamv1.DeletePermissionRequest) (*iamv1.DeletePermissionResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeletePermissionResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	if err := h.deleteHandler.Handle(ctx, permapp.DeleteCommand{
		PermissionID: req.GetPermissionId(),
		DeletedBy:    userID,
	}); err != nil {
		return &iamv1.DeletePermissionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.DeletePermissionResponse{
		Base: SuccessResponse("Permission deleted successfully"),
	}, nil
}

// ListPermissions lists permissions with pagination.
func (h *PermissionHandler) ListPermissions(ctx context.Context, req *iamv1.ListPermissionsRequest) (*iamv1.ListPermissionsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ListPermissionsResponse{Base: baseResp}, nil
	}

	// Convert ActiveFilter to *bool
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

	result, err := h.listHandler.Handle(ctx, permapp.ListQuery{
		Page:        int(req.GetPage()),
		PageSize:    int(req.GetPageSize()),
		Search:      req.GetSearch(),
		IsActive:    isActive,
		ServiceName: req.GetServiceName(),
		ModuleName:  req.GetModuleName(),
		ActionType:  req.GetActionType(),
		SortBy:      req.GetSortBy(),
		SortOrder:   req.GetSortOrder(),
	})
	if err != nil {
		return &iamv1.ListPermissionsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	protoPerms := make([]*iamv1.PermissionDetail, len(result.Permissions))
	for i, p := range result.Permissions {
		protoPerms[i] = permissionToDetailProto(p)
	}

	return &iamv1.ListPermissionsResponse{
		Base: SuccessResponse("Permissions retrieved successfully"),
		Data: protoPerms,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportPermissions exports permissions.
func (h *PermissionHandler) ExportPermissions(_ context.Context, _ *iamv1.ExportPermissionsRequest) (*iamv1.ExportPermissionsResponse, error) {
	return &iamv1.ExportPermissionsResponse{
		Base: ErrorResponse("501", "Not implemented"),
	}, nil
}

// ImportPermissions imports permissions.
func (h *PermissionHandler) ImportPermissions(_ context.Context, _ *iamv1.ImportPermissionsRequest) (*iamv1.ImportPermissionsResponse, error) {
	return &iamv1.ImportPermissionsResponse{
		Base: ErrorResponse("501", "Not implemented"),
	}, nil
}

// DownloadPermissionTemplate downloads the permission import template.
func (h *PermissionHandler) DownloadPermissionTemplate(_ context.Context, _ *iamv1.DownloadPermissionTemplateRequest) (*iamv1.DownloadPermissionTemplateResponse, error) {
	return &iamv1.DownloadPermissionTemplateResponse{
		Base: ErrorResponse("501", "Not implemented"),
	}, nil
}

// GetPermissionsByService gets permissions grouped by service and module.
func (h *PermissionHandler) GetPermissionsByService(ctx context.Context, req *iamv1.GetPermissionsByServiceRequest) (*iamv1.GetPermissionsByServiceResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetPermissionsByServiceResponse{Base: baseResp}, nil
	}

	servicePerms, err := h.permRepo.GetByService(ctx, req.GetServiceName(), req.GetIncludeInactive())
	if err != nil {
		return &iamv1.GetPermissionsByServiceResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	protoServicePerms := make([]*iamv1.ServicePermissions, len(servicePerms))
	for i, sp := range servicePerms {
		protoModules := make([]*iamv1.ModulePermissions, len(sp.Modules))
		for j, mp := range sp.Modules {
			protoPermissions := make([]*iamv1.PermissionDetail, len(mp.Permissions))
			for k, p := range mp.Permissions {
				protoPermissions[k] = permissionToDetailProto(p)
			}
			protoModules[j] = &iamv1.ModulePermissions{
				ModuleName:  mp.ModuleName,
				Permissions: protoPermissions,
			}
		}
		protoServicePerms[i] = &iamv1.ServicePermissions{
			ServiceName: sp.ServiceName,
			Modules:     protoModules,
		}
	}

	return &iamv1.GetPermissionsByServiceResponse{
		Base: SuccessResponse("Permissions by service retrieved successfully"),
		Data: protoServicePerms,
	}, nil
}
