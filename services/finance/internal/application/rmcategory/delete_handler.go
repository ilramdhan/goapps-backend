// Package rmcategory provides application layer handlers for RMCategory operations.
package rmcategory

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcategory"
)

// DeleteCommand represents the delete RMCategory command.
type DeleteCommand struct {
	RMCategoryID string
	DeletedBy    string
}

// DeleteHandler handles the DeleteRMCategory command.
type DeleteHandler struct {
	repo rmcategory.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo rmcategory.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete RMCategory command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	// 1. Parse ID
	id, err := uuid.Parse(cmd.RMCategoryID)
	if err != nil {
		return rmcategory.ErrNotFound
	}

	// 2. Check existence
	exists, err := h.repo.ExistsByID(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return rmcategory.ErrNotFound
	}

	// 3. Soft delete
	return h.repo.SoftDelete(ctx, id, cmd.DeletedBy)
}
