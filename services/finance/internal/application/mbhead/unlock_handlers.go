// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// UnlockActorStore records and recovers the UUID identity of whoever asked for an
// unlock, so that the eventual grant/reject can notify THAT person (E4/E5).
//
// 🔴 WHY a separate port instead of a new Transition argument. The username already
// carried through the whole workflow (Transition's actorUserID, mbhl_actor_user_id,
// mbh_unlock_requested_by) is a VARCHAR(20) display identity — ⛔ it is NOT a UUID and
// IAM's BY_USER_ID resolver does uuid.Parse on the value it is given, failing the WHOLE
// fan-out when it cannot parse. The requester's real UUID therefore travels on its own,
// into mbhl_meta (JSONB, already present since migration 000485 and until now unused by
// any Go code), and ⛔ never over the audit columns, whose types stay untouched.
//
// Both methods are BEST-EFFORT from the caller's point of view: a failure records or
// recovers nothing and costs at most one notification — it must never fail an unlock.
type UnlockActorStore interface {
	// RecordUnlockRequestActor stamps actorUUID onto mbhl_meta of the most recent
	// UNLOCK_REQUEST lock-log row of the head.
	RecordUnlockRequestActor(ctx context.Context, mbhID uuid.UUID, actorUUID string) error
	// LatestUnlockRequestActor returns the UUID stamped by RecordUnlockRequestActor on
	// the most recent UNLOCK_REQUEST row, or "" when there is none — which is the normal
	// answer for every row written before this feature existed, and for requests raised
	// by a system/service caller that has no UUID at all.
	LatestUnlockRequestActor(ctx context.Context, mbhID uuid.UUID) (string, error)
}

// requesterRule builds the BY_USER_ID recipient rule for the original requester, or
// reports ok = false when there is nobody addressable.
//
// 🔴 ok = false is the LOAD-BEARING case, not an edge case. IAM's resolver parses the
// rule value as a UUID and abandons the entire notification when it cannot
// (iam request_handler.go BY_USER_ID branch), so a blank or malformed value must never
// be sent — ⛔ not even as an "empty" rule. The caller emits NOTHING and logs instead.
func requesterRule(ctx context.Context, store UnlockActorStore, mbhID uuid.UUID) ([]NotifRule, bool) {
	if store == nil {
		return nil, false
	}
	raw, err := store.LatestUnlockRequestActor(ctx, mbhID)
	if err != nil {
		log.Warn().Err(err).Str("mbh_id", mbhID.String()).
			Msg("mbhead: cannot recover unlock requester; notification skipped")
		return nil, false
	}
	if raw == "" {
		log.Warn().Str("mbh_id", mbhID.String()).
			Msg("mbhead: unlock request carries no requester UUID (legacy or system-raised); notification skipped")
		return nil, false
	}
	if _, parseErr := uuid.Parse(raw); parseErr != nil {
		log.Warn().Str("mbh_id", mbhID.String()).Str("raw", raw).
			Msg("mbhead: unlock requester identity is not a UUID; notification skipped")
		return nil, false
	}
	return []NotifRule{{RuleType: RuleByUserID, Value: raw}}, true
}

// RequestUnlockCommand asks for a locked recipe to be reopened (P10).
//
// The head moves APPROVED/VALIDATED → UNLOCK_REQUESTED and stays LOCKED while it waits
// for a decision.
type RequestUnlockCommand struct {
	MbhID uuid.UUID
	// Reason is MANDATORY. The domain returns mbhead.ErrReasonRequired for an empty or
	// whitespace-only value — an unlock reopens already-approved content, so the record
	// must say why.
	Reason      string
	ActorUserID string
	// ActorUserUUID is the requester's IAM user UUID, taken from the authenticated
	// context. ⛔ It is a DIFFERENT value from ActorUserID (a username) and the two must
	// never be conflated: only this one is addressable by IAM's BY_USER_ID rule.
	//
	// It is OPTIONAL — empty for unauthenticated/system callers — and an empty value
	// simply means the eventual grant/reject notifies nobody.
	ActorUserUUID string
}

