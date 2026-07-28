// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	thresholdapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/threshold"
	thresholddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/threshold"
)

// thresholdHandler implements the overrun-threshold-config RPCs of PPCService.
type thresholdHandler struct {
	svc *thresholdapp.Service
}

func newThresholdHandler(svc *thresholdapp.Service) *thresholdHandler {
	return &thresholdHandler{svc: svc}
}

// CreateOverrunThresholdConfig creates a new overrun threshold config.
func (h *thresholdHandler) CreateOverrunThresholdConfig(ctx context.Context, req *ppcv1.CreateOverrunThresholdConfigRequest) (*ppcv1.CreateOverrunThresholdConfigResponse, error) {
	warning, base := decimalField("warning_value", req.GetWarningValue())
	if base != nil {
		return &ppcv1.CreateOverrunThresholdConfigResponse{Base: base}, nil
	}
	block, base := decimalField("block_value", req.GetBlockValue())
	if base != nil {
		return &ppcv1.CreateOverrunThresholdConfigResponse{Base: base}, nil
	}

	entity, err := h.svc.Create(ctx, thresholdapp.CreateCommand{
		Level:        thresholdLevelToString(req.GetLevel()),
		RefID:        req.RefId,
		Unit:         thresholdUnitToString(req.GetThresholdUnit()),
		WarningValue: warning,
		BlockValue:   block,
		Notes:        req.GetNotes(),
		CreatedBy:    getUserFromContext(ctx),
	})
	if err != nil {
		return &ppcv1.CreateOverrunThresholdConfigResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateOverrunThresholdConfigResponse{
		Base: successResponse("Overrun threshold config created successfully"),
		Data: thresholdConfigToProto(entity),
	}, nil
}

// GetOverrunThresholdConfig retrieves an overrun threshold config by ID.
func (h *thresholdHandler) GetOverrunThresholdConfig(ctx context.Context, req *ppcv1.GetOverrunThresholdConfigRequest) (*ppcv1.GetOverrunThresholdConfigResponse, error) {
	entity, err := h.svc.Get(ctx, req.GetThresholdId())
	if err != nil {
		return &ppcv1.GetOverrunThresholdConfigResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetOverrunThresholdConfigResponse{
		Base: successResponse("Overrun threshold config retrieved successfully"),
		Data: thresholdConfigToProto(entity),
	}, nil
}

// UpdateOverrunThresholdConfig updates an existing overrun threshold config.
func (h *thresholdHandler) UpdateOverrunThresholdConfig(ctx context.Context, req *ppcv1.UpdateOverrunThresholdConfigRequest) (*ppcv1.UpdateOverrunThresholdConfigResponse, error) {
	cmd := thresholdapp.UpdateCommand{
		ID:        req.GetThresholdId(),
		Notes:     req.Notes,
		IsActive:  req.IsActive,
		UpdatedBy: getUserFromContext(ctx),
	}
	if req.ThresholdUnit != nil {
		u := thresholdUnitToString(req.GetThresholdUnit())
		cmd.Unit = &u
	}
	if req.WarningValue != nil {
		v, base := decimalField("warning_value", req.GetWarningValue())
		if base != nil {
			return &ppcv1.UpdateOverrunThresholdConfigResponse{Base: base}, nil
		}
		cmd.WarningValue = &v
	}
	if req.BlockValue != nil {
		v, base := decimalField("block_value", req.GetBlockValue())
		if base != nil {
			return &ppcv1.UpdateOverrunThresholdConfigResponse{Base: base}, nil
		}
		cmd.BlockValue = &v
	}

	entity, err := h.svc.Update(ctx, cmd)
	if err != nil {
		return &ppcv1.UpdateOverrunThresholdConfigResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateOverrunThresholdConfigResponse{
		Base: successResponse("Overrun threshold config updated successfully"),
		Data: thresholdConfigToProto(entity),
	}, nil
}

// DeleteOverrunThresholdConfig deletes an overrun threshold config.
func (h *thresholdHandler) DeleteOverrunThresholdConfig(ctx context.Context, req *ppcv1.DeleteOverrunThresholdConfigRequest) (*ppcv1.DeleteOverrunThresholdConfigResponse, error) {
	if err := h.svc.Delete(ctx, req.GetThresholdId()); err != nil {
		return &ppcv1.DeleteOverrunThresholdConfigResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeleteOverrunThresholdConfigResponse{Base: successResponse("Overrun threshold config deleted successfully")}, nil
}

// ListOverrunThresholdConfigs lists overrun threshold configs with filtering and pagination.
func (h *thresholdHandler) ListOverrunThresholdConfigs(ctx context.Context, req *ppcv1.ListOverrunThresholdConfigsRequest) (*ppcv1.ListOverrunThresholdConfigsResponse, error) {
	result, err := h.svc.List(ctx, thresholdapp.ListQuery{
		Page:      int(req.GetPage()),
		PageSize:  int(req.GetPageSize()),
		Level:     thresholdLevelToString(req.GetLevel()),
		IsActive:  activeFilterToBool(req.GetActiveFilter()),
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListOverrunThresholdConfigsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*ppcv1.OverrunThresholdConfig, len(result.Items))
	for i, entity := range result.Items {
		items[i] = thresholdConfigToProto(entity)
	}
	return &ppcv1.ListOverrunThresholdConfigsResponse{
		Base: successResponse("Overrun threshold configs retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

func thresholdConfigToProto(e *thresholddomain.Config) *ppcv1.OverrunThresholdConfig {
	proto := &ppcv1.OverrunThresholdConfig{
		ThresholdId:   e.ID(),
		Level:         stringToThresholdLevel(e.Level()),
		ThresholdUnit: stringToThresholdUnit(e.Unit()),
		WarningValue:  formatDecimal(e.WarningValue()),
		BlockValue:    formatDecimal(e.BlockValue()),
		Notes:         e.Notes(),
		IsActive:      e.IsActive(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.RefID() != nil {
		proto.RefId = *e.RefID()
	}
	if e.UpdatedAt() != nil {
		proto.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		proto.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return proto
}
