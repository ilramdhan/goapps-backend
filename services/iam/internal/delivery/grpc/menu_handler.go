// Package grpc provides gRPC delivery layer implementations.
package grpc

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/menu"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/pkg/safeconv"
)

// MenuHandler implements iamv1.MenuServiceServer.
type MenuHandler struct {
	iamv1.UnimplementedMenuServiceServer
	menuRepo         menu.Repository
	validationHelper *ValidationHelper
}

// NewMenuHandler creates a new MenuHandler.
func NewMenuHandler(menuRepo menu.Repository, validationHelper *ValidationHelper) *MenuHandler {
	return &MenuHandler{
		menuRepo:         menuRepo,
		validationHelper: validationHelper,
	}
}

// CreateMenu creates a new menu.
func (h *MenuHandler) CreateMenu(ctx context.Context, req *iamv1.CreateMenuRequest) (*iamv1.CreateMenuResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.CreateMenuResponse{Base: baseResp}, nil
	}

	var parentID *uuid.UUID
	if req.ParentId != nil && *req.ParentId != "" {
		id, err := uuid.Parse(*req.ParentId)
		if err != nil {
			return &iamv1.CreateMenuResponse{Base: ErrorResponse("400", "invalid parent_id format")}, nil //nolint:nilerr // error returned in response body
		}
		parentID = &id
	}

	exists, err := h.menuRepo.ExistsByCode(ctx, req.GetMenuCode())
	if err != nil {
		return &iamv1.CreateMenuResponse{Base: InternalErrorResponse(fmt.Sprintf("failed to check code: %v", err))}, nil //nolint:nilerr // error returned in response body
	}
	if exists {
		return &iamv1.CreateMenuResponse{Base: ConflictResponse("menu code already exists")}, nil
	}

	m, err := menu.NewMenu(
		parentID,
		req.GetMenuCode(),
		req.GetMenuTitle(),
		req.GetMenuUrl(),
		req.GetIconName(),
		req.GetServiceName(),
		int(req.GetMenuLevel()),
		int(req.GetSortOrder()),
		req.GetIsVisible(),
		"system",
	)
	if err != nil {
		return &iamv1.CreateMenuResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	if err := h.menuRepo.Create(ctx, m); err != nil {
		return &iamv1.CreateMenuResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	if len(req.GetPermissionIds()) > 0 {
		permIDs := make([]uuid.UUID, 0, len(req.GetPermissionIds()))
		for _, pid := range req.GetPermissionIds() {
			if id, err := uuid.Parse(pid); err == nil {
				permIDs = append(permIDs, id)
			}
		}
		if err := h.menuRepo.AssignPermissions(ctx, m.ID(), permIDs, "system"); err != nil {
			log.Warn().Err(err).Str("menu_id", m.ID().String()).Msg("failed to assign permissions during menu creation")
		}
	}

	return &iamv1.CreateMenuResponse{
		Base: &commonv1.BaseResponse{
			IsSuccess:  true,
			StatusCode: "201",
			Message:    "Menu created successfully",
		},
		Data: toMenuProto(m),
	}, nil
}

// GetMenu retrieves a menu by ID.
func (h *MenuHandler) GetMenu(ctx context.Context, req *iamv1.GetMenuRequest) (*iamv1.GetMenuResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetMenuResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetMenuId())
	if err != nil {
		return &iamv1.GetMenuResponse{Base: ErrorResponse("400", "invalid menu_id format")}, nil //nolint:nilerr // error returned in response body
	}

	m, err := h.menuRepo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.GetMenuResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	perms, err := h.menuRepo.GetPermissions(ctx, id)
	if err != nil {
		log.Warn().Err(err).Str("menu_id", id.String()).Msg("failed to get permissions for menu")
	}

	return &iamv1.GetMenuResponse{
		Base:                SuccessResponse("Menu retrieved successfully"),
		Data:                toMenuProto(m),
		RequiredPermissions: toPermissionProtos(perms),
	}, nil
}

