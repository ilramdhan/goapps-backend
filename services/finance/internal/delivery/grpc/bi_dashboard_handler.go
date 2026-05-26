package grpc

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	dashboardapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/dashboard"
	groupapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/group"
	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
)

// emptypb import-only retention — gateway sometimes needs it via google.protobuf.Empty.
var _ = emptypb.Empty{}

// BIDashboardHandler implements financev1.DashboardServiceServer.
type BIDashboardHandler struct {
	financev1.UnimplementedDashboardServiceServer
	createHandler          *dashboardapp.CreateHandler
	getHandler             *dashboardapp.GetHandler
	listHandler            *dashboardapp.ListHandler
	updateHandler          *dashboardapp.UpdateHandler
	deleteHandler          *dashboardapp.DeleteHandler
	duplicateHandler       *dashboardapp.DuplicateHandler
	setRolesHandler        *dashboardapp.SetRolesHandler
	listAccessibleHandler  *dashboardapp.ListAccessibleHandler
	groupCreateHandler     *groupapp.CreateHandler
	groupListHandler       *groupapp.ListHandler
	groupUpdateHandler     *groupapp.UpdateHandler
	groupDeleteHandler     *groupapp.DeleteHandler
	validationHelper       *ValidationHelper
}

// NewBIDashboardHandler wires the dashboard + group application handlers into a gRPC server.
func NewBIDashboardHandler(
	create *dashboardapp.CreateHandler,
	get *dashboardapp.GetHandler,
	list *dashboardapp.ListHandler,
	update *dashboardapp.UpdateHandler,
	del *dashboardapp.DeleteHandler,
	dup *dashboardapp.DuplicateHandler,
	setRoles *dashboardapp.SetRolesHandler,
	listAccessible *dashboardapp.ListAccessibleHandler,
	groupCreate *groupapp.CreateHandler,
	groupList *groupapp.ListHandler,
	groupUpdate *groupapp.UpdateHandler,
	groupDelete *groupapp.DeleteHandler,
) (*BIDashboardHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &BIDashboardHandler{
		createHandler:         create,
		getHandler:            get,
		listHandler:           list,
		updateHandler:         update,
		deleteHandler:         del,
		duplicateHandler:      dup,
		setRolesHandler:       setRoles,
		listAccessibleHandler: listAccessible,
		groupCreateHandler:    groupCreate,
		groupListHandler:      groupList,
		groupUpdateHandler:    groupUpdate,
		groupDeleteHandler:    groupDelete,
		validationHelper:      v,
	}, nil
}

