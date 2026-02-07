// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"context"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	roleapp "github.com/mutugading/goapps-backend/services/iam/internal/application/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
)

// RoleHandler implements the RoleService gRPC service.
type RoleHandler struct {
	iamv1.UnimplementedRoleServiceServer
	createHandler            *roleapp.CreateHandler
	getHandler               *roleapp.GetHandler
	updateHandler            *roleapp.UpdateHandler
	deleteHandler            *roleapp.DeleteHandler
	listHandler              *roleapp.ListHandler
	assignPermissionsHandler *roleapp.AssignPermissionsHandler
	removePermissionsHandler *roleapp.RemovePermissionsHandler
	getPermissionsHandler    *roleapp.GetPermissionsHandler
	validationHelper         *ValidationHelper
}

// NewRoleHandler creates a new RoleHandler.
func NewRoleHandler(roleRepo role.Repository, validationHelper *ValidationHelper) *RoleHandler {
	return &RoleHandler{
		createHandler:            roleapp.NewCreateHandler(roleRepo),
		getHandler:               roleapp.NewGetHandler(roleRepo),
		updateHandler:            roleapp.NewUpdateHandler(roleRepo),
		deleteHandler:            roleapp.NewDeleteHandler(roleRepo),
		listHandler:              roleapp.NewListHandler(roleRepo),
		assignPermissionsHandler: roleapp.NewAssignPermissionsHandler(roleRepo),
		removePermissionsHandler: roleapp.NewRemovePermissionsHandler(roleRepo),
		getPermissionsHandler:    roleapp.NewGetPermissionsHandler(roleRepo),
		validationHelper:         validationHelper,
	}
}

