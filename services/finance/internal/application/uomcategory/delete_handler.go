// Package uomcategory provides application layer handlers for UOM Category operations.
package uomcategory

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uomcategory"
)

// DeleteCommand represents the delete UOM Category command.
type DeleteCommand struct {
	UOMCategoryID string
	DeletedBy     string
}

// DeleteHandler handles the DeleteUOMCategory command.
type DeleteHandler struct {
	repo uomcategory.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo uomcategory.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete UOM Category command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	// 1. Parse ID
	id, err := uuid.Parse(cmd.UOMCategoryID)
	if err != nil {
		return uomcategory.ErrNotFound
	}

	// 2. Check existence
	exists, err := h.repo.ExistsByID(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return uomcategory.ErrNotFound
	}

	// 3. Check if in use by any UOM
	inUse, err := h.repo.IsInUse(ctx, id)
	if err != nil {
		return err
	}
	if inUse {
		return uomcategory.ErrInUse
	}

	// 4. Soft delete
	return h.repo.SoftDelete(ctx, id, cmd.DeletedBy)
}
