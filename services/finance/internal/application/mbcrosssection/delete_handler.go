package mbcrosssection

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
)

// DeleteCommand represents the delete MB cross-section command.
type DeleteCommand struct {
	ID        string
	DeletedBy string
}

// DeleteHandler handles the DeleteMbCrossSection command.
type DeleteHandler struct {
	repo mbcrosssection.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo mbcrosssection.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete MB cross-section command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.Delete(ctx, cmd.ID, cmd.DeletedBy)
}