// CreateDashboard creates a new dashboard.
func (h *BIDashboardHandler) CreateDashboard(ctx context.Context, req *financev1.CreateDashboardRequest) (*financev1.CreateDashboardResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.CreateDashboardResponse{Base: baseResp}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	compareModes := make([]string, 0, len(req.GetCompareModes()))
	for _, m := range req.GetCompareModes() {
		compareModes = append(compareModes, compareModeToString(m))
	}
	cmd := dashboardapp.CreateCommand{
		Code:               req.GetDashboardCode(),
		Title:              req.GetDashboardTitle(),
		Description:        req.GetDescription(),
		FilterType:         req.GetFilterType(),
		FilterGroup1:       req.GetFilterGroup_1(),
		PeriodGrain:        periodeGrainToString(req.GetPeriodeGrain()),
		DefaultPeriod:      req.GetDefaultPeriod(),
		ChartType:          chartTypeToString(req.GetChartType()),
		ChartConfigRaw:     structToMap(req.GetChartConfig()),
		LayoutConfigRaw:    structToMap(req.GetLayoutConfig()),
		KpiConfigRaw:       structListToMaps(req.GetKpiConfig()),
		CompareModes:       compareModes,
		DrillEnabled:       req.GetDrillEnabled(),
		MaxDrillLevel:      int(req.GetMaxDrillLevel()),
		CacheTTLSec:        int(req.GetCacheTtlSec()),
		RefreshIntervalSec: int(req.GetRefreshIntervalSec()),
		DisplayOrder:       int(req.GetDisplayOrder()),
		GroupID:            uuidFromString(req.GetGroupId()),
		AllowedRoleCodes:   req.GetAllowedRoleCodes(),
		IsActive:           req.GetIsActive(),
		CreatedBy:          userUUIDFromContext(userID),
	}
	d, err := h.createHandler.Handle(ctx, cmd)
	if err != nil {
		return &financev1.CreateDashboardResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.CreateDashboardResponse{
		Base: successResponse("Dashboard created successfully"),
		Data: dashboardToProto(d),
	}, nil
}

// GetDashboard returns a dashboard by ID.
func (h *BIDashboardHandler) GetDashboard(ctx context.Context, req *financev1.GetDashboardRequest) (*financev1.GetDashboardResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.GetDashboardResponse{Base: baseResp}, nil
	}
	d, err := h.getHandler.HandleByID(ctx, dashboardapp.GetByIDQuery{ID: uuidFromString(req.GetDashboardId())})
	if err != nil {
		return &financev1.GetDashboardResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.GetDashboardResponse{
		Base: successResponse("Dashboard retrieved"),
		Data: dashboardToProto(d),
	}, nil
}

// GetDashboardByCode returns a dashboard by its business code.
func (h *BIDashboardHandler) GetDashboardByCode(ctx context.Context, req *financev1.GetDashboardByCodeRequest) (*financev1.GetDashboardByCodeResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.GetDashboardByCodeResponse{Base: baseResp}, nil
	}
	d, err := h.getHandler.HandleByCode(ctx, dashboardapp.GetByCodeQuery{Code: req.GetDashboardCode()})
	if err != nil {
		return &financev1.GetDashboardByCodeResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.GetDashboardByCodeResponse{
		Base: successResponse("Dashboard retrieved"),
		Data: dashboardToProto(d),
	}, nil
}

// ListDashboards returns paginated dashboards.
func (h *BIDashboardHandler) ListDashboards(ctx context.Context, req *financev1.ListDashboardsRequest) (*financev1.ListDashboardsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.ListDashboardsResponse{Base: baseResp}, nil
	}
	q := dashboardapp.ListQuery{
		Search:          req.GetSearch(),
		IncludeInactive: req.GetIncludeInactive(),
		Page:            int(req.GetPage()),
		PageSize:        int(req.GetPageSize()),
		SortField:       req.GetSortBy(),
		SortDir:         req.GetSortOrder(),
		FilterType:      req.GetFilterType(),
	}
	if gid := uuidFromString(req.GetGroupId()); gid != [16]byte{} {
		g := gid
		q.GroupID = &g
	}
	result, err := h.listHandler.Handle(ctx, q)
	if err != nil {
		return &financev1.ListDashboardsResponse{Base: biDomainErrorToBase(err)}, nil
	}
	items := make([]*financev1.Dashboard, 0, len(result.Items))
	for _, d := range result.Items {
		items = append(items, dashboardToProto(d))
	}
	return &financev1.ListDashboardsResponse{
		Base:       successResponse("Dashboards listed"),
		Data:       items,
		Pagination: paginationResponse(q.Page, q.PageSize, result.Total),
	}, nil
}