// RequestUnlockHandler handles the RequestUnlockMBHead command.
type RequestUnlockHandler struct {
	repo mbhead.Repository
	// notifier is OPTIONAL: nil disables notifications.
	notifier Notifier
	// actors is OPTIONAL: nil disables requester-identity recording, which in turn
	// leaves E4/E5 silent for this request.
	actors UnlockActorStore
}

// WithNotifier attaches the MB recipe notifier and returns the handler for chaining.
func (h *RequestUnlockHandler) WithNotifier(n Notifier) *RequestUnlockHandler {
	h.notifier = n
	return h
}

// WithUnlockActors attaches the requester-identity store and returns the handler for
// chaining.
func (h *RequestUnlockHandler) WithUnlockActors(s UnlockActorStore) *RequestUnlockHandler {
	h.actors = s
	return h
}

// NewRequestUnlockHandler creates a new RequestUnlockHandler.
func NewRequestUnlockHandler(repo mbhead.Repository) *RequestUnlockHandler {
	return &RequestUnlockHandler{repo: repo}
}

// Handle executes the request-unlock transition.
//
// 🔴 K-53: this deliberately adds ⛔ NO guard requiring entity.IsLocked() to already be
// true. Legacy rows carry mbh_is_locked = NULL (which reads as "not locked", per 000485)
// while sitting in VALIDATED; refusing them would make the feature unusable on exactly
// the data it exists for. The domain enforces the same rule — do not re-add the check here.
//
// 🔴 Persistence goes through the EXISTING repo.Transition path, so the mst_mb_head lock
// columns, the mst_mb_workflow_log row and the mst_mb_head_lock_log UNLOCK_REQUEST row are
// all written in ONE transaction (DeriveLockEffect decides the lock side effects).
// ⛔ Do not add a second persist call for the lock columns.
//
// The reason forwarded to Transition is entity.UnlockReason(), not cmd.Reason: the domain
// TRIMS the reason, and the trimmed value is the one that must land in both
// mbh_unlock_reason and mbhl_reason.
func (h *RequestUnlockHandler) Handle(ctx context.Context, cmd RequestUnlockCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, err
	}

	fromState := entity.EntryStatus()
	if err := entity.RequestUnlock(cmd.ActorUserID, cmd.Reason); err != nil {
		return nil, err
	}

	reason := ""
	if r := entity.UnlockReason(); r != nil {
		reason = *r
	}

	// ⛔ stateReason is deliberately NOT used as the transition reason here. An unlock
	// request is not a workflow refusal; entity.RequestUnlock stores its reason in
	// unlockReason and leaves stateReason untouched (principle U-2 — the previous state
	// reason stays readable).
	if err := h.repo.Transition(ctx, entity.ID(), fromState, entity.EntryStatus(),
		entity.CurrentVersion(), reason, cmd.ActorUserID, nil); err != nil {
		return nil, err
	}

	h.recordRequester(ctx, entity.ID(), cmd.ActorUserUUID)

	// Best-effort notification to whoever can decide the unlock. Fired AFTER the commit.
	emitEvent(ctx, h.notifier, Event{
		EventType:   EventUnlockRequested,
		MbhID:       entity.ID(),
		MBCosting:   entity.MBCosting(),
		FromState:   fromState,
		ToState:     entity.EntryStatus(),
		Version:     entity.CurrentVersion(),
		ActorUserID: cmd.ActorUserID,
		Rules:       []NotifRule{{RuleType: RuleByPermission, Value: PermMBRecipeUnlock}},
	})

	return entity, nil
}

// recordRequester stamps the requester's UUID onto the UNLOCK_REQUEST lock-log row that
// the transition above just committed.
//
// 🔴 BEST-EFFORT, and deliberately AFTER the commit rather than inside it. It writes to
// mbhl_meta only — a column that has existed unused since 000485 — so a failure costs at
// most the later E4/E5 notification and ⛔ can never fail or roll back an unlock request
// that the user legitimately made.
//
// An empty or malformed UUID is dropped here rather than stored: storing a value IAM
// cannot parse would only move the failure to grant time.
func (h *RequestUnlockHandler) recordRequester(ctx context.Context, mbhID uuid.UUID, actorUUID string) {
	if h.actors == nil || actorUUID == "" {
		return
	}
	if _, err := uuid.Parse(actorUUID); err != nil {
		log.Warn().Str("mbh_id", mbhID.String()).Str("actor", actorUUID).
			Msg("mbhead: unlock requester identity is not a UUID; not recorded")
		return
	}
	if err := h.actors.RecordUnlockRequestActor(ctx, mbhID, actorUUID); err != nil {
		log.Warn().Err(err).Str("mbh_id", mbhID.String()).
			Msg("mbhead: recording unlock requester failed (non-fatal)")
	}
}

