// Package mbspin provides application layer handlers for MB Spin operations.
package mbspin

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// DeleteCommand represents the delete MB Spin command.
type DeleteCommand struct {
	ID        uuid.UUID
	DeletedBy string
}

// DeleteHandler handles the DeleteMBSpin command.
type DeleteHandler struct {
	repo mbspin.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo mbspin.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete MB Spin command (soft delete).
//
// Guard order: existence, then children (lineage integrity), then cost
// product usage. Children are checked first because a spin with children is
// almost always ALSO the more actionable problem to surface first (the user
// must resolve duplicated lineage before anything else); cost product usage
// is the narrower, secondary condition.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	exists, err := h.repo.ExistsByID(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if !exists {
		return mbspin.ErrNotFound
	}

	hasChildren, err := h.repo.HasChildren(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if hasChildren {
		return mbspin.ErrHasChildren
	}

	inUse, err := h.repo.IsUsedByCostProduct(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if inUse {
		return mbspin.ErrInUse
	}

	return h.repo.SoftDelete(ctx, cmd.ID, cmd.DeletedBy)
}
