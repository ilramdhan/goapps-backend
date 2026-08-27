// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	appmbcomposition "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomposition"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// SubmitCommand represents the DRAFT → SUBMITTED transition command.
type SubmitCommand struct {
	MbhID       uuid.UUID
	ActorUserID string
}

// SubmitHandler handles the SubmitMBHead command.
type SubmitHandler struct {
	repo mbhead.Repository
	// compositionRepo backs the [G.5] composition-sum gate. It is OPTIONAL: nil
	// means the gate is skipped entirely, which keeps every existing construction
	// site (and the backfill tool) working unchanged.
	compositionRepo mbcomposition.Repository
	// notifier is OPTIONAL: nil disables notifications entirely, which keeps every
	// existing construction site compiling unchanged.
	notifier Notifier
}

// WithNotifier attaches the MB recipe notifier and returns the handler for chaining.
// A nil notifier leaves notifications off.
func (h *SubmitHandler) WithNotifier(n Notifier) *SubmitHandler {
	h.notifier = n
	return h
}

// NewSubmitHandler creates a new SubmitHandler without the [G.5] composition-sum
// gate. Retained so existing callers compile unchanged; prefer
// NewSubmitHandlerWithComposition on the serving path.
func NewSubmitHandler(repo mbhead.Repository) *SubmitHandler {
	return &SubmitHandler{repo: repo}
}

// NewSubmitHandlerWithComposition creates a SubmitHandler that enforces the
// composition-sum rule before allowing DRAFT → SUBMITTED (plan §11 item 78).
//
// Without this gate the rule is enforced only on the composition CRUD path, so a
// head whose composition was never edited through those handlers could be submitted
// with a total that does not add up. The gate still respects
// MB_COMPOSITION_SUM_ENFORCED, so it is inert until the flag is turned on.
func NewSubmitHandlerWithComposition(repo mbhead.Repository, compositionRepo mbcomposition.Repository) *SubmitHandler {
	return &SubmitHandler{repo: repo, compositionRepo: compositionRepo}
}

// Handle executes the submit MB Head transition.
func (h *SubmitHandler) Handle(ctx context.Context, cmd SubmitCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, err
	}

	fromState := entity.EntryStatus()
	if err := entity.Submit(); err != nil {
		return nil, err
	}

	// [G.5] Composition-sum gate: checked AFTER the state-machine check so a head in
	// the wrong state still reports ErrInvalidTransition rather than a confusing sum
	// error, but BEFORE the transition is persisted — entity.Submit() has only mutated
	// the in-memory copy at this point, so returning here writes nothing. No-op when
	// compositionRepo is nil or the feature flag is off.
	if h.compositionRepo != nil {
		if err := appmbcomposition.EnforceHeadSum(ctx, h.compositionRepo, cmd.MbhID.String()); err != nil {
			return nil, err
		}
	}

	if err := h.repo.Transition(ctx, entity.ID(), fromState, entity.EntryStatus(), entity.CurrentVersion(), "", cmd.ActorUserID, nil); err != nil {
		return nil, err
	}

	// Best-effort notification to whoever can approve. Fired AFTER the transition is
	// committed — a notification failure must never undo a successful submit.
	emitEvent(ctx, h.notifier, Event{
		EventType:   EventSubmitted,
		MbhID:       entity.ID(),
		MBCosting:   entity.MBCosting(),
		FromState:   fromState,
		ToState:     entity.EntryStatus(),
		Version:     entity.CurrentVersion(),
		ActorUserID: cmd.ActorUserID,
		Rules:       []NotifRule{{RuleType: RuleByPermission, Value: PermMBHeadApprove}},
	})

	return entity, nil
}
