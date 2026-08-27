// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// ReturnToDraftCommand represents the REJECTED → DRAFT transition command
// (user decision K-29, 2026-08-23).
type ReturnToDraftCommand struct {
	MbhID uuid.UUID
	// 🔴 Reason is OPTIONAL here — deliberately unlike RejectCommand, UnApproveCommand
	// and RevokeCommand, which all mandate one. Sending work back to its own author is
	// not an accusation, so nothing has to be justified. ⛔ Do NOT "tidy" this into a
	// required field: an empty Reason is a valid, expected input.
	Reason      string
	ActorUserID string
}

// ReturnToDraftHandler handles the ReturnToDraftMBHead command.
type ReturnToDraftHandler struct {
	repo mbhead.Repository
	// notifier is OPTIONAL: nil disables notifications.
	notifier Notifier
}

// WithNotifier attaches the MB recipe notifier and returns the handler for chaining.
func (h *ReturnToDraftHandler) WithNotifier(n Notifier) *ReturnToDraftHandler {
	h.notifier = n
	return h
}

// NewReturnToDraftHandler creates a new ReturnToDraftHandler.
func NewReturnToDraftHandler(repo mbhead.Repository) *ReturnToDraftHandler {
	return &ReturnToDraftHandler{repo: repo}
}

// Handle executes the return-to-draft MB Head transition.
//
// 🔴 K-29 contract, do not "fix": cmd.Reason MAY be empty. When it is,
// entity.ReturnToDraft leaves the previously stored stateReason untouched, so the
// entity.StateReason() value forwarded to repo.Transition below is very often the OLD
// REJECT reason, intentionally carried forward rather than blanked. The author must
// still be able to read WHY the MB was rejected while reworking it (global principle
// U-2 — don't erase the trail). Passing "" to Transition instead, or making Reason
// mandatory, would break that.
func (h *ReturnToDraftHandler) Handle(ctx context.Context, cmd ReturnToDraftCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, err
	}

	fromState := entity.EntryStatus()
	if err := entity.ReturnToDraft(cmd.Reason); err != nil {
		return nil, err
	}

	if err := h.repo.Transition(ctx, entity.ID(), fromState, entity.EntryStatus(), entity.CurrentVersion(), entity.StateReason(), cmd.ActorUserID, nil); err != nil {
		return nil, err
	}

	// Best-effort notification to whoever can re-submit, so the author learns the
	// recipe is back in their court. Fired AFTER the commit.
	//
	// 🔴 fromState is carried into the event so this notification is attributable to
	// the REJECTED → DRAFT road specifically. The other two roads into DRAFT
	// (grant-unlock, create-new) deliberately do NOT emit it — see EventReturnedToDraft.
	emitEvent(ctx, h.notifier, Event{
		EventType:   EventReturnedToDraft,
		MbhID:       entity.ID(),
		MBCosting:   entity.MBCosting(),
		FromState:   fromState,
		ToState:     entity.EntryStatus(),
		Version:     entity.CurrentVersion(),
		ActorUserID: cmd.ActorUserID,
		Rules:       []NotifRule{{RuleType: RuleByPermission, Value: PermMBHeadSubmit}},
	})

	return entity, nil
}