// UpdateDashboard mutates a dashboard.
func (h *BIDashboardHandler) UpdateDashboard(ctx context.Context, req *financev1.UpdateDashboardRequest) (*financev1.UpdateDashboardResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateDashboardResponse{Base: baseResp}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	cmd := dashboardapp.UpdateCommand{
		ID:              uuidFromString(req.GetDashboardId()),
		ChartConfigRaw:  structToMap(req.GetChartConfig()),
		LayoutConfigRaw: structToMap(req.GetLayoutConfig()),
		KpiConfigRaw:    structListToMaps(req.GetKpiConfig()),
		UpdatedBy:       userUUIDFromContext(userID),
	}
	if req.DashboardTitle != nil {
		t := req.GetDashboardTitle()
		cmd.Title = &t
	}
	if req.Description != nil {
		d := req.GetDescription()
		cmd.Description = &d
	}
	if req.FilterType != nil {
		v := req.GetFilterType()
		cmd.FilterType = &v
	}
	if req.FilterGroup_1 != nil {
		v := req.GetFilterGroup_1()
		cmd.FilterGroup1 = &v
	}
	if req.PeriodeGrain != nil {
		v := periodeGrainToString(req.GetPeriodeGrain())
		cmd.PeriodGrain = &v
	}
	if req.DefaultPeriod != nil {
		v := req.GetDefaultPeriod()
		cmd.DefaultPeriod = &v
	}
	if req.ChartType != nil {
		v := chartTypeToString(req.GetChartType())
		cmd.ChartType = &v
	}
	if req.DrillEnabled != nil {
		v := req.GetDrillEnabled()
		cmd.DrillEnabled = &v
	}
	if req.MaxDrillLevel != nil {
		v := int(req.GetMaxDrillLevel())
		cmd.MaxDrillLevel = &v
	}
	if req.CacheTtlSec != nil {
		v := int(req.GetCacheTtlSec())
		cmd.CacheTTLSec = &v
	}
	if req.RefreshIntervalSec != nil {
		v := int(req.GetRefreshIntervalSec())
		cmd.RefreshIntervalSec = &v
	}
	if req.DisplayOrder != nil {
		v := int(req.GetDisplayOrder())
		cmd.DisplayOrder = &v
	}
	if req.GroupId != nil {
		gid := uuidFromString(req.GetGroupId())
		cmd.GroupID = &gid
	}
	if req.IsActive != nil {
		v := req.GetIsActive()
		cmd.IsActive = &v
	}
	if len(req.GetCompareModes()) > 0 {
		modes := make([]string, 0, len(req.GetCompareModes()))
		for _, m := range req.GetCompareModes() {
			modes = append(modes, compareModeToString(m))
		}
		cmd.CompareModes = modes
	}
	d, err := h.updateHandler.Handle(ctx, cmd)
	if err != nil {
		return &financev1.UpdateDashboardResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.UpdateDashboardResponse{
		Base: successResponse("Dashboard updated"),
		Data: dashboardToProto(d),
	}, nil
}

// DeleteDashboard soft-deletes a dashboard.
func (h *BIDashboardHandler) DeleteDashboard(ctx context.Context, req *financev1.DeleteDashboardRequest) (*financev1.DeleteDashboardResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.DeleteDashboardResponse{Base: baseResp}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	if err := h.deleteHandler.Handle(ctx, dashboardapp.DeleteCommand{
		ID:        uuidFromString(req.GetDashboardId()),
		DeletedBy: userUUIDFromContext(userID),
	}); err != nil {
		return &financev1.DeleteDashboardResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.DeleteDashboardResponse{Base: successResponse("Dashboard deleted")}, nil
}

// DuplicateDashboard clones an existing dashboard.
func (h *BIDashboardHandler) DuplicateDashboard(ctx context.Context, req *financev1.DuplicateDashboardRequest) (*financev1.DuplicateDashboardResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.DuplicateDashboardResponse{Base: baseResp}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	d, err := h.duplicateHandler.Handle(ctx, dashboardapp.DuplicateCommand{
		SourceID:  uuidFromString(req.GetDashboardId()),
		NewCode:   req.GetNewCode(),
		NewTitle:  req.GetNewTitle(),
		CreatedBy: userUUIDFromContext(userID),
	})
	if err != nil {
		return &financev1.DuplicateDashboardResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.DuplicateDashboardResponse{
		Base: successResponse("Dashboard duplicated"),
		Data: dashboardToProto(d),
	}, nil
}

// SetDashboardRoles overwrites the role whitelist.
func (h *BIDashboardHandler) SetDashboardRoles(ctx context.Context, req *financev1.SetDashboardRolesRequest) (*financev1.SetDashboardRolesResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.SetDashboardRolesResponse{Base: baseResp}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	roles, err := h.setRolesHandler.Handle(ctx, dashboardapp.SetRolesCommand{
		DashboardID: uuidFromString(req.GetDashboardId()),
		RoleCodes:   req.GetRoleCodes(),
		UpdatedBy:   userUUIDFromContext(userID),
	})
	if err != nil {
		return &financev1.SetDashboardRolesResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.SetDashboardRolesResponse{
		Base:      successResponse("Roles updated"),
		RoleCodes: roles,
	}, nil
}

// ListAccessibleDashboards returns dashboards the calling user can see.
func (h *BIDashboardHandler) ListAccessibleDashboards(ctx context.Context, _ *financev1.ListAccessibleDashboardsRequest) (*financev1.ListAccessibleDashboardsResponse, error) {
	roles := GetRolesFromCtx(ctx)
	out, err := h.listAccessibleHandler.Handle(ctx, dashboardapp.ListAccessibleQuery{
		UserRoles:    roles,
		IsSuperAdmin: IsSuperAdmin(ctx),
	})
	if err != nil {
		return &financev1.ListAccessibleDashboardsResponse{Base: biDomainErrorToBase(err)}, nil
	}
	items := make([]*financev1.Dashboard, 0, len(out))
	for _, d := range out {
		items = append(items, dashboardToProto(d))
	}
	return &financev1.ListAccessibleDashboardsResponse{
		Base: successResponse("Accessible dashboards listed"),
		Data: items,
	}, nil
}

// ----------- Group RPCs -----------

// CreateDashboardGroup creates a new group.
func (h *BIDashboardHandler) CreateDashboardGroup(ctx context.Context, req *financev1.CreateDashboardGroupRequest) (*financev1.CreateDashboardGroupResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.CreateDashboardGroupResponse{Base: baseResp}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	g, err := h.groupCreateHandler.Handle(ctx, groupapp.CreateCommand{
		Code:         req.GetGroupCode(),
		Name:         req.GetGroupName(),
		Description:  req.GetDescription(),
		Icon:         req.GetIcon(),
		DisplayOrder: int(req.GetDisplayOrder()),
		IsActive:     req.GetIsActive(),
		CreatedBy:    userUUIDFromContext(userID),
	})
	if err != nil {
		return &financev1.CreateDashboardGroupResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.CreateDashboardGroupResponse{
		Base: successResponse("Group created"),
		Data: groupToProto(g),
	}, nil
}

// ListDashboardGroups returns groups.
func (h *BIDashboardHandler) ListDashboardGroups(ctx context.Context, req *financev1.ListDashboardGroupsRequest) (*financev1.ListDashboardGroupsResponse, error) {
	gs, err := h.groupListHandler.Handle(ctx, req.GetIncludeInactive())
	if err != nil {
		return &financev1.ListDashboardGroupsResponse{Base: biDomainErrorToBase(err)}, nil
	}
	items := make([]*financev1.DashboardGroup, 0, len(gs))
	for _, g := range gs {
		items = append(items, groupToProto(g))
	}
	return &financev1.ListDashboardGroupsResponse{
		Base: successResponse("Groups listed"),
		Data: items,
	}, nil
}

// UpdateDashboardGroup mutates a group.
func (h *BIDashboardHandler) UpdateDashboardGroup(ctx context.Context, req *financev1.UpdateDashboardGroupRequest) (*financev1.UpdateDashboardGroupResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateDashboardGroupResponse{Base: baseResp}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	cmd := groupapp.UpdateCommand{
		ID:        uuidFromString(req.GetGroupId()),
		UpdatedBy: userUUIDFromContext(userID),
	}
	if req.GroupName != nil {
		v := req.GetGroupName()
		cmd.Name = &v
	}
	if req.Description != nil {
		v := req.GetDescription()
		cmd.Description = &v
	}
	if req.Icon != nil {
		v := req.GetIcon()
		cmd.Icon = &v
	}
	if req.DisplayOrder != nil {
		v := int(req.GetDisplayOrder())
		cmd.DisplayOrder = &v
	}
	if req.IsActive != nil {
		v := req.GetIsActive()
		cmd.IsActive = &v
	}
	g, err := h.groupUpdateHandler.Handle(ctx, cmd)
	if err != nil {
		return &financev1.UpdateDashboardGroupResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.UpdateDashboardGroupResponse{
		Base: successResponse("Group updated"),
		Data: groupToProto(g),
	}, nil
}

// DeleteDashboardGroup removes a group (refuses if in use).
func (h *BIDashboardHandler) DeleteDashboardGroup(ctx context.Context, req *financev1.DeleteDashboardGroupRequest) (*financev1.DeleteDashboardGroupResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.DeleteDashboardGroupResponse{Base: baseResp}, nil
	}
	if err := h.groupDeleteHandler.Handle(ctx, uuidFromString(req.GetGroupId())); err != nil {
		return &financev1.DeleteDashboardGroupResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.DeleteDashboardGroupResponse{Base: successResponse("Group deleted")}, nil
}

// Compile-time assertion this handler satisfies the gRPC interface.
var _ financev1.DashboardServiceServer = (*BIDashboardHandler)(nil)

// reference to dashboarddomain to suppress unused-import when only used via app package types
var _ = dashboarddomain.ListFilter{}
