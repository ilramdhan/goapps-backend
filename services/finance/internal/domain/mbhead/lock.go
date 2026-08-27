package mbhead

import (
	"strings"
	"time"
)

// AutoRelockAfter is how long a granted unlock stays open before the recipe is
// re-locked automatically. It is written to mst_mb_head_lock_log.mbhl_auto_relock_at
// at grant time (mbhl_auto_relock_at = now + AutoRelockAfter).
//
// ⚠ GERBANG KEPUTUSAN USER — BELUM DIKONFIRMASI VERBATIM. The 24-hour figure comes
// from the plan text and the COMMENT ON TABLE in migration 000485 ("Auto-relock 24
// jam ... konsisten dengan R9"); it has never been confirmed by the user directly.
// It lives here as a single named constant precisely so that changing it is a
// one-line edit rather than a hunt through SQL literals. ⛔ Do not inline the
// duration anywhere else.
//
// ⚠ Nothing ENFORCES this deadline yet. Writing mbhl_auto_relock_at only RECORDS
// the intent; the cron/worker that acts on it is explicitly out of scope for this
// phase. A head whose window has expired therefore stays unlocked until something
// re-locks it.
const AutoRelockAfter = 24 * time.Hour

// Lock marks the recipe as locked and records who locked it and when.
//
// 🔴 Called when a head ENTERS a state that lockOnEnter reports as lockable
// (APPROVED, VALIDATED). It takes ⛔ no error return on purpose: locking is never
// a refusable operation — re-locking an already-locked head simply refreshes the
// actor/timestamp, which is exactly what an auto-relock or a re-approval should do.
//
// It deliberately does ⛔ NOT touch entryStatus. Moving the workflow state is the
// job of the transition methods; this only flips the lock flag alongside them.
func (e *Entity) Lock(actor string) {
	now := time.Now()
	e.isLocked = true
	e.lockedAt = &now
	if actor != "" {
		e.lockedBy = &actor
	}
	// ⛔ unlockRequestedAt/By are NOT cleared here. A relock that lands while a
	// request is still parked must not erase the request — the request's fate is
	// decided by GrantUnlock/RejectUnlock, never by a side effect of locking.
}

// RequestUnlock parks a locked recipe in UNLOCK_REQUESTED so an approver can decide.
//
// The reason is MANDATORY: an unlock reopens content that was already approved, so
// the record must say why. Whitespace-only input counts as empty (same rule as
// costproductrequest.Reject) and returns ErrReasonRequired.
//
// 🔴 It records the state it came FROM in preUnlockStatus, so RejectUnlock can put
// the head back exactly there instead of guessing between APPROVED and VALIDATED.
//
// ⚠ It does ⛔ NOT require isLocked to already be true. Every legacy row carries
// mbh_is_locked = NULL (reads as "not locked", per 000485) while sitting in
// VALIDATED — 4190 of them in production. Refusing the request for those rows
// would make the feature unusable on exactly the data it exists for, and no user
// decision says to refuse it. The request is accepted; the lock flag is whatever
// storage says it is.
func (e *Entity) RequestUnlock(actor, reason string) error {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return ErrReasonRequired
	}
	if !canRequestUnlock(e.entryStatus) || !canTransition(e.entryStatus, StatusUnlockRequested) {
		return ErrInvalidTransition
	}
	now := time.Now()
	e.preUnlockStatus = e.entryStatus
	e.entryStatus = StatusUnlockRequested
	e.unlockRequestedAt = &now
	e.unlockRequestedBy = &actor
	e.unlockReason = &trimmed
	e.RecomputeCheckStatusCalc()
	return nil
}

// GrantUnlock approves a pending unlock request: the recipe is unlocked and returns
// to DRAFT so its author can edit it again.
//
// Returns ErrUnlockNotRequested when there is nothing pending — either the head is
// not parked in UNLOCK_REQUESTED, or no request timestamp is stored. ⛔ Never a
// silent success.
//
// 🔴 unlockReason is PRESERVED, ⛔ never cleared: the reason the unlock was asked
// for must stay readable afterwards (principle U-2). Only the PENDING markers
// (unlockRequestedAt/By) are cleared, because those are what "a request is waiting"
// means — leaving them set would let the same request be granted twice.
//
// 🔴 lockedAt/lockedBy are also PRESERVED as the trail of the lock being lifted.
// isLocked alone carries the live truth.
func (e *Entity) GrantUnlock(actor string) error {
	if !canGrantUnlock(e.entryStatus) || e.unlockRequestedAt == nil {
		return ErrUnlockNotRequested
	}
	if !canTransition(e.entryStatus, StatusDraft) {
		return ErrInvalidTransition
	}
	e.entryStatus = StatusDraft
	e.isLocked = false
	e.unlockRequestedAt = nil
	e.unlockRequestedBy = nil
	if actor != "" {
		e.updatedBy = &actor
	}
	now := time.Now()
	e.updatedAt = &now
	e.RecomputeCheckStatusCalc()
	return nil
}

