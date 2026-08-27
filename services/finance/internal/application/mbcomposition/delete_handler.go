package mbcomposition

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// DeleteCommand represents the delete MB composition command.
type DeleteCommand struct {
	ID string
}

// DeleteHandler handles the DeleteMbComposition command.
type DeleteHandler struct {
	repo mbcomposition.Repository
}

// NewDeleteHandler creates a new DeleteHandler.
func NewDeleteHandler(repo mbcomposition.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the delete MB composition command (soft delete).
//
// The row is read first purely to learn its parent mbh_id: DeleteCommand carries
// only the composition id, and the [K-33] DRAFT gate needs the parent. A missing
// row still reports mbcomposition.ErrNotFound exactly as before — GetByID returns
// that sentinel and it is propagated unwrapped.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	existing, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return err
	}

	if err := ensureParentDraft(ctx, h.repo, existing.MbhID()); err != nil {
		return err
	}

	return h.repo.Delete(ctx, cmd.ID)
}
