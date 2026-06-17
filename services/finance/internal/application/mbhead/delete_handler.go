// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// DeleteCommand represents the delete MB Head command.
type DeleteCommand struct {
	ID        uuid.UUID
	DeletedBy string
}

// DeleteHandler handles the DeleteMBHead command.
type DeleteHandler struct {
	repo mbhead.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo mbhead.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete MB Head command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.SoftDelete(ctx, cmd.ID, cmd.DeletedBy)
}
