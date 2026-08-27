package mbcomposition

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// UpdateCommand represents the update MB composition command.
type UpdateCommand struct {
	ID             string
	GroupHeadID    string
	CompositionPct string
	SourceType     string
	MbRefMbhID     string
	IsCarrier      bool
	UpdatedBy      string
}

// UpdateHandler handles the UpdateMbComposition command.
type UpdateHandler struct {
	repo mbcomposition.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo mbcomposition.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update MB composition command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*mbcomposition.Entity, error) {
	existing, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	// [K-33] DRAFT gate: the parent id comes from the stored row, never from the
	// request, so a caller cannot aim the check at some other, still-DRAFT head.
	if err := ensureParentDraft(ctx, h.repo, existing.MbhID()); err != nil {
		return nil, err
	}

	entity := mbcomposition.Reconstruct(
		existing.ID(), existing.MbhID(), existing.SeqNo(), cmd.GroupHeadID, cmd.CompositionPct,
		cmd.SourceType, cmd.MbRefMbhID, cmd.IsCarrier, existing.LegacySysID(),
		existing.CreatedAt(), existing.CreatedBy(), existing.UpdatedAt(), cmd.UpdatedBy,
		existing.DeletedAt(), existing.DeletedBy(),
	)

	// [G.5] Composition-sum rule (D17): the stored total already contains the
	// existing row, so the pending change is (new contribution - old contribution),
	// each counted only when the row is not a carrier. ⭐ G24: the guard runs inside
	// the update's own transaction, under a lock on the parent head row.
	newContrib, err := pctDelta(entity.CompositionPct(), entity.IsCarrier())
	if err != nil {
		return nil, err
	}
	oldContrib, err := pctDelta(existing.CompositionPct(), existing.IsCarrier())
	if err != nil {
		return nil, err
	}

	if err := h.repo.UpdateWithSumGuard(ctx, entity, sumGuardFor(newContrib-oldContrib)); err != nil {
		return nil, err
	}

	return entity, nil
}
