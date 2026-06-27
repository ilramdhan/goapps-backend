// Package grpc provides gRPC server implementation for the finance service.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmachine "github.com/mutugading/goapps-backend/services/finance/internal/application/machine"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// MachineHandler implements financev1.MachineServiceServer.
type MachineHandler struct {
	financev1.UnimplementedMachineServiceServer
	createHandler    *appmachine.CreateHandler
	getHandler       *appmachine.GetHandler
	listHandler      *appmachine.ListHandler
	updateHandler    *appmachine.UpdateHandler
	deleteHandler    *appmachine.DeleteHandler
	validationHelper *ValidationHelper
}

// NewMachineHandler creates a new Machine gRPC handler.
func NewMachineHandler(repo machine.Repository) (*MachineHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MachineHandler{
		createHandler:    appmachine.NewCreateHandler(repo),
		getHandler:       appmachine.NewGetHandler(repo),
		listHandler:      appmachine.NewListHandler(repo),
		updateHandler:    appmachine.NewUpdateHandler(repo),
		deleteHandler:    appmachine.NewDeleteHandler(repo),
		validationHelper: v,
	}, nil
}

// CreateMachine creates a new machine record.
func (h *MachineHandler) CreateMachine(ctx context.Context, req *financev1.CreateMachineRequest) (*financev1.CreateMachineResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordMachineOperation("create", false)
		return &financev1.CreateMachineResponse{Base: baseResp}, nil
	}

	cmd := appmachine.CreateCommand{
		Code:               req.MachineCode,
		Name:               req.MachineName,
		MCType:             req.McType,
		Location:           req.McLocation,
		NoOfPosition:       int(req.NoOfPosition),
		NoOfEnd:            int(req.NoOfEnd),
		MCSpeed:            req.McSpeed,
		MachineRPM:         req.MachineRpm,
		MCEfficiency:       req.McEfficiency,
		PowerPerDay:        req.PowerPerDay,
		MpPerDay:           req.MpPerDay,
		OhsPerDay:          req.OhsPerDay,
		SparesPerDay:       req.SparesPerDay,
		KgsLostChange:      req.KgsLostChange,
		Vb1Qty:             req.Vb1Qty,
		Vb2Qty:             req.Vb2Qty,
		Vb3Qty:             req.Vb3Qty,
		Vb4Qty:             req.Vb4Qty,
		Vb5Qty:             req.Vb5Qty,
		McPoyBobbinWeight:  req.McPoyBobbinWeight,
		McTotFxdCst:        req.McTotFxdCst,
		McBobbinPerTrolly:  req.McBobbinPerTrolly,
		McBoxCost:          req.McBoxCost,
		McCaptivePerBobbin: req.McCaptivePerBobbin,
		McWeightage:        req.McWeightage,
		Notes:              req.Notes,
		CreatedBy:          getUserFromContext(ctx),
	}

	entity, err := h.createHandler.Handle(ctx, cmd)
	if err != nil {
		RecordMachineOperation("create", false)
		return &financev1.CreateMachineResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMachineOperation("create", true)
	return &financev1.CreateMachineResponse{
		Base: successResponse("Machine created successfully"),
		Data: machineEntityToProto(entity),
	}, nil
}