// RejectUnlock refuses a pending unlock request: the recipe stays locked and returns
// to whichever state it was parked from (APPROVED or VALIDATED).
//
// The reason is MANDATORY, consistent with every other refusing transition in this
// entity (Reject, UnApprove, Revoke). Whitespace-only counts as empty.
//
// Returns ErrUnlockNotRequested when nothing is pending. Returns
// ErrUnlockOriginUnknown when the state to return to cannot be established —
// ⛔ it does NOT pick one: sending a VALIDATED recipe back as merely APPROVED (or
// the reverse) would silently rewrite its costing standing.
func (e *Entity) RejectUnlock(actor, reason string) error {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return ErrReasonRequired
	}
	if !canGrantUnlock(e.entryStatus) || e.unlockRequestedAt == nil {
		return ErrUnlockNotRequested
	}
	target := e.preUnlockStatus
	if !canRequestUnlock(target) || !canTransition(e.entryStatus, target) {
		return ErrUnlockOriginUnknown
	}
	e.entryStatus = target
	e.preUnlockStatus = ""
	e.unlockRequestedAt = nil
	e.unlockRequestedBy = nil
	e.stateReason = trimmed
	// The head was locked before it was parked and stays locked now — refusing an
	// unlock cannot leave the recipe open.
	e.Lock(actor)
	e.RecomputeCheckStatusCalc()
	return nil
}

// Lock-log event names. 🔴 These are the ONLY values mst_mb_head_lock_log.mbhl_event
// accepts — its CHECK constraint (migration 000485) lists exactly five. ⛔ Do not
// invent a sixth here: a new event name needs a new migration AND a user decision,
// in that order.
const (
	// LockEventLock — the recipe was locked on entering APPROVED or VALIDATED.
	LockEventLock = "LOCK"
	// LockEventUnlockRequest — someone asked for the recipe to be unlocked.
	LockEventUnlockRequest = "UNLOCK_REQUEST"
	// LockEventUnlockGrant — the request was approved; the recipe is now editable.
	LockEventUnlockGrant = "UNLOCK_GRANT"
	// LockEventUnlockReject — the request was refused; the recipe stays locked and
	// returns to the state it was parked from. 🔴 One row, not two: the relock is
	// part of the refusal, so no separate RELOCK row is written for it.
	LockEventUnlockReject = "UNLOCK_REJECT"
	// LockEventRelock — the recipe was re-locked without a refusal, i.e. by the
	// auto-relock deadline expiring. ⚠ NOT produced by any code path in this phase:
	// the job that acts on mbhl_auto_relock_at is out of scope. The constant exists
	// so that job has a name to use and does not invent one.
	LockEventRelock = "RELOCK"
)

// LockEffect describes what a workflow transition must do to the lock columns and
// to mst_mb_head_lock_log. It is the ONE place the lock side effects of a transition
// are decided, so the repository never re-derives them from state names itself.
type LockEffect struct {
	// Event is the mbhl_event value to log, or "" when this transition touches
	// nothing lock-related and no log row should be written.
	Event string
	// SetLocked, when non-nil, is the value to write to mbh_is_locked (plus
	// mbh_locked_at/by when true). nil leaves the column alone.
	SetLocked *bool
	// SetUnlockRequest records a new pending request (mbh_unlock_requested_at/by and
	// mbh_unlock_reason).
	SetUnlockRequest bool
	// ClearUnlockRequest clears the pending-request markers. ⛔ It never clears
	// mbh_unlock_reason — that stays as the trail of why the unlock was asked for.
	ClearUnlockRequest bool
	// SetAutoRelock records mbhl_auto_relock_at = now + AutoRelockAfter on the log row.
	SetAutoRelock bool
}