// CreateRole creates a new role.
func (h *RoleHandler) CreateRole(ctx context.Context, req *iamv1.CreateRoleRequest) (*iamv1.CreateRoleResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateRoleResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	entity, err := h.createHandler.Handle(ctx, roleapp.CreateCommand{
		Code:        req.GetRoleCode(),
		Name:        req.GetRoleName(),
		Description: req.GetDescription(),
		CreatedBy:   userID,
	})
	if err != nil {
		return &iamv1.CreateRoleResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.CreateRoleResponse{
		Base: &commonv1.BaseResponse{
			IsSuccess:  true,
			StatusCode: "201",
			Message:    "Role created successfully",
		},
		Data: toRoleProto(entity),
	}, nil
}

// GetRole retrieves a role by ID.
func (h *RoleHandler) GetRole(ctx context.Context, req *iamv1.GetRoleRequest) (*iamv1.GetRoleResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetRoleResponse{Base: baseResp}, nil
	}

	entity, err := h.getHandler.Handle(ctx, roleapp.GetQuery{
		RoleID: req.GetRoleId(),
	})
	if err != nil {
		return &iamv1.GetRoleResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.GetRoleResponse{
		Base: SuccessResponse("Role retrieved successfully"),
		Data: toRoleProto(entity),
	}, nil
}

// UpdateRole updates a role.
func (h *RoleHandler) UpdateRole(ctx context.Context, req *iamv1.UpdateRoleRequest) (*iamv1.UpdateRoleResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateRoleResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	entity, err := h.updateHandler.Handle(ctx, roleapp.UpdateCommand{
		RoleID:      req.GetRoleId(),
		Name:        req.RoleName,
		Description: req.Description,
		IsActive:    req.IsActive,
		UpdatedBy:   userID,
	})
	if err != nil {
		return &iamv1.UpdateRoleResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.UpdateRoleResponse{
		Base: SuccessResponse("Role updated successfully"),
		Data: toRoleProto(entity),
	}, nil
}

// DeleteRole deletes a role.
func (h *RoleHandler) DeleteRole(ctx context.Context, req *iamv1.DeleteRoleRequest) (*iamv1.DeleteRoleResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteRoleResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	if err := h.deleteHandler.Handle(ctx, roleapp.DeleteCommand{
		RoleID:    req.GetRoleId(),
		DeletedBy: userID,
	}); err != nil {
		return &iamv1.DeleteRoleResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.DeleteRoleResponse{
		Base: SuccessResponse("Role deleted successfully"),
	}, nil
}

// ListRoles lists roles with pagination.
func (h *RoleHandler) ListRoles(ctx context.Context, req *iamv1.ListRolesRequest) (*iamv1.ListRolesResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ListRolesResponse{Base: baseResp}, nil
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

	result, err := h.listHandler.Handle(ctx, roleapp.ListQuery{
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		Search:    req.GetSearch(),
		IsActive:  isActive,
		IsSystem:  req.IsSystem,
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &iamv1.ListRolesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	protoRoles := make([]*iamv1.Role, len(result.Roles))
	for i, r := range result.Roles {
		protoRoles[i] = toRoleProto(r)
	}

	return &iamv1.ListRolesResponse{
		Base: SuccessResponse("Roles retrieved successfully"),
		Data: protoRoles,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportRoles exports roles.
func (h *RoleHandler) ExportRoles(_ context.Context, _ *iamv1.ExportRolesRequest) (*iamv1.ExportRolesResponse, error) {
	return &iamv1.ExportRolesResponse{
		Base: ErrorResponse("501", "Not implemented"),
	}, nil
}

// ImportRoles imports roles.
func (h *RoleHandler) ImportRoles(_ context.Context, _ *iamv1.ImportRolesRequest) (*iamv1.ImportRolesResponse, error) {
	return &iamv1.ImportRolesResponse{
		Base: ErrorResponse("501", "Not implemented"),
	}, nil
}

// DownloadRoleTemplate downloads the role import template.
func (h *RoleHandler) DownloadRoleTemplate(_ context.Context, _ *iamv1.DownloadRoleTemplateRequest) (*iamv1.DownloadRoleTemplateResponse, error) {
	return &iamv1.DownloadRoleTemplateResponse{
		Base: ErrorResponse("501", "Not implemented"),
	}, nil
}

// AssignRolePermissions assigns permissions to a role.
func (h *RoleHandler) AssignRolePermissions(ctx context.Context, req *iamv1.AssignRolePermissionsRequest) (*iamv1.AssignRolePermissionsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.AssignRolePermissionsResponse{Base: baseResp}, nil
	}

	userID := getUserFromCtx(ctx)

	if err := h.assignPermissionsHandler.Handle(ctx, roleapp.AssignPermissionsCommand{
		RoleID:        req.GetRoleId(),
		PermissionIDs: req.GetPermissionIds(),
		AssignedBy:    userID,
	}); err != nil {
		return &iamv1.AssignRolePermissionsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.AssignRolePermissionsResponse{
		Base: SuccessResponse("Permissions assigned successfully"),
	}, nil
}

// RemoveRolePermissions removes permissions from a role.
func (h *RoleHandler) RemoveRolePermissions(ctx context.Context, req *iamv1.RemoveRolePermissionsRequest) (*iamv1.RemoveRolePermissionsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.RemoveRolePermissionsResponse{Base: baseResp}, nil
	}

	if err := h.removePermissionsHandler.Handle(ctx, roleapp.RemovePermissionsCommand{
		RoleID:        req.GetRoleId(),
		PermissionIDs: req.GetPermissionIds(),
	}); err != nil {
		return &iamv1.RemoveRolePermissionsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.RemoveRolePermissionsResponse{
		Base: SuccessResponse("Permissions removed successfully"),
	}, nil
}

// GetRolePermissions gets permissions for a role.
func (h *RoleHandler) GetRolePermissions(ctx context.Context, req *iamv1.GetRolePermissionsRequest) (*iamv1.GetRolePermissionsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetRolePermissionsResponse{Base: baseResp}, nil
	}

	perms, err := h.getPermissionsHandler.Handle(ctx, roleapp.GetPermissionsQuery{
		RoleID: req.GetRoleId(),
	})
	if err != nil {
		return &iamv1.GetRolePermissionsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	protoPerms := make([]*iamv1.Permission, len(perms))
	for i, p := range perms {
		protoPerms[i] = permissionToProto(p)
	}

	return &iamv1.GetRolePermissionsResponse{
		Base: SuccessResponse("Permissions retrieved successfully"),
		Data: protoPerms,
	}, nil
}

// Helper methods

func toRoleProto(r *role.Role) *iamv1.Role {
	return &iamv1.Role{
		RoleId:      r.ID().String(),
		RoleCode:    r.Code(),
		RoleName:    r.Name(),
		Description: r.Description(),
		IsSystem:    r.IsSystem(),
		IsActive:    r.IsActive(),
		Audit:       toAuditProto(r.Audit()),
	}
}

// permissionToProto converts a domain Permission to a proto Permission message.
// This is a package-level function shared by both RoleHandler and PermissionHandler.
func permissionToProto(p *role.Permission) *iamv1.Permission {
	return &iamv1.Permission{
		PermissionId:   p.ID().String(),
		PermissionCode: p.Code(),
		PermissionName: p.Name(),
		ServiceName:    p.ServiceName(),
		ModuleName:     p.ModuleName(),
		ActionType:     p.ActionType(),
	}
}

// permissionToDetailProto converts a domain Permission to a proto PermissionDetail message.
// PermissionDetail includes additional fields like Description, IsActive, and Audit.
func permissionToDetailProto(p *role.Permission) *iamv1.PermissionDetail {
	return &iamv1.PermissionDetail{
		PermissionId:   p.ID().String(),
		PermissionCode: p.Code(),
		PermissionName: p.Name(),
		Description:    p.Description(),
		ServiceName:    p.ServiceName(),
		ModuleName:     p.ModuleName(),
		ActionType:     p.ActionType(),
		IsActive:       p.IsActive(),
		Audit:          toAuditProto(p.Audit()),
	}
}
