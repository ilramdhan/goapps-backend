// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	machineapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/machine"
	"github.com/mutugading/goapps-backend/services/ppc/internal/application/machinesync"
	machinedomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// machineHandler implements the machine RPCs of PPCService.
type machineHandler struct {
	svc  *machineapp.Service
	sync *machinesync.Usecase
}

func newMachineHandler(svc *machineapp.Service, sync *machinesync.Usecase) *machineHandler {
	return &machineHandler{svc: svc, sync: sync}
}

// GetMachine retrieves a machine by ID.
func (h *machineHandler) GetMachine(ctx context.Context, req *ppcv1.GetMachineRequest) (*ppcv1.GetMachineResponse, error) {
	entity, err := h.svc.Get(ctx, req.GetMachineId())
	if err != nil {
		return &ppcv1.GetMachineResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetMachineResponse{
		Base: successResponse("Machine retrieved successfully"),
		Data: machineToProto(entity),
	}, nil
}

// UpdateMachine updates the PPC-local fields of an existing machine.
func (h *machineHandler) UpdateMachine(ctx context.Context, req *ppcv1.UpdateMachineRequest) (*ppcv1.UpdateMachineResponse, error) {
	cmd := machineapp.UpdateCommand{
		ID:        req.GetMachineId(),
		Line:      req.MachineLine,
		GroupID:   req.MachineGroupId,
		IsActive:  req.MachineIsActive,
		OrionCode: req.MachineOrionCode,
		UpdatedBy: getUserFromContext(ctx),
	}
	if req.MachineArea != nil {
		a := areaCodeToString(req.GetMachineArea())
		cmd.Area = &a
	}
	if req.MachineDoffWeightKg != nil {
		weight, base := optionalDecimalField("machine_doff_weight_kg", *req.MachineDoffWeightKg)
		if base != nil {
			return &ppcv1.UpdateMachineResponse{Base: base}, nil
		}
		cmd.DoffWeightKg = weight
	}

	entity, err := h.svc.Update(ctx, cmd)
	if err != nil {
		return &ppcv1.UpdateMachineResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateMachineResponse{
		Base: successResponse("Machine updated successfully"),
		Data: machineToProto(entity),
	}, nil
}

// ListMachines lists machines with filtering and pagination.
func (h *machineHandler) ListMachines(ctx context.Context, req *ppcv1.ListMachinesRequest) (*ppcv1.ListMachinesResponse, error) {
	result, err := h.svc.List(ctx, machineapp.ListQuery{
		Page:           int(req.GetPage()),
		PageSize:       int(req.GetPageSize()),
		Search:         req.GetSearch(),
		Area:           areaCodeToString(req.GetArea()),
		MachineGroupID: req.MachineGroupId,
		IsActive:       activeFilterToBool(req.GetActiveFilter()),
		SortBy:         req.GetSortBy(),
		SortOrder:      req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListMachinesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.Machine, len(result.Items))
	for i, entity := range result.Items {
		items[i] = machineToProto(entity)
	}
	return &ppcv1.ListMachinesResponse{
		Base: successResponse("Machines retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// SyncMachines triggers an on-demand machine sync from finance + Oracle.
func (h *machineHandler) SyncMachines(ctx context.Context, _ *ppcv1.SyncMachinesRequest) (*ppcv1.SyncMachinesResponse, error) {
	if h.sync == nil {
		return &ppcv1.SyncMachinesResponse{Base: errorResponse("503", "machine sync is not configured")}, nil
	}
	res, err := h.sync.Sync(ctx)
	if err != nil {
		return &ppcv1.SyncMachinesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.SyncMachinesResponse{
		Base:          successResponse("Machine sync completed"),
		InsertedCount: safeconv.IntToInt32(res.Inserted),
		UpdatedCount:  safeconv.IntToInt32(res.Updated),
		SkippedCount:  safeconv.IntToInt32(res.Skipped),
	}, nil
}

func machineToProto(e *machinedomain.Machine) *ppcv1.Machine {
	proto := &ppcv1.Machine{
		MachineId:           e.ID(),
		MachineNo:           e.No(),
		MachineArea:         stringToAreaCode(e.Area().String()),
		MachineGroupName:    e.GroupName(),
		MachineDoffWeightKg: formatOptionalDecimal(e.DoffWeightKg()),
		MachineIsActive:     e.IsActive(),
		MachineOrionCode:    e.OrionCode(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.Line() != nil {
		proto.MachineLine = *e.Line()
	}
	if e.GroupID() != nil {
		proto.MachineGroupId = *e.GroupID()
	}
	if e.SourceMcID() != nil {
		proto.SourceMcId = *e.SourceMcID()
	}
	if e.SyncedAt() != nil {
		proto.SyncedAt = e.SyncedAt().Format(time.RFC3339)
	}
	if e.UpdatedAt() != nil {
		proto.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		proto.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return proto
}
