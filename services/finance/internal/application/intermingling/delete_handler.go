// Package intermingling provides application layer handlers for Intermingling operations.
package intermingling

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
)

// DeleteCommand represents the delete Intermingling command.
type DeleteCommand struct {
	ID        uuid.UUID
	DeletedBy string
}

// DeleteHandler handles the DeleteIntermingling command.
type DeleteHandler struct {
	repo intermingling.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo intermingling.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete Intermingling command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.SoftDelete(ctx, cmd.ID, cmd.DeletedBy)
}
