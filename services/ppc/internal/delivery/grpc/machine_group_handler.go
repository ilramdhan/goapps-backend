// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	app "github.com/mutugading/goapps-backend/services/ppc/internal/application/machinegroup"
	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/machinegroup"
)

// machineGroupHandler implements the machine-group RPCs of PPCService.
type machineGroupHandler struct {
	svc *app.Service
}

func newMachineGroupHandler(svc *app.Service) *machineGroupHandler {
	return &machineGroupHandler{svc: svc}
}

// CreateMachineGroup creates a new machine group.
func (h *machineGroupHandler) CreateMachineGroup(ctx context.Context, req *ppcv1.CreateMachineGroupRequest) (*ppcv1.CreateMachineGroupResponse, error) {
	entity, err := h.svc.Create(ctx, app.CreateCommand{
		Name:      req.GetGroupName(),
		Area:      areaCodeToString(req.GetGroupArea()),
		CreatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.CreateMachineGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateMachineGroupResponse{
		Base: successResponse("Machine group created successfully"),
		Data: machineGroupToProto(entity),
	}, nil
}

// GetMachineGroup retrieves a machine group by ID.
func (h *machineGroupHandler) GetMachineGroup(ctx context.Context, req *ppcv1.GetMachineGroupRequest) (*ppcv1.GetMachineGroupResponse, error) {
	entity, err := h.svc.Get(ctx, req.GetGroupId())
	if err != nil {
		return &ppcv1.GetMachineGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetMachineGroupResponse{
		Base: successResponse("Machine group retrieved successfully"),
		Data: machineGroupToProto(entity),
	}, nil
}

// UpdateMachineGroup updates an existing machine group.
func (h *machineGroupHandler) UpdateMachineGroup(ctx context.Context, req *ppcv1.UpdateMachineGroupRequest) (*ppcv1.UpdateMachineGroupResponse, error) {
	cmd := app.UpdateCommand{
		ID:        req.GetGroupId(),
		Name:      req.GroupName,
		UpdatedBy: getUserFromContext(ctx),
	}
	if req.GroupArea != nil {
		a := areaCodeToString(req.GetGroupArea())
		cmd.Area = &a
	}
	entity, err := h.svc.Update(ctx, cmd)
	if err != nil {
		return &ppcv1.UpdateMachineGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateMachineGroupResponse{
		Base: successResponse("Machine group updated successfully"),
		Data: machineGroupToProto(entity),
	}, nil
}

// DeleteMachineGroup deletes a machine group.
func (h *machineGroupHandler) DeleteMachineGroup(ctx context.Context, req *ppcv1.DeleteMachineGroupRequest) (*ppcv1.DeleteMachineGroupResponse, error) {
	if err := h.svc.Delete(ctx, req.GetGroupId()); err != nil {
		return &ppcv1.DeleteMachineGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeleteMachineGroupResponse{Base: successResponse("Machine group deleted successfully")}, nil
}

// ListMachineGroups lists machine groups with filtering and pagination.
func (h *machineGroupHandler) ListMachineGroups(ctx context.Context, req *ppcv1.ListMachineGroupsRequest) (*ppcv1.ListMachineGroupsResponse, error) {
	result, err := h.svc.List(ctx, app.ListQuery{
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		Search:    req.GetSearch(),
		Area:      areaCodeToString(req.GetArea()),
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListMachineGroupsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.MachineGroup, len(result.Items))
	for i, entity := range result.Items {
		items[i] = machineGroupToProto(entity)
	}
	return &ppcv1.ListMachineGroupsResponse{
		Base: successResponse("Machine groups retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

func machineGroupToProto(e *machinegroup.MachineGroup) *ppcv1.MachineGroup {
	proto := &ppcv1.MachineGroup{
		GroupId:   e.ID(),
		GroupName: e.Name(),
		GroupArea: stringToAreaCode(e.Area().String()),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.UpdatedAt() != nil {
		proto.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		proto.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return proto
}
