// Package productgrade provides application layer handlers for Product Grade operations.
package productgrade

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
)

// DeleteCommand represents the delete Product Grade command.
type DeleteCommand struct {
	ProductGradeID uuid.UUID
	DeletedBy      string
}

// DeleteHandler handles the DeleteProductGrade command.
type DeleteHandler struct {
	repo productgrade.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo productgrade.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete Product Grade command (soft delete).
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.SoftDelete(ctx, cmd.ProductGradeID, cmd.DeletedBy)
}