// GetMachine retrieves a machine by ID.
func (h *MachineHandler) GetMachine(ctx context.Context, req *financev1.GetMachineRequest) (*financev1.GetMachineResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordMachineOperation("get", false)
		return &financev1.GetMachineResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MachineId)
	if err != nil {
		RecordMachineOperation("get", false)
		return &financev1.GetMachineResponse{Base: invalidIDResponse("machine_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.getHandler.Handle(ctx, appmachine.GetQuery{MachineID: id})
	if err != nil {
		RecordMachineOperation("get", false)
		return &financev1.GetMachineResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMachineOperation("get", true)
	return &financev1.GetMachineResponse{
		Base: successResponse("Machine retrieved successfully"),
		Data: machineEntityToProto(entity),
	}, nil
}

// UpdateMachine updates an existing machine record.
//
//nolint:gocognit // Multiple optional numeric fields require sequential nil checks.
func (h *MachineHandler) UpdateMachine(ctx context.Context, req *financev1.UpdateMachineRequest) (*financev1.UpdateMachineResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordMachineOperation("update", false)
		return &financev1.UpdateMachineResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MachineId)
	if err != nil {
		RecordMachineOperation("update", false)
		return &financev1.UpdateMachineResponse{Base: invalidIDResponse("machine_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	cmd := appmachine.UpdateCommand{
		MachineID:          id,
		Name:               req.MachineName,
		MCType:             req.McType,
		Location:           req.McLocation,
		IsActive:           req.IsActive,
		Notes:              req.Notes,
		MachineRPM:         req.MachineRpm,
		PowerPerDay:        req.PowerPerDay,
		MpPerDay:           req.MpPerDay,
		OhsPerDay:          req.OhsPerDay,
		SparesPerDay:       req.SparesPerDay,
		KgsLostChange:      req.KgsLostChange,
		Vb1Qty:             req.Vb1Qty,
		Vb2Qty:             req.Vb2Qty,
		Vb3Qty:             req.Vb3Qty,
		Vb4Qty:             req.Vb4Qty,
		Vb5Qty:             req.Vb5Qty,
		McPoyBobbinWeight:  req.McPoyBobbinWeight,
		McTotFxdCst:        req.McTotFxdCst,
		McBobbinPerTrolly:  req.McBobbinPerTrolly,
		McBoxCost:          req.McBoxCost,
		McCaptivePerBobbin: req.McCaptivePerBobbin,
		McWeightage:        req.McWeightage,
		UpdatedBy:          getUserFromContext(ctx),
	}
	if req.NoOfPosition != nil {
		v := int(*req.NoOfPosition)
		cmd.NoOfPosition = &v
	}
	if req.NoOfEnd != nil {
		v := int(*req.NoOfEnd)
		cmd.NoOfEnd = &v
	}
	if req.McSpeed != nil {
		cmd.MCSpeed = req.McSpeed
	}
	if req.McEfficiency != nil {
		cmd.MCEfficiency = req.McEfficiency
	}

	entity, err := h.updateHandler.Handle(ctx, cmd)
	if err != nil {
		RecordMachineOperation("update", false)
		return &financev1.UpdateMachineResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMachineOperation("update", true)
	return &financev1.UpdateMachineResponse{
		Base: successResponse("Machine updated successfully"),
		Data: machineEntityToProto(entity),
	}, nil
}

// DeleteMachine soft-deletes a machine record.
func (h *MachineHandler) DeleteMachine(ctx context.Context, req *financev1.DeleteMachineRequest) (*financev1.DeleteMachineResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordMachineOperation("delete", false)
		return &financev1.DeleteMachineResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MachineId)
	if err != nil {
		RecordMachineOperation("delete", false)
		return &financev1.DeleteMachineResponse{Base: invalidIDResponse("machine_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	if err := h.deleteHandler.Handle(ctx, appmachine.DeleteCommand{MachineID: id, DeletedBy: getUserFromContext(ctx)}); err != nil {
		RecordMachineOperation("delete", false)
		return &financev1.DeleteMachineResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMachineOperation("delete", true)
	return &financev1.DeleteMachineResponse{Base: successResponse("Machine deleted successfully")}, nil
}

// ListMachines lists machine records with search, filter, and pagination.
func (h *MachineHandler) ListMachines(ctx context.Context, req *financev1.ListMachinesRequest) (*financev1.ListMachinesResponse, error) {
	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := appmachine.ListQuery{
		Page:      page,
		PageSize:  pageSize,
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		t := true
		query.IsActive = &t
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		f := false
		query.IsActive = &f
	default:
	}

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		RecordMachineOperation("list", false)
		return &financev1.ListMachinesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMachineOperation("list", true)

	items := make([]*financev1.Machine, len(result.Machines))
	for i, e := range result.Machines {
		items[i] = machineEntityToProto(e)
	}

	return &financev1.ListMachinesResponse{
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

// ExportMachines is not yet implemented.
func (h *MachineHandler) ExportMachines(_ context.Context, _ *financev1.ExportMachinesRequest) (*financev1.ExportMachinesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ExportMachines not implemented")
}

// ImportMachines is not yet implemented.
func (h *MachineHandler) ImportMachines(_ context.Context, _ *financev1.ImportMachinesRequest) (*financev1.ImportMachinesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ImportMachines not implemented")
}

// DownloadMachineTemplate is not yet implemented.
func (h *MachineHandler) DownloadMachineTemplate(_ context.Context, _ *financev1.DownloadMachineTemplateRequest) (*financev1.DownloadMachineTemplateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DownloadMachineTemplate not implemented")
}

// machineEntityToProto converts a domain Machine entity to its proto representation.
func machineEntityToProto(e *machine.Entity) *financev1.Machine {
	p := &financev1.Machine{
		MachineId:          e.ID().String(),
		MachineCode:        e.Code(),
		MachineName:        e.Name(),
		McType:             e.MCType(),
		McLocation:         e.Location(),
		NoOfPosition:       safeconv.IntToInt32(e.NoOfPosition()),
		NoOfEnd:            safeconv.IntToInt32(e.NoOfEnd()),
		McSpeed:            e.MCSpeed(),
		MachineRpm:         e.MachineRPM(),
		McEfficiency:       e.MCEfficiency(),
		PowerPerDay:        e.PowerPerDay(),
		MpPerDay:           e.MpPerDay(),
		OhsPerDay:          e.OhsPerDay(),
		SparesPerDay:       e.SparesPerDay(),
		KgsLostChange:      e.KgsLostChange(),
		Vb1Qty:             e.Vb1Qty(),
		Vb2Qty:             e.Vb2Qty(),
		Vb3Qty:             e.Vb3Qty(),
		Vb4Qty:             e.Vb4Qty(),
		Vb5Qty:             e.Vb5Qty(),
		McPoyBobbinWeight:  e.McPoyBobbinWeight(),
		McTotFxdCst:        e.McTotFxdCst(),
		McBobbinPerTrolly:  e.McBobbinPerTrolly(),
		McBoxCost:          e.McBoxCost(),
		McCaptivePerBobbin: e.McCaptivePerBobbin(),
		McWeightage:        e.McWeightage(),
		IsActive:           e.IsActive(),
		Notes:              e.Notes(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.UpdatedAt() != nil {
		p.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		p.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return p
}