// UpdateMenu updates a menu.
func (h *MenuHandler) UpdateMenu(ctx context.Context, req *iamv1.UpdateMenuRequest) (*iamv1.UpdateMenuResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdateMenuResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetMenuId())
	if err != nil {
		return &iamv1.UpdateMenuResponse{Base: ErrorResponse("400", "invalid menu_id format")}, nil //nolint:nilerr // error returned in response body
	}

	m, err := h.menuRepo.GetByID(ctx, id)
	if err != nil {
		return &iamv1.UpdateMenuResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	if err := m.Update(
		req.MenuTitle,
		req.MenuUrl,
		req.IconName,
		convertInt32Ptr(req.SortOrder),
		req.IsVisible,
		req.IsActive,
		"system",
	); err != nil {
		return &iamv1.UpdateMenuResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	if err := h.menuRepo.Update(ctx, m); err != nil {
		return &iamv1.UpdateMenuResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.UpdateMenuResponse{
		Base: SuccessResponse("Menu updated successfully"),
		Data: toMenuProto(m),
	}, nil
}

// DeleteMenu deletes a menu.
func (h *MenuHandler) DeleteMenu(ctx context.Context, req *iamv1.DeleteMenuRequest) (*iamv1.DeleteMenuResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.DeleteMenuResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.GetMenuId())
	if err != nil {
		return &iamv1.DeleteMenuResponse{Base: ErrorResponse("400", "invalid menu_id format")}, nil //nolint:nilerr // error returned in response body
	}

	hasChildren, err := h.menuRepo.HasChildren(ctx, id)
	if err != nil {
		return &iamv1.DeleteMenuResponse{Base: InternalErrorResponse(fmt.Sprintf("failed to check children: %v", err))}, nil //nolint:nilerr // error returned in response body
	}
	if hasChildren {
		return &iamv1.DeleteMenuResponse{Base: ErrorResponse("409", "cannot delete menu with children")}, nil
	}

	if err := h.menuRepo.Delete(ctx, id, "system"); err != nil {
		return &iamv1.DeleteMenuResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.DeleteMenuResponse{
		Base: SuccessResponse("Menu deleted successfully"),
	}, nil
}

// ListMenus lists menus with pagination.
func (h *MenuHandler) ListMenus(ctx context.Context, req *iamv1.ListMenusRequest) (*iamv1.ListMenusResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ListMenusResponse{Base: baseResp}, nil
	}

	page := int(req.GetPage())
	if page < 1 {
		page = 1
	}
	pageSize := int(req.GetPageSize())
	if pageSize < 1 {
		pageSize = 10
	}

	params := menu.ListParams{
		Page:        page,
		PageSize:    pageSize,
		Search:      req.GetSearch(),
		ServiceName: req.GetServiceName(),
		SortBy:      req.GetSortBy(),
		SortOrder:   req.GetSortOrder(),
	}

	menus, total, err := h.menuRepo.List(ctx, params)
	if err != nil {
		return &iamv1.ListMenusResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	data := make([]*iamv1.Menu, len(menus))
	for i, m := range menus {
		data[i] = toMenuProto(m)
	}

	totalPages := safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))

	return &iamv1.ListMenusResponse{
		Base: SuccessResponse("Menus listed successfully"),
		Data: data,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: safeconv.IntToInt32(page),
			PageSize:    safeconv.IntToInt32(pageSize),
			TotalItems:  total,
			TotalPages:  totalPages,
		},
	}, nil
}

// GetMenuTree retrieves the menu tree for authenticated user.
func (h *MenuHandler) GetMenuTree(ctx context.Context, req *iamv1.GetMenuTreeRequest) (*iamv1.GetMenuTreeResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetMenuTreeResponse{Base: baseResp}, nil
	}

	userID := uuid.Nil // TODO: get from context

	tree, err := h.menuRepo.GetTreeForUser(ctx, userID, req.GetServiceName())
	if err != nil {
		return &iamv1.GetMenuTreeResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.GetMenuTreeResponse{
		Base: SuccessResponse("Menu tree retrieved successfully"),
		Data: toMenuWithChildrenProtos(tree),
	}, nil
}

