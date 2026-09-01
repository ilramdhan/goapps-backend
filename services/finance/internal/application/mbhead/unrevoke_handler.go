// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// UnrevokeCommand represents the REVOKED → DRAFT transition command
// (user decision 2026-08-31, mirroring K-29's ReturnToDraft).
type UnrevokeCommand struct {
	MbhID uuid.UUID
	// 🔴 Reason is OPTIONAL here — same rationale as ReturnToDraftCommand. Sending work
	// back to its own author is not an accusation, so nothing has to be justified.
	// ⛔ Do NOT "tidy" this into a required field: an empty Reason is a valid, expected
	// input.
	Reason      string
	ActorUserID string
}

// UnrevokeHandler handles the UnrevokeMBHead command. Gated at the delivery layer by
// the dedicated finance.mb.head.unrevoke permission, granted only to Super Admin.
type UnrevokeHandler struct {
	repo mbhead.Repository
	// notifier is OPTIONAL: nil disables notifications.
	notifier Notifier
}

// WithNotifier attaches the MB recipe notifier and returns the handler for chaining.
func (h *UnrevokeHandler) WithNotifier(n Notifier) *UnrevokeHandler {
	h.notifier = n
	return h
}

// NewUnrevokeHandler creates a new UnrevokeHandler.
func NewUnrevokeHandler(repo mbhead.Repository) *UnrevokeHandler {
	return &UnrevokeHandler{repo: repo}
}

// Handle executes the unrevoke MB Head transition.
//
// 🔴 Do not "fix": cmd.Reason MAY be empty. When it is, entity.Unrevoke leaves the
// previously stored stateReason untouched, so the entity.StateReason() value forwarded
// to repo.Transition below is very often the OLD REVOKE reason, intentionally carried
// forward rather than blanked (global principle U-2 — don't erase the trail).
func (h *UnrevokeHandler) Handle(ctx context.Context, cmd UnrevokeCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, err
	}

	fromState := entity.EntryStatus()
	if err := entity.Unrevoke(cmd.Reason); err != nil {
		return nil, err
	}

	if err := h.repo.Transition(ctx, entity.ID(), fromState, entity.EntryStatus(), entity.CurrentVersion(), entity.StateReason(), cmd.ActorUserID, nil); err != nil {
		return nil, err
	}

	// Best-effort notification to whoever can re-submit, so the author learns the
	// recipe is back in their court. Fired AFTER the commit.
	emitEvent(ctx, h.notifier, Event{
		EventType:   EventUnrevoked,
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