// GrantUnlockCommand approves a pending unlock request (P10).
//
// The head moves UNLOCK_REQUESTED → DRAFT, is unlocked, and the auto-relock deadline
// (mbhead.AutoRelockAfter) is recorded on the lock-log row.
//
// ⛔ No Reason field: granting is an assent, not a refusal, and the ORIGINAL request
// reason is what stays on record (it is never cleared).
type GrantUnlockCommand struct {
	MbhID       uuid.UUID
	ActorUserID string
}

// GrantUnlockHandler handles the GrantUnlockMBHead command.
type GrantUnlockHandler struct {
	repo mbhead.Repository
	// notifier is OPTIONAL: nil disables notifications.
	notifier Notifier
	// actors is OPTIONAL: nil means the requester cannot be recovered, so E4 stays silent.
	actors UnlockActorStore
}

// WithNotifier attaches the MB recipe notifier and returns the handler for chaining.
func (h *GrantUnlockHandler) WithNotifier(n Notifier) *GrantUnlockHandler {
	h.notifier = n
	return h
}

// WithUnlockActors attaches the requester-identity store and returns the handler for
// chaining.
func (h *GrantUnlockHandler) WithUnlockActors(s UnlockActorStore) *GrantUnlockHandler {
	h.actors = s
	return h
}

// NewGrantUnlockHandler creates a new GrantUnlockHandler.
func NewGrantUnlockHandler(repo mbhead.Repository) *GrantUnlockHandler {
	return &GrantUnlockHandler{repo: repo}
}

// Handle executes the grant-unlock transition.
//
// Returns mbhead.ErrUnlockNotRequested when there is nothing pending — ⛔ never a silent
// success. Persistence is the single existing repo.Transition transaction, which clears the
// pending markers, writes mbh_is_locked = FALSE, and appends the UNLOCK_GRANT lock-log row
// carrying mbhl_auto_relock_at.
//
// 🔴 entity.StateReason() is forwarded UNCHANGED — granting does not rewrite the stored
// state reason, and blanking it would erase the trail (principle U-2).
//
// 🔴 E4 — the notification goes to the REQUESTER, ⛔ not to the approver who is standing
// here. The requester's UUID is read BEFORE entity.GrantUnlock runs: that method clears
// the pending markers (domain/mbhead/lock.go GrantUnlock), and reading identity after it
// would be reading a deliberately emptied record. The rule is emitted only when a real
// UUID came back — see requesterRule.
func (h *GrantUnlockHandler) Handle(ctx context.Context, cmd GrantUnlockCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, err
	}

	// ⛔ BEFORE GrantUnlock — see the doc comment.
	rules, hasRequester := requesterRule(ctx, h.actors, entity.ID())

	fromState := entity.EntryStatus()
	if err := entity.GrantUnlock(cmd.ActorUserID); err != nil {
		return nil, err
	}

	if err := h.repo.Transition(ctx, entity.ID(), fromState, entity.EntryStatus(),
		entity.CurrentVersion(), entity.StateReason(), cmd.ActorUserID, nil); err != nil {
		return nil, err
	}

	if hasRequester {
		emitEvent(ctx, h.notifier, Event{
			EventType:   EventUnlockGranted,
			MbhID:       entity.ID(),
			MBCosting:   entity.MBCosting(),
			FromState:   fromState,
			ToState:     entity.EntryStatus(),
			Version:     entity.CurrentVersion(),
			ActorUserID: cmd.ActorUserID,
			Rules:       rules,
		})
	}

	return entity, nil
}

// RejectUnlockCommand refuses a pending unlock request (P10, K-52).
//
// The head returns to whichever locked state it was parked from (APPROVED or VALIDATED)
// and stays LOCKED.
type RejectUnlockCommand struct {
	MbhID uuid.UUID
	// Reason is MANDATORY, consistent with every other refusing transition on this
	// entity (Reject, UnApprove, Revoke).
	Reason      string
	ActorUserID string
}