// GetFullMenuTree retrieves the full menu tree for admin.
func (h *MenuHandler) GetFullMenuTree(ctx context.Context, req *iamv1.GetFullMenuTreeRequest) (*iamv1.GetFullMenuTreeResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetFullMenuTreeResponse{Base: baseResp}, nil
	}

	tree, err := h.menuRepo.GetTree(ctx, req.GetServiceName(), req.GetIncludeInactive(), req.GetIncludeHidden())
	if err != nil {
		return &iamv1.GetFullMenuTreeResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.GetFullMenuTreeResponse{
		Base: SuccessResponse("Full menu tree retrieved successfully"),
		Data: toMenuWithChildrenProtos(tree),
	}, nil
}

// AssignMenuPermissions assigns permissions to a menu.
func (h *MenuHandler) AssignMenuPermissions(ctx context.Context, req *iamv1.AssignMenuPermissionsRequest) (*iamv1.AssignMenuPermissionsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.AssignMenuPermissionsResponse{Base: baseResp}, nil
	}

	menuID, err := uuid.Parse(req.GetMenuId())
	if err != nil {
		return &iamv1.AssignMenuPermissionsResponse{Base: ErrorResponse("400", "invalid menu_id format")}, nil //nolint:nilerr // error returned in response body
	}

	permIDs := make([]uuid.UUID, 0, len(req.GetPermissionIds()))
	for _, pid := range req.GetPermissionIds() {
		if id, err := uuid.Parse(pid); err == nil {
			permIDs = append(permIDs, id)
		}
	}

	if err := h.menuRepo.AssignPermissions(ctx, menuID, permIDs, "system"); err != nil {
		return &iamv1.AssignMenuPermissionsResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.AssignMenuPermissionsResponse{
		Base: SuccessResponse("Permissions assigned successfully"),
	}, nil
}

// RemoveMenuPermissions removes permissions from a menu.
func (h *MenuHandler) RemoveMenuPermissions(ctx context.Context, req *iamv1.RemoveMenuPermissionsRequest) (*iamv1.RemoveMenuPermissionsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.RemoveMenuPermissionsResponse{Base: baseResp}, nil
	}

	menuID, err := uuid.Parse(req.GetMenuId())
	if err != nil {
		return &iamv1.RemoveMenuPermissionsResponse{Base: ErrorResponse("400", "invalid menu_id format")}, nil //nolint:nilerr // error returned in response body
	}

	permIDs := make([]uuid.UUID, 0, len(req.GetPermissionIds()))
	for _, pid := range req.GetPermissionIds() {
		if id, err := uuid.Parse(pid); err == nil {
			permIDs = append(permIDs, id)
		}
	}

	if err := h.menuRepo.RemovePermissions(ctx, menuID, permIDs); err != nil {
		return &iamv1.RemoveMenuPermissionsResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.RemoveMenuPermissionsResponse{
		Base: SuccessResponse("Permissions removed successfully"),
	}, nil
}

// GetMenuPermissions gets permissions for a menu.
func (h *MenuHandler) GetMenuPermissions(ctx context.Context, req *iamv1.GetMenuPermissionsRequest) (*iamv1.GetMenuPermissionsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.GetMenuPermissionsResponse{Base: baseResp}, nil
	}

	menuID, err := uuid.Parse(req.GetMenuId())
	if err != nil {
		return &iamv1.GetMenuPermissionsResponse{Base: ErrorResponse("400", "invalid menu_id format")}, nil //nolint:nilerr // error returned in response body
	}

	perms, err := h.menuRepo.GetPermissions(ctx, menuID)
	if err != nil {
		return &iamv1.GetMenuPermissionsResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.GetMenuPermissionsResponse{
		Base: SuccessResponse("Permissions retrieved successfully"),
		Data: toPermissionProtos(perms),
	}, nil
}

