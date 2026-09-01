// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// ForceUnvalidateCommand represents the VALIDATED → DRAFT force-unvalidate command
// used by the Bulk MB Head Regenerate feature (Phase C). Unlike the ordinary
// workflow transitions, this bypasses the normal state-machine gate (no
// Submit/Approve required first) — it exists specifically so a Super Admin can
// re-trigger the Unvalidate → Submit → Validate lifecycle in bulk.
type ForceUnvalidateCommand struct {
	MbhID       uuid.UUID
	Reason      string
	ActorUserID string
}

// ForceUnvalidateHandler handles the BulkForceUnvalidateMBHead worker action. It is
// a thin wrapper over mbhead.Entity.ForceUnvalidate + Repository.ForceUnvalidateTransition,
// following the same GetByID → mutate → persist shape as SubmitHandler/ValidateHandler.
type ForceUnvalidateHandler struct {
	repo mbhead.Repository
}

// NewForceUnvalidateHandler creates a new ForceUnvalidateHandler.
func NewForceUnvalidateHandler(repo mbhead.Repository) *ForceUnvalidateHandler {
	return &ForceUnvalidateHandler{repo: repo}
}

// Handle executes the force-unvalidate transition.
func (h *ForceUnvalidateHandler) Handle(ctx context.Context, cmd ForceUnvalidateCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, err
	}

	if err := entity.ForceUnvalidate(cmd.Reason); err != nil {
		return nil, err
	}

	if err := h.repo.ForceUnvalidateTransition(ctx, entity.ID(), int(entity.CurrentVersion()), entity.StateReason(), cmd.ActorUserID); err != nil {
		return nil, err
	}

	return entity, nil
}
