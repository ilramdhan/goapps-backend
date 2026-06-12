package costfillassignment

import (
	"context"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// UpsertGlobalConfigCommand carries the data for creating/replacing a global level config.
type UpsertGlobalConfigCommand struct {
	RouteLevel        int32
	FillerType        string
	FillerValue       string
	ApproverType      *string
	ApproverValue     *string
	ReapproveOnChange *bool
	SLAFillHours      *int32
	SLAApproveHours   *int32
	Actor             string
}

// UpsertGlobalConfigHandler upserts the active global config for a route level.
type UpsertGlobalConfigHandler struct {
	repo domain.ConfigRepository
}

// NewUpsertGlobalConfigHandler constructs the handler.
func NewUpsertGlobalConfigHandler(repo domain.ConfigRepository) *UpsertGlobalConfigHandler {
	return &UpsertGlobalConfigHandler{repo: repo}
}

// Handle validates the command and delegates to the config repository.
func (h *UpsertGlobalConfigHandler) Handle(ctx context.Context, cmd UpsertGlobalConfigCommand) error {
	if cmd.RouteLevel < 1 {
		return fmt.Errorf("route level must be >= 1")
	}
	if cmd.FillerType == "" || cmd.FillerValue == "" {
		return fmt.Errorf("filler type and value are required")
	}
	if cmd.Actor == "" {
		return fmt.Errorf("actor is required")
	}

	cfg := &domain.Config{
		Tier:              domain.TierGlobal,
		RouteLevel:        cmd.RouteLevel,
		FillerType:        &cmd.FillerType,
		FillerValue:       &cmd.FillerValue,
		ApproverType:      cmd.ApproverType,
		ApproverValue:     cmd.ApproverValue,
		ReapproveOnChange: cmd.ReapproveOnChange,
		SLAFillHours:      cmd.SLAFillHours,
		SLAApproveHours:   cmd.SLAApproveHours,
	}
	if err := h.repo.UpsertGlobal(ctx, cfg, cmd.Actor); err != nil {
		return fmt.Errorf("upsert global config level %d: %w", cmd.RouteLevel, err)
	}
	return nil
}
