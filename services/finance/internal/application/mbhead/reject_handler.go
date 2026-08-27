// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// RejectCommand represents the SUBMITTED → REJECTED transition command (user decision K-2).
type RejectCommand struct {
	MbhID uuid.UUID
	// Reason is MANDATORY. The domain returns mbhead.ErrReasonRequired when it is
	// empty — an approver who turns work down must say why.
	Reason      string
	ActorUserID string
}

// RejectHandler handles the RejectMBHead command.
type RejectHandler struct {
	repo mbhead.Repository
}

// NewRejectHandler creates a new RejectHandler.
func NewRejectHandler(repo mbhead.Repository) *RejectHandler {
	return &RejectHandler{repo: repo}
}

// Handle executes the reject MB Head transition.
func (h *RejectHandler) Handle(ctx context.Context, cmd RejectCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, err
	}

	fromState := entity.EntryStatus()
	if err := entity.Reject(cmd.Reason); err != nil {
		return nil, err
	}

	if err := h.repo.Transition(ctx, entity.ID(), fromState, entity.EntryStatus(), entity.CurrentVersion(), entity.StateReason(), cmd.ActorUserID, nil); err != nil {
		return nil, err
	}

	return entity, nil
}
