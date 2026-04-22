// Package rmgroup provides application layer handlers for RM group head and detail operations.
package rmgroup

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// DeleteCommand is the soft-delete command for a head. The repository cascades the
// delete to every active detail belonging to the head.
type DeleteCommand struct {
	HeadID    string
	DeletedBy string
}

// CostChecker reports whether a group head has produced any cost rows. Kept as
// a narrow interface so the rmgroup application package does not import the
// rmcost domain directly.
type CostChecker interface {
	ExistsForGroupHead(ctx context.Context, groupHeadID uuid.UUID) (bool, error)
}

// DeleteHandler handles DeleteHead commands.
type DeleteHandler struct {
	repo        rmgroup.Repository
	costChecker CostChecker
}

// NewDeleteHandler builds a DeleteHandler. costChecker may be nil; when nil the
// delete-guard is skipped (used by tests that do not exercise cost state).
func NewDeleteHandler(repo rmgroup.Repository, costChecker CostChecker) *DeleteHandler {
	return &DeleteHandler{repo: repo, costChecker: costChecker}
}

// Handle soft-deletes the head and its active details.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	if cmd.DeletedBy == "" {
		return rmgroup.ErrEmptyUpdatedBy
	}
	id, err := uuid.Parse(cmd.HeadID)
	if err != nil {
		return rmgroup.ErrNotFound
	}

	exists, err := h.repo.ExistsHeadByID(ctx, id)
	if err != nil {
		return fmt.Errorf("check head existence: %w", err)
	}
	if !exists {
		return rmgroup.ErrNotFound
	}

	if h.costChecker != nil {
		hasCost, err := h.costChecker.ExistsForGroupHead(ctx, id)
		if err != nil {
			return fmt.Errorf("check cost data for group head: %w", err)
		}
		if hasCost {
			return rmgroup.ErrGroupHasCostData
		}
	}

	if err := h.repo.SoftDeleteHead(ctx, id, cmd.DeletedBy); err != nil {
		return fmt.Errorf("soft delete head: %w", err)
	}
	return nil
}
