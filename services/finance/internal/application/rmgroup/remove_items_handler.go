// Package rmgroup provides application layer handlers for RM group head and detail operations.
package rmgroup

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// RemoveMode controls how items are removed from a group.
type RemoveMode string

// Remove modes.
const (
	// RemoveModeDeactivate sets is_active = false, keeping the detail row for audit.
	RemoveModeDeactivate RemoveMode = "deactivate"
	// RemoveModeSoftDelete sets deleted_at/deleted_by, hiding the row from most reads.
	RemoveModeSoftDelete RemoveMode = "soft_delete"
)

// RemoveItemsCommand removes a set of detail rows from a head. Detail IDs must
// belong to the same head — a mismatch fails the whole call before any row is
// mutated, so partial effects are impossible.
type RemoveItemsCommand struct {
	HeadID    string
	DetailIDs []string
	Mode      RemoveMode
	RemovedBy string
}

// RemoveItemsResult reports the detail rows that were removed.
type RemoveItemsResult struct {
	HeadID   uuid.UUID
	Removed  []uuid.UUID
	NotFound []uuid.UUID
}

// RemoveItemsHandler handles RemoveItems commands.
type RemoveItemsHandler struct {
	repo rmgroup.Repository
}

// NewRemoveItemsHandler builds a RemoveItemsHandler.
func NewRemoveItemsHandler(repo rmgroup.Repository) *RemoveItemsHandler {
	return &RemoveItemsHandler{repo: repo}
}

// Handle validates input, loads each detail, verifies ownership, then removes
// per the configured mode.
func (h *RemoveItemsHandler) Handle(ctx context.Context, cmd RemoveItemsCommand) (*RemoveItemsResult, error) {
	if cmd.RemovedBy == "" {
		return nil, rmgroup.ErrEmptyUpdatedBy
	}
	mode := cmd.Mode
	if mode == "" {
		mode = RemoveModeSoftDelete
	}
	if mode != RemoveModeDeactivate && mode != RemoveModeSoftDelete {
		return nil, fmt.Errorf("invalid remove mode %q", cmd.Mode)
	}

	headID, err := uuid.Parse(cmd.HeadID)
	if err != nil {
		return nil, rmgroup.ErrNotFound
	}
	if _, err := h.repo.GetHeadByID(ctx, headID); err != nil {
		return nil, err
	}

	detailIDs, err := parseDetailIDs(cmd.DetailIDs)
	if err != nil {
		return nil, err
	}

	result := &RemoveItemsResult{HeadID: headID}
	for _, id := range detailIDs {
		if err := h.removeOne(ctx, headID, id, mode, cmd.RemovedBy, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func parseDetailIDs(raw []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("invalid detail id %q: %w", s, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (h *RemoveItemsHandler) removeOne(
	ctx context.Context,
	headID, detailID uuid.UUID,
	mode RemoveMode,
	removedBy string,
	result *RemoveItemsResult,
) error {
	detail, err := h.repo.GetDetailByID(ctx, detailID)
	if err != nil {
		if errors.Is(err, rmgroup.ErrDetailNotFound) {
			result.NotFound = append(result.NotFound, detailID)
			return nil
		}
		return fmt.Errorf("get detail %s: %w", detailID, err)
	}
	if detail.HeadID() != headID {
		return fmt.Errorf("detail %s does not belong to head %s", detailID, headID)
	}

	switch mode {
	case RemoveModeDeactivate:
		if err := detail.Deactivate(removedBy); err != nil {
			return err
		}
		if err := h.repo.UpdateDetail(ctx, detail); err != nil {
			return fmt.Errorf("deactivate detail %s: %w", detailID, err)
		}
	case RemoveModeSoftDelete:
		if err := h.repo.SoftDeleteDetail(ctx, detailID, removedBy); err != nil {
			return fmt.Errorf("soft delete detail %s: %w", detailID, err)
		}
	}
	result.Removed = append(result.Removed, detailID)
	return nil
}
