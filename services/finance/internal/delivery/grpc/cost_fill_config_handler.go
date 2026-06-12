package grpc

import (
	"context"
	"errors"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costfillassignment"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// CostFillConfigHandler implements financev1.CostLevelAssignmentConfigServiceServer.
type CostFillConfigHandler struct {
	financev1.UnimplementedCostLevelAssignmentConfigServiceServer
	upsertGlobal   *app.UpsertGlobalConfigHandler
	upsertOverride *app.UpsertOverrideHandler
	deleteGlobal   *app.DeleteGlobalConfigHandler
	listGlobal     *app.ListGlobalConfigHandler
}

// NewCostFillConfigHandler constructs the handler.
func NewCostFillConfigHandler(
	upsertGlobal *app.UpsertGlobalConfigHandler,
	upsertOverride *app.UpsertOverrideHandler,
	deleteGlobal *app.DeleteGlobalConfigHandler,
	listGlobal *app.ListGlobalConfigHandler,
) *CostFillConfigHandler {
	return &CostFillConfigHandler{
		upsertGlobal:   upsertGlobal,
		upsertOverride: upsertOverride,
		deleteGlobal:   deleteGlobal,
		listGlobal:     listGlobal,
	}
}

// UpsertLevelConfig creates or replaces an assignment config for a level+tier.
func (h *CostFillConfigHandler) UpsertLevelConfig(ctx context.Context, req *financev1.UpsertLevelConfigRequest) (*financev1.UpsertLevelConfigResponse, error) {
	actor := actorFromCtx(ctx)

	tier := req.GetTier()
	switch tier {
	case "GLOBAL":
		fillerType := req.GetFillerType()
		fillerValue := req.GetFillerValue()
		approverType := req.GetApproverType()
		approverValue := req.GetApproverValue()
		reapprove := req.GetReapproveOnChange()
		slaFill := req.GetSlaFillHours()
		slaApprove := req.GetSlaApproveHours()

		cmd := app.UpsertGlobalConfigCommand{
			RouteLevel:        req.GetRouteLevel(),
			FillerType:        fillerType,
			FillerValue:       fillerValue,
			ReapproveOnChange: &reapprove,
			SLAFillHours:      &slaFill,
			SLAApproveHours:   &slaApprove,
			Actor:             actor,
		}
		if approverType != "" {
			cmd.ApproverType = &approverType
		}
		if approverValue != "" {
			cmd.ApproverValue = &approverValue
		}
		if err := h.upsertGlobal.Handle(ctx, cmd); err != nil {
			return &financev1.UpsertLevelConfigResponse{Base: fillConfigErrToBase(err)}, nil
		}
	case "PRODUCT", "REQUEST":
		fillerType := req.GetFillerType()
		fillerValue := req.GetFillerValue()
		approverType := req.GetApproverType()
		approverValue := req.GetApproverValue()
		reapprove := req.GetReapproveOnChange()
		slaFill := req.GetSlaFillHours()
		slaApprove := req.GetSlaApproveHours()

		overrideTier := app.OverrideTierProduct
		if tier == "REQUEST" {
			overrideTier = app.OverrideTierRequest
		}
		cmd := app.UpsertOverrideCommand{
			Tier:              overrideTier,
			ProductSysID:      req.GetProductSysId(),
			RequestID:         req.GetRequestId(),
			RouteLevel:        req.GetRouteLevel(),
			ReapproveOnChange: &reapprove,
			SLAFillHours:      &slaFill,
			SLAApproveHours:   &slaApprove,
			Actor:             actor,
		}
		if fillerType != "" {
			cmd.FillerType = &fillerType
		}
		if fillerValue != "" {
			cmd.FillerValue = &fillerValue
		}
		if approverType != "" {
			cmd.ApproverType = &approverType
		}
		if approverValue != "" {
			cmd.ApproverValue = &approverValue
		}
		if err := h.upsertOverride.Handle(ctx, cmd); err != nil {
			return &financev1.UpsertLevelConfigResponse{Base: fillConfigErrToBase(err)}, nil
		}
	default:
		return &financev1.UpsertLevelConfigResponse{
			Base: ErrorResponse("400", "tier must be GLOBAL, PRODUCT, or REQUEST"),
		}, nil
	}

	return &financev1.UpsertLevelConfigResponse{Base: successResponse("Config saved")}, nil
}

// DeleteGlobalConfig deactivates the global config for a level.
func (h *CostFillConfigHandler) DeleteGlobalConfig(ctx context.Context, req *financev1.DeleteGlobalConfigRequest) (*financev1.DeleteGlobalConfigResponse, error) {
	cmd := app.DeleteGlobalConfigCommand{RouteLevel: req.GetRouteLevel()}
	if err := h.deleteGlobal.Handle(ctx, cmd); err != nil {
		return &financev1.DeleteGlobalConfigResponse{Base: fillConfigErrToBase(err)}, nil
	}
	return &financev1.DeleteGlobalConfigResponse{Base: successResponse("Config deleted")}, nil
}

// ListGlobalConfigs returns all active global-tier configs.
func (h *CostFillConfigHandler) ListGlobalConfigs(ctx context.Context, _ *financev1.ListGlobalConfigsRequest) (*financev1.ListGlobalConfigsResponse, error) {
	result, err := h.listGlobal.Handle(ctx, app.ListGlobalConfigQuery{})
	if err != nil {
		return &financev1.ListGlobalConfigsResponse{Base: fillConfigErrToBase(err)}, nil
	}
	data := make([]*financev1.LevelAssignmentConfig, 0, len(result.Configs))
	for _, cfg := range result.Configs {
		data = append(data, configToProto(cfg))
	}
	return &financev1.ListGlobalConfigsResponse{
		Base: successResponse("OK"),
		Data: data,
	}, nil
}

// =============================================================================
// proto <-> domain mappers
// =============================================================================

func configToProto(cfg *domain.Config) *financev1.LevelAssignmentConfig {
	if cfg == nil {
		return nil
	}
	p := &financev1.LevelAssignmentConfig{
		ConfigId:     cfg.ConfigID,
		Tier:         string(cfg.Tier),
		RouteLevel:   cfg.RouteLevel,
		ProductSysId: cfg.ProductSysID,
		RequestId:    cfg.RequestID,
	}
	if cfg.FillerType != nil {
		p.FillerType = *cfg.FillerType
	}
	if cfg.FillerValue != nil {
		p.FillerValue = *cfg.FillerValue
	}
	if cfg.ApproverType != nil {
		p.ApproverType = *cfg.ApproverType
	}
	if cfg.ApproverValue != nil {
		p.ApproverValue = *cfg.ApproverValue
	}
	if cfg.ReapproveOnChange != nil {
		p.ReapproveOnChange = *cfg.ReapproveOnChange
	}
	if cfg.SLAFillHours != nil {
		p.SlaFillHours = *cfg.SLAFillHours
	}
	if cfg.SLAApproveHours != nil {
		p.SlaApproveHours = *cfg.SLAApproveHours
	}
	return p
}

// =============================================================================
// error mapping
// =============================================================================

func fillConfigErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domain.ErrConfigNotFound):
		return ErrorResponse("404", "config not found")
	case errors.Is(err, domain.ErrInvalidActorType), errors.Is(err, domain.ErrEmptyActorValue):
		return ErrorResponse("400", err.Error())
	}
	return ErrorResponse("500", err.Error())
}
