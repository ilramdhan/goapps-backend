// Package spinfixedcost provides application layer handlers for Spin Fixed Cost operations.
package spinfixedcost

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/spinfixedcost"
)

// DeleteCommand represents the delete Spin Fixed Cost command.
type DeleteCommand struct {
	ID        uuid.UUID
	DeletedBy string
}

// DeleteHandler handles the DeleteSpinFixedCost command.
type DeleteHandler struct {
	repo spinfixedcost.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo spinfixedcost.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete Spin Fixed Cost command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return err
	}

	// Soft-deleting the anchor row silently zeroes POY fixed cost for every POY
	// product rather than raising anywhere; refuse instead. See CheckAnchorGuard.
	stats, err := h.repo.LoadAnchorStats(ctx, entity.ID())
	if err != nil {
		return err
	}
	if err := spinfixedcost.CheckAnchorGuard(entity, stats); err != nil {
		return err
	}

	return h.repo.SoftDelete(ctx, cmd.ID, cmd.DeletedBy)
}
