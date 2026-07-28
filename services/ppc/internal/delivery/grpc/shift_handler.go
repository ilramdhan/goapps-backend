// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	shiftapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/shift"
	shiftdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/shift"
)

// shiftHandler implements the PpcShift RPCs of PPCService.
type shiftHandler struct {
	svc *shiftapp.Service
}

func newShiftHandler(svc *shiftapp.Service) *shiftHandler {
	return &shiftHandler{svc: svc}
}

// CreatePpcShift creates a new shift.
func (h *shiftHandler) CreatePpcShift(ctx context.Context, req *ppcv1.CreatePpcShiftRequest) (*ppcv1.CreatePpcShiftResponse, error) {
	entity, err := h.svc.Create(ctx, shiftapp.CreateCommand{
		Code:      req.GetCode(),
		Name:      req.GetName(),
		StartTime: req.GetStartTime(),
		EndTime:   req.GetEndTime(),
		CreatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.CreatePpcShiftResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreatePpcShiftResponse{
		Base: successResponse("Shift created successfully"),
		Data: shiftToProto(entity),
	}, nil
}

// GetPpcShift retrieves a shift by ID.
func (h *shiftHandler) GetPpcShift(ctx context.Context, req *ppcv1.GetPpcShiftRequest) (*ppcv1.GetPpcShiftResponse, error) {
	entity, err := h.svc.Get(ctx, req.GetShiftId())
	if err != nil {
		return &ppcv1.GetPpcShiftResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetPpcShiftResponse{
		Base: successResponse("Shift retrieved successfully"),
		Data: shiftToProto(entity),
	}, nil
}

// UpdatePpcShift updates an existing shift.
func (h *shiftHandler) UpdatePpcShift(ctx context.Context, req *ppcv1.UpdatePpcShiftRequest) (*ppcv1.UpdatePpcShiftResponse, error) {
	entity, err := h.svc.Update(ctx, shiftapp.UpdateCommand{
		ID:        req.GetShiftId(),
		Name:      req.Name,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		IsActive:  req.IsActive,
		UpdatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.UpdatePpcShiftResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdatePpcShiftResponse{
		Base: successResponse("Shift updated successfully"),
		Data: shiftToProto(entity),
	}, nil
}

// DeletePpcShift deletes a shift.
func (h *shiftHandler) DeletePpcShift(ctx context.Context, req *ppcv1.DeletePpcShiftRequest) (*ppcv1.DeletePpcShiftResponse, error) {
	if err := h.svc.Delete(ctx, req.GetShiftId()); err != nil {
		return &ppcv1.DeletePpcShiftResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeletePpcShiftResponse{Base: successResponse("Shift deleted successfully")}, nil
}

// ListPpcShifts lists shifts with filtering and pagination.
func (h *shiftHandler) ListPpcShifts(ctx context.Context, req *ppcv1.ListPpcShiftsRequest) (*ppcv1.ListPpcShiftsResponse, error) {
	result, err := h.svc.List(ctx, shiftapp.ListQuery{
		Page:     int(req.GetPage()),
		PageSize: int(req.GetPageSize()),
		IsActive: activeFilterToBool(req.GetActiveFilter()),
	})
	if err != nil {
		return &ppcv1.ListPpcShiftsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.PpcShift, len(result.Items))
	for i, entity := range result.Items {
		items[i] = shiftToProto(entity)
	}
	return &ppcv1.ListPpcShiftsResponse{
		Base: successResponse("Shifts retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

func shiftToProto(e *shiftdomain.Shift) *ppcv1.PpcShift {
	proto := &ppcv1.PpcShift{
		ShiftId:   e.ID(),
		Code:      e.Code(),
		Name:      e.Name(),
		StartTime: e.StartTime(),
		EndTime:   e.EndTime(),
		IsActive:  e.IsActive(),
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
