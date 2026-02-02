// Package uom provides application layer handlers for UOM operations.
package uom

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

// DeleteCommand represents the delete UOM command.
type DeleteCommand struct {
	UOMID     string
	DeletedBy string
}

// DeleteHandler handles the DeleteUOM command.
type DeleteHandler struct {
	repo uom.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo uom.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete UOM command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	// 1. Parse ID
	id, err := uuid.Parse(cmd.UOMID)
	if err != nil {
		return uom.ErrNotFound
	}

	// 2. Check existence
	exists, err := h.repo.ExistsByID(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return uom.ErrNotFound
	}

	// 3. Soft delete
	return h.repo.SoftDelete(ctx, id, cmd.DeletedBy)
}