// RejectUnlockHandler handles the RejectUnlockMBHead command.
type RejectUnlockHandler struct {
	repo mbhead.Repository
	// notifier is OPTIONAL: nil disables notifications.
	notifier Notifier
	// actors is OPTIONAL: nil means the requester cannot be recovered, so E5 stays silent.
	actors UnlockActorStore
}

// WithNotifier attaches the MB recipe notifier and returns the handler for chaining.
func (h *RejectUnlockHandler) WithNotifier(n Notifier) *RejectUnlockHandler {
	h.notifier = n
	return h
}

// WithUnlockActors attaches the requester-identity store and returns the handler for
// chaining.
func (h *RejectUnlockHandler) WithUnlockActors(s UnlockActorStore) *RejectUnlockHandler {
	h.actors = s
	return h
}

// NewRejectUnlockHandler creates a new RejectUnlockHandler.
func NewRejectUnlockHandler(repo mbhead.Repository) *RejectUnlockHandler {
	return &RejectUnlockHandler{repo: repo}
}

// Handle executes the reject-unlock transition.
//
// 🔴 K-52: when the state the head was parked from cannot be established (no
// UNLOCK_REQUESTED row in mst_mb_workflow_log to restore PreUnlockStatus from), the domain
// returns mbhead.ErrUnlockOriginUnknown and NOTHING is persisted. ⛔ This handler must not
// pick a target: sending a VALIDATED recipe back as merely APPROVED — or the reverse —
// would silently rewrite its costing standing.
//
// 🔴 K-54: exactly ONE lock-log row (UNLOCK_REJECT) is written, because there is exactly
// ONE repo.Transition call. ⛔ Do not add a second call to record the relock — the relock
// is part of the refusal, and DeriveLockEffect already sets mbh_is_locked = TRUE on this
// same transition.
//
// 🔴 K-55 — FIXED (was: refusing an unlock on a VALIDATED-origin head failed at runtime).
// This handler calls repo.Transition with toState = VALIDATED at entity.CurrentVersion(),
// which RejectUnlock deliberately does NOT bump. The transition used to snapshot the
// composition for EVERY toState == VALIDATED, so it re-inserted a version the original
// Validate had already snapshotted and uq_mbcv_seq (000440:16) rolled the whole
// transaction back. The fix is option (a), in the transition path where it belongs:
// postgres.snapshotOnTransition skips the snapshot when fromState == UNLOCK_REQUESTED,
// leaving normal DRAFT → VALIDATED snapshots untouched. ⛔ Do not also add ON CONFLICT DO
// NOTHING to SnapshotVersion — see that function's comment for why the constraint must
// stay loud. APPROVED-origin refusals were never affected.
//
// 🔴 E5 — the notification goes to the REQUESTER, ⛔ not to the approver refusing here,
// and the identity is read BEFORE entity.RejectUnlock clears the pending markers
// (domain/mbhead/lock.go RejectUnlock). No UUID means no rule and no notification at all.
func (h *RejectUnlockHandler) Handle(ctx context.Context, cmd RejectUnlockCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, err
	}

	// ⛔ BEFORE RejectUnlock — see the doc comment.
	rules, hasRequester := requesterRule(ctx, h.actors, entity.ID())

	fromState := entity.EntryStatus()
	if err := entity.RejectUnlock(cmd.ActorUserID, cmd.Reason); err != nil {
		return nil, err
	}

	if err := h.repo.Transition(ctx, entity.ID(), fromState, entity.EntryStatus(),
		entity.CurrentVersion(), entity.StateReason(), cmd.ActorUserID, nil); err != nil {
		return nil, err
	}

	if hasRequester {
		emitEvent(ctx, h.notifier, Event{
			EventType:   EventUnlockRejected,
			MbhID:       entity.ID(),
			MBCosting:   entity.MBCosting(),
			FromState:   fromState,
			ToState:     entity.EntryStatus(),
			Version:     entity.CurrentVersion(),
			ActorUserID: cmd.ActorUserID,
			Rules:       rules,
		})
	}

	return entity, nil
}
