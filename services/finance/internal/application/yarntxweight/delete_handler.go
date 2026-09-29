// Package yarntxweight provides application layer handlers for the TX Weight master.
package yarntxweight

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
)

// DeleteCommand represents the delete TX Weight rule command.
type DeleteCommand struct {
	ID        uuid.UUID
	DeletedBy string
}

// DeleteHandler handles DeleteYarnTxWeight.
type DeleteHandler struct {
	repo yarntxweight.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo yarntxweight.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle soft-deletes the rule. The product type then falls back to the ratio
// formula for that grade.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.SoftDelete(ctx, cmd.ID, cmd.DeletedBy)
}
