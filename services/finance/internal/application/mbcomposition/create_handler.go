// Package mbcomposition provides application layer handlers for MB composition operations.
package mbcomposition

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// CreateCommand represents the create MB composition command.
type CreateCommand struct {
	MbhID          string
	GroupHeadID    string
	CompositionPct string
	SourceType     string
	SeqNo          int32
	MbRefMbhID     string
	IsCarrier      bool
	CreatedBy      string
}

// CreateHandler handles the CreateMbComposition command.
type CreateHandler struct {
	repo mbcomposition.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo mbcomposition.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create MB composition command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*mbcomposition.Entity, error) {
	entity, err := mbcomposition.NewEntity(cmd.MbhID, cmd.GroupHeadID, cmd.CompositionPct, cmd.SourceType,
		cmd.SeqNo, cmd.MbRefMbhID, cmd.IsCarrier, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	// [K-33] DRAFT gate: refuse the write outright unless the parent head is still
	// DRAFT. Checked before any write and before the sum work, so a locked recipe
	// costs one cheap status read and leaves nothing behind.
	if err := ensureParentDraft(ctx, h.repo, cmd.MbhID); err != nil {
		return nil, err
	}

	// [G.5] Composition-sum rule (D17): the new row's percentage counts toward the
	// mbh_id total only when it is not a carrier row (carriers are excluded from the
	// sum query). ⭐ G24: the check is no longer performed here — it is handed to the
	// repository, which runs it inside the insert's own transaction under a lock on
	// the parent head row, so a rejected write leaves nothing behind AND a concurrent
	// writer cannot slip between the check and the insert.
	delta, err := pctDelta(entity.CompositionPct(), entity.IsCarrier())
	if err != nil {
		return nil, err
	}

	if err := h.repo.CreateWithSumGuard(ctx, entity, sumGuardFor(delta)); err != nil {
		return nil, err
	}

	return entity, nil
}
