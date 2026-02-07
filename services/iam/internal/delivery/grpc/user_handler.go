// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	userapp "github.com/mutugading/goapps-backend/services/iam/internal/application/user"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// UserHandler implements the UserService gRPC service.
type UserHandler struct {
	iamv1.UnimplementedUserServiceServer
	createHandler              *userapp.CreateHandler
	getHandler                 *userapp.GetHandler
	getDetailHandler           *userapp.GetDetailHandler
	updateHandler              *userapp.UpdateHandler
	updateDetailHandler        *userapp.UpdateDetailHandler
	deleteHandler              *userapp.DeleteHandler
	listHandler                *userapp.ListHandler
	assignRolesHandler         *userapp.AssignRolesHandler
	removeRolesHandler         *userapp.RemoveRolesHandler
	assignPermissionsHandler   *userapp.AssignPermissionsHandler
	removePermissionsHandler   *userapp.RemovePermissionsHandler
	getRolesPermissionsHandler *userapp.GetRolesPermissionsHandler
	validationHelper           *ValidationHelper
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(
	userRepo user.Repository,
	userRoleRepo role.UserRoleRepository,
	userPermissionRepo role.UserPermissionRepository,
	validationHelper *ValidationHelper,
) *UserHandler {
	return &UserHandler{
		createHandler:              userapp.NewCreateHandler(userRepo),
		getHandler:                 userapp.NewGetHandler(userRepo),
		getDetailHandler:           userapp.NewGetDetailHandler(userRepo),
		updateHandler:              userapp.NewUpdateHandler(userRepo),
		updateDetailHandler:        userapp.NewUpdateDetailHandler(userRepo),
		deleteHandler:              userapp.NewDeleteHandler(userRepo),
		listHandler:                userapp.NewListHandler(userRepo),
		assignRolesHandler:         userapp.NewAssignRolesHandler(userRepo, userRoleRepo),
		removeRolesHandler:         userapp.NewRemoveRolesHandler(userRepo, userRoleRepo),
		assignPermissionsHandler:   userapp.NewAssignPermissionsHandler(userRepo, userPermissionRepo),
		removePermissionsHandler:   userapp.NewRemovePermissionsHandler(userRepo, userPermissionRepo),
		getRolesPermissionsHandler: userapp.NewGetRolesPermissionsHandler(userRepo),
		validationHelper:           validationHelper,
	}
}

// getActorID extracts the authenticated user ID as a string from context.
// Falls back to "system" if not authenticated.
func (h *UserHandler) getActorID(ctx context.Context) string {
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return "system"
	}
	return userID.String()
}

