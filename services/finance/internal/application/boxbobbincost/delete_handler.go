// Package boxbobbincost provides application layer handlers for Box Bobbin Cost operations.
package boxbobbincost

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
)

// DeleteCommand is the input for deleting a Box Bobbin Cost.
type DeleteCommand struct {
	ID        uuid.UUID
	DeletedBy string
}

// DeleteHandler handles the DeleteBoxBobbinCost command.
type DeleteHandler struct {
	repo boxbobbincost.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo boxbobbincost.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the soft-delete command.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.Delete(ctx, cmd.ID, cmd.DeletedBy)
}