// ReorderMenus reorders menus within the same parent.
func (h *MenuHandler) ReorderMenus(ctx context.Context, req *iamv1.ReorderMenusRequest) (*iamv1.ReorderMenusResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ReorderMenusResponse{Base: baseResp}, nil
	}

	var parentID *uuid.UUID
	if req.ParentId != nil && *req.ParentId != "" {
		id, err := uuid.Parse(*req.ParentId)
		if err != nil {
			return &iamv1.ReorderMenusResponse{Base: ErrorResponse("400", "invalid parent_id format")}, nil //nolint:nilerr // error returned in response body
		}
		parentID = &id
	}

	menuIDs := make([]uuid.UUID, 0, len(req.GetMenuIds()))
	for _, mid := range req.GetMenuIds() {
		if id, err := uuid.Parse(mid); err == nil {
			menuIDs = append(menuIDs, id)
		}
	}

	if err := h.menuRepo.Reorder(ctx, parentID, menuIDs); err != nil {
		return &iamv1.ReorderMenusResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.ReorderMenusResponse{
		Base: SuccessResponse("Menus reordered successfully"),
	}, nil
}

// ExportMenus exports menus.
func (h *MenuHandler) ExportMenus(_ context.Context, _ *iamv1.ExportMenusRequest) (*iamv1.ExportMenusResponse, error) {
	return &iamv1.ExportMenusResponse{
		Base: ErrorResponse("501", "export not implemented"),
	}, nil
}

// ImportMenus imports menus.
func (h *MenuHandler) ImportMenus(_ context.Context, _ *iamv1.ImportMenusRequest) (*iamv1.ImportMenusResponse, error) {
	return &iamv1.ImportMenusResponse{
		Base: ErrorResponse("501", "import not implemented"),
	}, nil
}

// DownloadMenuTemplate downloads the menu template.
func (h *MenuHandler) DownloadMenuTemplate(_ context.Context, _ *iamv1.DownloadMenuTemplateRequest) (*iamv1.DownloadMenuTemplateResponse, error) {
	return &iamv1.DownloadMenuTemplateResponse{
		Base: ErrorResponse("501", "template download not implemented"),
	}, nil
}

// Helper functions

func toMenuProto(m *menu.Menu) *iamv1.Menu {
	var parentID *string
	if m.ParentID() != nil {
		s := m.ParentID().String()
		parentID = &s
	}

	return &iamv1.Menu{
		MenuId:      m.ID().String(),
		ParentId:    parentID,
		MenuCode:    m.Code(),
		MenuTitle:   m.Title(),
		MenuUrl:     strPtr(m.URL()),
		IconName:    m.IconName(),
		ServiceName: m.ServiceName(),
		MenuLevel:   iamv1.MenuLevel(safeconv.IntToInt32(m.Level())),
		SortOrder:   safeconv.IntToInt32(m.SortOrder()),
		IsVisible:   m.IsVisible(),
		IsActive:    m.IsActive(),
		Audit:       toAuditProto(m.Audit()),
	}
}

func toMenuWithChildrenProtos(items []*menu.WithChildren) []*iamv1.MenuWithChildren {
	result := make([]*iamv1.MenuWithChildren, len(items))
	for i, item := range items {
		result[i] = &iamv1.MenuWithChildren{
			Menu:                toMenuProto(item.Menu),
			Children:            toMenuWithChildrenProtos(item.Children),
			RequiredPermissions: item.RequiredPermissions,
		}
	}
	return result
}

func convertInt32Ptr(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toPermissionProtos(perms []*role.Permission) []*iamv1.Permission {
	result := make([]*iamv1.Permission, len(perms))
	for i, p := range perms {
		result[i] = &iamv1.Permission{
			PermissionId:   p.ID().String(),
			PermissionCode: p.Code(),
			PermissionName: p.Name(),
			ServiceName:    p.ServiceName(),
			ModuleName:     p.ModuleName(),
			ActionType:     p.ActionType(),
		}
	}
	return result
}
