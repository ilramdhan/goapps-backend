package costfillassignment

import (
	"context"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// OverrideTier identifies which config tier an override targets.
type OverrideTier string

const (
	// OverrideTierProduct targets the product-level override table.
	OverrideTierProduct OverrideTier = "PRODUCT"
	// OverrideTierRequest targets the request-level override table.
	OverrideTierRequest OverrideTier = "REQUEST"
)

// UpsertOverrideCommand carries data for a product or request level override.
type UpsertOverrideCommand struct {
	Tier              OverrideTier
	ProductSysID      int64 // used when Tier == PRODUCT
	RequestID         int64 // used when Tier == REQUEST
	RouteLevel        int32
	FillerType        *string
	FillerValue       *string
	ApproverType      *string
	ApproverValue     *string
	ReapproveOnChange *bool
	SLAFillHours      *int32
	SLAApproveHours   *int32
	Actor             string
}

// UpsertOverrideHandler inserts or updates a product or request level config override.
type UpsertOverrideHandler struct {
	repo domain.ConfigRepository
}

// NewUpsertOverrideHandler constructs the handler.
func NewUpsertOverrideHandler(repo domain.ConfigRepository) *UpsertOverrideHandler {
	return &UpsertOverrideHandler{repo: repo}
}

// Handle validates and upserts the override.
func (h *UpsertOverrideHandler) Handle(ctx context.Context, cmd UpsertOverrideCommand) error {
	if cmd.RouteLevel < 1 {
		return fmt.Errorf("route level must be >= 1")
	}
	if cmd.Actor == "" {
		return fmt.Errorf("actor is required")
	}
	cfg := &domain.Config{
		RouteLevel:        cmd.RouteLevel,
		FillerType:        cmd.FillerType,
		FillerValue:       cmd.FillerValue,
		ApproverType:      cmd.ApproverType,
		ApproverValue:     cmd.ApproverValue,
		ReapproveOnChange: cmd.ReapproveOnChange,
		SLAFillHours:      cmd.SLAFillHours,
		SLAApproveHours:   cmd.SLAApproveHours,
	}
	switch cmd.Tier {
	case OverrideTierProduct:
		if cmd.ProductSysID <= 0 {
			return fmt.Errorf("product sys ID required for PRODUCT tier override")
		}
		cfg.Tier = domain.TierProduct
		cfg.ProductSysID = cmd.ProductSysID
		return h.repo.UpsertProduct(ctx, cfg, cmd.Actor)
	case OverrideTierRequest:
		if cmd.RequestID <= 0 {
			return fmt.Errorf("request ID required for REQUEST tier override")
		}
		cfg.Tier = domain.TierRequest
		cfg.RequestID = cmd.RequestID
		return h.repo.UpsertRequest(ctx, cfg, cmd.Actor)
	default:
		return fmt.Errorf("unknown override tier: %s", cmd.Tier)
	}
}