// CreateUser creates a new user.
func (h *UserHandler) CreateUser(ctx context.Context, req *iamv1.CreateUserRequest) (*iamv1.CreateUserResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateUserResponse{Base: baseResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, userapp.CreateCommand{
		Username:     req.GetUsername(),
		Email:        req.GetEmail(),
		PasswordHash: req.GetPassword(),
		EmployeeCode: req.GetEmployeeCode(),
		FullName:     req.GetFullName(),
		FirstName:    req.GetFirstName(),
		LastName:     req.GetLastName(),
		CreatedBy:    h.getActorID(ctx),
	})
	if err != nil {
		return &iamv1.CreateUserResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.CreateUserResponse{
		Base: SuccessResponse("User created successfully"),
		Data: h.toUserWithDetailProto(entity, nil, nil),
	}, nil
}

// GetUser retrieves a user by ID.
func (h *UserHandler) GetUser(ctx context.Context, req *iamv1.GetUserRequest) (*iamv1.GetUserResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetUserResponse{Base: baseResp}, nil
	}

	entity, err := h.getHandler.Handle(ctx, userapp.GetQuery{
		UserID: req.GetUserId(),
	})
	if err != nil {
		return &iamv1.GetUserResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.GetUserResponse{
		Base: SuccessResponse("User retrieved successfully"),
		Data: h.toUserProto(entity),
	}, nil
}

// GetUserDetail retrieves user with full employee details.
func (h *UserHandler) GetUserDetail(ctx context.Context, req *iamv1.GetUserDetailRequest) (*iamv1.GetUserDetailResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetUserDetailResponse{Base: baseResp}, nil
	}

	result, err := h.getDetailHandler.Handle(ctx, userapp.GetDetailQuery{
		UserID: req.GetUserId(),
	})
	if err != nil {
		return &iamv1.GetUserDetailResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.GetUserDetailResponse{
		Base: SuccessResponse("User detail retrieved successfully"),
		Data: h.toUserWithDetailProto(result.User, result.Detail, nil),
	}, nil
}

// UpdateUser updates user credentials.
func (h *UserHandler) UpdateUser(ctx context.Context, req *iamv1.UpdateUserRequest) (*iamv1.UpdateUserResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateUserResponse{Base: baseResp}, nil
	}

	entity, err := h.updateHandler.Handle(ctx, userapp.UpdateCommand{
		UserID:    req.GetUserId(),
		Email:     req.Email,
		IsActive:  req.IsActive,
		UpdatedBy: h.getActorID(ctx),
	})
	if err != nil {
		return &iamv1.UpdateUserResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.UpdateUserResponse{
		Base: SuccessResponse("User updated successfully"),
		Data: h.toUserProto(entity),
	}, nil
}

// UpdateUserDetail updates employee details.
func (h *UserHandler) UpdateUserDetail(ctx context.Context, req *iamv1.UpdateUserDetailRequest) (*iamv1.UpdateUserDetailResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateUserDetailResponse{Base: baseResp}, nil
	}

	cmd := userapp.UpdateDetailCommand{
		UserID:         req.GetUserId(),
		FullName:       req.FullName,
		FirstName:      req.FirstName,
		LastName:       req.LastName,
		Phone:          req.Phone,
		ProfilePicture: req.ProfilePictureUrl,
		Position:       req.Position,
		Address:        req.Address,
		UpdatedBy:      h.getActorID(ctx),
	}

	// Parse section ID if provided.
	if req.SectionId != nil {
		sectionID, err := uuid.Parse(req.GetSectionId())
		if err == nil {
			cmd.SectionID = &sectionID
		}
	}

	// Parse date of birth if provided.
	if req.DateOfBirth != nil {
		dob, err := time.Parse("2006-01-02", req.GetDateOfBirth())
		if err == nil {
			cmd.DateOfBirth = &dob
		}
	}

	detail, err := h.updateDetailHandler.Handle(ctx, cmd)
	if err != nil {
		return &iamv1.UpdateUserDetailResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.UpdateUserDetailResponse{
		Base: SuccessResponse("User detail updated successfully"),
		Data: h.toUserDetailProto(detail),
	}, nil
}

// DeleteUser soft deletes a user.
func (h *UserHandler) DeleteUser(ctx context.Context, req *iamv1.DeleteUserRequest) (*iamv1.DeleteUserResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteUserResponse{Base: baseResp}, nil
	}

	err := h.deleteHandler.Handle(ctx, userapp.DeleteCommand{
		UserID:    req.GetUserId(),
		DeletedBy: h.getActorID(ctx),
	})
	if err != nil {
		return &iamv1.DeleteUserResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.DeleteUserResponse{
		Base: SuccessResponse("User deleted successfully"),
	}, nil
}

// ListUsers lists users with search, filter, sort, and pagination.
func (h *UserHandler) ListUsers(ctx context.Context, req *iamv1.ListUsersRequest) (*iamv1.ListUsersResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ListUsersResponse{Base: baseResp}, nil
	}

	query := userapp.ListQuery{
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		Search:    req.GetSearch(),
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	}

	// Map active filter enum to *bool.
	switch req.GetActiveFilter() {
	case iamv1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		active := true
		query.IsActive = &active
	case iamv1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		inactive := false
		query.IsActive = &inactive
	case iamv1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
		// No filter.
	}

	// Parse optional UUID filters.
	query.SectionID = parseOptionalUUID(req.SectionId)
	query.DepartmentID = parseOptionalUUID(req.DepartmentId)
	query.DivisionID = parseOptionalUUID(req.DivisionId)
	query.CompanyID = parseOptionalUUID(req.CompanyId)

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		return &iamv1.ListUsersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	protoUsers := make([]*iamv1.UserWithDetail, len(result.Users))
	for i, uwd := range result.Users {
		roleCodes := make([]string, len(uwd.Roles))
		for j, r := range uwd.Roles {
			roleCodes[j] = r.RoleCode
		}
		protoUsers[i] = h.toUserWithDetailProto(uwd.User, uwd.Detail, roleCodes)
	}

	return &iamv1.ListUsersResponse{
		Base: SuccessResponse("Users retrieved successfully"),
		Data: protoUsers,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportUsers exports users to Excel file.
func (h *UserHandler) ExportUsers(_ context.Context, _ *iamv1.ExportUsersRequest) (*iamv1.ExportUsersResponse, error) {
	return &iamv1.ExportUsersResponse{Base: ErrorResponse("501", "Not implemented")}, nil
}

// ImportUsers imports users from Excel file.
func (h *UserHandler) ImportUsers(_ context.Context, _ *iamv1.ImportUsersRequest) (*iamv1.ImportUsersResponse, error) {
	return &iamv1.ImportUsersResponse{Base: ErrorResponse("501", "Not implemented")}, nil
}

// DownloadTemplate downloads the Excel import template.
func (h *UserHandler) DownloadTemplate(_ context.Context, _ *iamv1.DownloadTemplateRequest) (*iamv1.DownloadTemplateResponse, error) {
	return &iamv1.DownloadTemplateResponse{Base: ErrorResponse("501", "Not implemented")}, nil
}

// AssignUserRoles assigns roles to a user.
func (h *UserHandler) AssignUserRoles(ctx context.Context, req *iamv1.AssignUserRolesRequest) (*iamv1.AssignUserRolesResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.AssignUserRolesResponse{Base: baseResp}, nil
	}

	err := h.assignRolesHandler.Handle(ctx, userapp.AssignRolesCommand{
		UserID:     req.GetUserId(),
		RoleIDs:    req.GetRoleIds(),
		AssignedBy: h.getActorID(ctx),
	})
	if err != nil {
		return &iamv1.AssignUserRolesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.AssignUserRolesResponse{
		Base: SuccessResponse("Roles assigned successfully"),
	}, nil
}

// RemoveUserRoles removes roles from a user.
func (h *UserHandler) RemoveUserRoles(ctx context.Context, req *iamv1.RemoveUserRolesRequest) (*iamv1.RemoveUserRolesResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.RemoveUserRolesResponse{Base: baseResp}, nil
	}

	err := h.removeRolesHandler.Handle(ctx, userapp.RemoveRolesCommand{
		UserID:  req.GetUserId(),
		RoleIDs: req.GetRoleIds(),
	})
	if err != nil {
		return &iamv1.RemoveUserRolesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.RemoveUserRolesResponse{
		Base: SuccessResponse("Roles removed successfully"),
	}, nil
}

// AssignUserPermissions assigns direct permissions to a user.
func (h *UserHandler) AssignUserPermissions(ctx context.Context, req *iamv1.AssignUserPermissionsRequest) (*iamv1.AssignUserPermissionsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.AssignUserPermissionsResponse{Base: baseResp}, nil
	}

	err := h.assignPermissionsHandler.Handle(ctx, userapp.AssignPermissionsCommand{
		UserID:        req.GetUserId(),
		PermissionIDs: req.GetPermissionIds(),
		AssignedBy:    h.getActorID(ctx),
	})
	if err != nil {
		return &iamv1.AssignUserPermissionsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.AssignUserPermissionsResponse{
		Base: SuccessResponse("Permissions assigned successfully"),
	}, nil
}

// RemoveUserPermissions removes direct permissions from a user.
func (h *UserHandler) RemoveUserPermissions(ctx context.Context, req *iamv1.RemoveUserPermissionsRequest) (*iamv1.RemoveUserPermissionsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.RemoveUserPermissionsResponse{Base: baseResp}, nil
	}

	err := h.removePermissionsHandler.Handle(ctx, userapp.RemovePermissionsCommand{
		UserID:        req.GetUserId(),
		PermissionIDs: req.GetPermissionIds(),
	})
	if err != nil {
		return &iamv1.RemoveUserPermissionsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	return &iamv1.RemoveUserPermissionsResponse{
		Base: SuccessResponse("Permissions removed successfully"),
	}, nil
}

// GetUserRolesAndPermissions gets all roles and permissions for a user.
func (h *UserHandler) GetUserRolesAndPermissions(ctx context.Context, req *iamv1.GetUserRolesAndPermissionsRequest) (*iamv1.GetUserRolesAndPermissionsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetUserRolesAndPermissionsResponse{Base: baseResp}, nil
	}

	result, err := h.getRolesPermissionsHandler.Handle(ctx, userapp.GetRolesPermissionsQuery{
		UserID: req.GetUserId(),
	})
	if err != nil {
		return &iamv1.GetUserRolesAndPermissionsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	// Map roles to proto.
	protoRoles := make([]*iamv1.RoleWithPermissions, len(result.Roles))
	for i, r := range result.Roles {
		protoRoles[i] = &iamv1.RoleWithPermissions{
			RoleId:   r.ID().String(),
			RoleCode: r.Code(),
			RoleName: r.Name(),
		}
	}

	// Map direct permissions to proto.
	protoPermissions := make([]*iamv1.Permission, len(result.Permissions))
	allPermCodes := make([]string, 0, len(result.Permissions))
	for i, p := range result.Permissions {
		protoPermissions[i] = &iamv1.Permission{
			PermissionId:   p.ID().String(),
			PermissionCode: p.Code(),
		}
		allPermCodes = append(allPermCodes, p.Code())
	}

	return &iamv1.GetUserRolesAndPermissionsResponse{
		Base: SuccessResponse("User roles and permissions retrieved successfully"),
		Data: &iamv1.UserAccessInfo{
			UserId:             req.GetUserId(),
			Roles:              protoRoles,
			DirectPermissions:  protoPermissions,
			AllPermissionCodes: allPermCodes,
		},
	}, nil
}

// Helper methods

func (h *UserHandler) toUserProto(u *user.User) *iamv1.User {
	proto := &iamv1.User{
		UserId:           u.ID().String(),
		Username:         u.Username(),
		Email:            u.Email(),
		IsActive:         u.IsActive(),
		IsLocked:         u.IsLocked(),
		TwoFactorEnabled: u.TwoFactorEnabled(),
		Audit:            toAuditProto(u.Audit()),
	}

	if u.LastLoginAt() != nil {
		lastLogin := u.LastLoginAt().Format("2006-01-02T15:04:05Z07:00")
		proto.LastLoginAt = &lastLogin
	}

	return proto
}

func (h *UserHandler) toUserWithDetailProto(u *user.User, detail *user.Detail, roleCodes []string) *iamv1.UserWithDetail {
	proto := &iamv1.UserWithDetail{
		User:      h.toUserProto(u),
		RoleCodes: roleCodes,
	}

	if detail != nil {
		proto.Detail = h.toUserDetailProto(detail)
	}

	return proto
}

func (h *UserHandler) toUserDetailProto(d *user.Detail) *iamv1.UserDetail {
	proto := &iamv1.UserDetail{
		DetailId:     d.ID().String(),
		UserId:       d.UserID().String(),
		EmployeeCode: d.EmployeeCode(),
		FullName:     d.FullName(),
		FirstName:    d.FirstName(),
		LastName:     d.LastName(),
		Audit:        toAuditProto(d.Audit()),
	}

	if d.SectionID() != nil {
		sectionID := d.SectionID().String()
		proto.SectionId = &sectionID
	}
	if d.Phone() != "" {
		phone := d.Phone()
		proto.Phone = &phone
	}
	if d.ProfilePicture() != "" {
		url := d.ProfilePicture()
		proto.ProfilePictureUrl = &url
	}
	if d.Position() != "" {
		position := d.Position()
		proto.Position = &position
	}
	if d.DateOfBirth() != nil {
		dob := d.DateOfBirth().Format("2006-01-02")
		proto.DateOfBirth = &dob
	}
	if d.Address() != "" {
		address := d.Address()
		proto.Address = &address
	}

	return proto
}
