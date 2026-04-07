// Package parameter provides application layer handlers for Parameter operations.
package parameter

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// DeleteCommand represents the delete Parameter command.
type DeleteCommand struct {
	ParamID   string
	DeletedBy string
}

// DeleteHandler handles the DeleteParameter command.
type DeleteHandler struct {
	repo parameter.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo parameter.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete Parameter command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	// 1. Parse ID
	id, err := uuid.Parse(cmd.ParamID)
	if err != nil {
		return parameter.ErrNotFound
	}

	// 2. Check existence
	exists, err := h.repo.ExistsByID(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return parameter.ErrNotFound
	}

	// 3. Soft delete
	return h.repo.SoftDelete(ctx, id, cmd.DeletedBy)
}
