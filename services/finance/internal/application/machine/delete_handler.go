// Package machine provides application layer handlers for Machine operations.
package machine

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
)

// DeleteCommand holds the input for deleting a machine.
type DeleteCommand struct {
	MachineID uuid.UUID
	DeletedBy string
}

// DeleteHandler handles the DeleteMachine command.
type DeleteHandler struct {
	repo machine.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo machine.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete machine command.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.SoftDelete(ctx, cmd.MachineID, cmd.DeletedBy)
}
