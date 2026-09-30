// Package yarntxweightgroup provides application layer handlers for TX Weight groups.
package yarntxweightgroup

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweightgroup"
)

// DeleteCommand represents the delete TX Weight group command.
type DeleteCommand struct {
	ID        uuid.UUID
	DeletedBy string
}

// DeleteHandler handles DeleteYarnTxWeightGroup.
type DeleteHandler struct {
	repo yarntxweightgroup.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo yarntxweightgroup.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle soft-deletes the group and frees its product types; their products
// then fall back to the ratio formula until mapped to another group.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	return h.repo.SoftDelete(ctx, cmd.ID, cmd.DeletedBy)
}
