// Package boxbobbincost provides application layer handlers for Box Bobbin Cost operations.
package boxbobbincost

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
)

// DeleteRateCommand represents the command to remove a rate entry.
type DeleteRateCommand struct {
	RateID    uuid.UUID
	DeletedBy string
}

// DeleteRateHandler handles the DeleteBoxBobbinCostRate command.
type DeleteRateHandler struct {
	repo boxbobbincost.Repository
}

// NewDeleteRateHandler creates a new DeleteRateHandler.
func NewDeleteRateHandler(repo boxbobbincost.Repository) *DeleteRateHandler {
	return &DeleteRateHandler{repo: repo}
}

// Handle executes the delete rate command (soft delete).
func (h *DeleteRateHandler) Handle(ctx context.Context, cmd DeleteRateCommand) error {
	return h.repo.DeleteRate(ctx, cmd.RateID, cmd.DeletedBy)
}
