// Package spinfixedcost provides application layer handlers for Spin Fixed Cost operations.
package spinfixedcost

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/spinfixedcost"
)

// UpdateCommand represents the update Spin Fixed Cost command.
// Period is absent by design: it is immutable after creation.
type UpdateCommand struct {
	ID                 uuid.UUID
	CommonPoyDenier    *float64
	PoyProduction      *float64
	SpinPowerMonth     *float64
	SpinManpowerMonth  *float64
	SpinOverheadsMonth *float64
	SpinConssprsMonth  *float64
	IsActive           *bool
	UpdatedBy          string
}

// UpdateHandler handles the UpdateSpinFixedCost command.
type UpdateHandler struct {
	repo spinfixedcost.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo spinfixedcost.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update Spin Fixed Cost command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*spinfixedcost.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	in := spinfixedcost.UpdateInput{
		CommonPoyDenier:    cmd.CommonPoyDenier,
		PoyProduction:      cmd.PoyProduction,
		SpinPowerMonth:     cmd.SpinPowerMonth,
		SpinManpowerMonth:  cmd.SpinManpowerMonth,
		SpinOverheadsMonth: cmd.SpinOverheadsMonth,
		SpinConssprsMonth:  cmd.SpinConssprsMonth,
		IsActive:           cmd.IsActive,
	}

	// Deactivating a row removes it from the calc engine's period resolution just as
	// surely as deleting it, so it runs the same anchor-row guard.
	if in.DeactivatesRow(entity) {
		stats, statsErr := h.repo.LoadAnchorStats(ctx, entity.ID())
		if statsErr != nil {
			return nil, statsErr
		}
		if guardErr := spinfixedcost.CheckAnchorGuard(entity, stats); guardErr != nil {
			return nil, guardErr
		}
	}

	if err := entity.Update(in, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
