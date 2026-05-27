// Package formula provides application layer handlers for Formula operations.
package formula

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/formula"
)

// DeleteCommand represents the delete Formula command.
type DeleteCommand struct {
	FormulaID string
	DeletedBy string
}

// DeleteHandler handles the DeleteFormula command.
type DeleteHandler struct {
	repo formula.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo formula.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete Formula command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	id, err := uuid.Parse(cmd.FormulaID)
	if err != nil {
		return formula.ErrNotFound
	}

	exists, err := h.repo.ExistsByID(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return formula.ErrNotFound
	}

	return h.repo.SoftDelete(ctx, id, cmd.DeletedBy)
}