// DeriveLockEffect computes the lock side effects of a fromState → toState move.
// PURE: same input, same output, no clock, no I/O.
//
// An empty Event means "this transition is not lock-related" — the caller then writes
// no lock-log row and touches no lock column.
func DeriveLockEffect(fromState, toState string) LockEffect {
	locked, unlocked := true, false
	switch {
	case toState == StatusUnlockRequested:
		// Parked awaiting a decision. The head STAYS locked while parked.
		return LockEffect{Event: LockEventUnlockRequest, SetUnlockRequest: true}
	case fromState == StatusUnlockRequested && toState == StatusDraft:
		// Granted: editable again, and the auto-relock deadline starts running.
		return LockEffect{
			Event: LockEventUnlockGrant, SetLocked: &unlocked,
			ClearUnlockRequest: true, SetAutoRelock: true,
		}
	case fromState == StatusUnlockRequested && lockOnEnter(toState):
		// Refused: back to APPROVED/VALIDATED and still locked.
		return LockEffect{
			Event: LockEventUnlockReject, SetLocked: &locked, ClearUnlockRequest: true,
		}
	case lockOnEnter(toState):
		// Normal approval/validation locks the recipe.
		return LockEffect{Event: LockEventLock, SetLocked: &locked}
	default:
		return LockEffect{}
	}
}

// SystemActorID is the mbhl_actor_user_id written for lock-log rows produced by an
// automated path rather than a person — today only the auto-relock job.
//
// 🔴 mbhl_actor_user_id is NOT NULL (migration 000485) and VARCHAR(20), so a machine
// action still has to name an actor. "SYSTEM" is chosen because it is unmistakably
// not a user id (real ones are numeric employee codes), fits the column at 6 of 20
// characters, and reads correctly in the audit trail without a lookup. ⛔ Do not use
// an empty string: the column would reject it in spirit (it is the "unknown actor"
// value the audit trail must never contain) and NULLIF would turn it into NULL,
// which the NOT NULL constraint rejects outright.
const SystemActorID = "SYSTEM"

// DeriveRelockEffect computes the lock side effects of an AUTO-RELOCK move, i.e. the
// deadline in mbhl_auto_relock_at expiring while the head sat in DRAFT.
// PURE: same input, same output, no clock, no I/O.
//
// 🔴 This is a SEPARATE function from DeriveLockEffect on purpose. The relock target
// is APPROVED or VALIDATED, and DeriveLockEffect's `case lockOnEnter(toState)` arm
// already claims DRAFT → APPROVED for the NORMAL approval path and answers LOCK.
// From the state names alone the two are indistinguishable — a normal approval and an
// auto-relock differ only in WHO is moving the head and why. Rather than smuggle that
// distinction into DeriveLockEffect (whose output for every existing input pair is
// pinned by TestDeriveLockEffect_* and must not move), the caller that KNOWS it is
// performing an auto-relock asks this function instead.
//
// An empty Event means toState is not a lockable state, so no relock is possible.
func DeriveRelockEffect(toState string) LockEffect {
	if !lockOnEnter(toState) {
		return LockEffect{}
	}
	locked := true
	// ⛔ No ClearUnlockRequest: GrantUnlock already cleared the pending markers when
	// the window opened. ⛔ No SetAutoRelock: the deadline has just been consumed, and
	// re-arming it on a head that is now locked would schedule a relock of a relock.
	return LockEffect{Event: LockEventRelock, SetLocked: &locked}
}

// AutoRelock closes an expired unlock window: the head returns to the state it was
// unlocked FROM and is locked again, exactly as if the unlock had never been granted
// (user decision K-57 option (a)). ⛔ It does NOT leave the head in DRAFT and ⛔ does
// not leave it open.
//
// target must be the head's pre-unlock status (APPROVED or VALIDATED). The caller
// reads it from PreUnlockStatus; ⛔ this method never guesses — an unknown target is
// ErrUnlockOriginUnknown, consistent with RejectUnlock's refusal to invent one.
//
// Returns ErrHeadLocked when the head is already locked: the window is not open, so
// there is nothing to close, and re-locking would write a spurious RELOCK audit row.
// Returns ErrInvalidTransition when the head is not sitting in DRAFT — an expired
// window only ever applies to a head that GrantUnlock parked there.
//
// 🔴 unlockReason is PRESERVED, ⛔ never cleared (principle U-2): the reason the
// unlock was asked for stays readable after the window closes.
func (e *Entity) AutoRelock(target string) error {
	if e.isLocked {
		return ErrHeadLocked
	}
	if !lockOnEnter(target) {
		return ErrUnlockOriginUnknown
	}
	if !canAutoRelock(e.entryStatus, target) {
		return ErrInvalidTransition
	}
	e.entryStatus = target
	e.preUnlockStatus = ""
	e.Lock(SystemActorID)
	e.RecomputeCheckStatusCalc()
	return nil
}
