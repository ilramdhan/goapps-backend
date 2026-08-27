package mbhead

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newLockedEntityAt builds an entity in `status` locked through the real Lock method,
// exactly as an APPROVED/VALIDATED head is once P10 locks it on entry — so lockedAt
// and lockedBy are stamped rather than left nil.
func newLockedEntityAt(status string) *Entity {
	e := newEntityAt(status, false)
	e.Lock("locker")
	return e
}

// ---------------------------------------------------------------------------
// State machine — UNLOCK_REQUESTED edges
// ---------------------------------------------------------------------------

func TestCanTransition_UnlockRequestedEdges(t *testing.T) {
	tests := []struct {
		name     string
		from, to string
		want     bool
	}{
		{"approved parks for unlock", StatusApproved, StatusUnlockRequested, true},
		{"validated parks for unlock", StatusValidated, StatusUnlockRequested, true},

		// ⛔ ILLEGAL — an editable or non-lockable state has nothing to unlock.
		{"draft cannot request unlock", StatusDraft, StatusUnlockRequested, false},
		{"submitted cannot request unlock", StatusSubmitted, StatusUnlockRequested, false},
		{"un_approved cannot request unlock", StatusUnApproved, StatusUnlockRequested, false},
		{"rejected cannot request unlock", StatusRejected, StatusUnlockRequested, false},
		{"revoked cannot request unlock", StatusRevoked, StatusUnlockRequested, false},

		// Exits from the parked state.
		{"grant goes to draft", StatusUnlockRequested, StatusDraft, true},
		{"reject returns to approved", StatusUnlockRequested, StatusApproved, true},
		{"reject returns to validated", StatusUnlockRequested, StatusValidated, true},

		// ⛔ Not exits.
		{"parked cannot go to submitted", StatusUnlockRequested, StatusSubmitted, false},
		{"parked cannot go to un_approved", StatusUnlockRequested, StatusUnApproved, false},
		{"parked cannot go to rejected", StatusUnlockRequested, StatusRejected, false},
		{"parked cannot self-transition", StatusUnlockRequested, StatusUnlockRequested, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canTransition(tc.from, tc.to))
		})
	}
}

// TestRevoked_StaysTerminal_WithUnlockRequested guards the one property the new state
// must NOT weaken: REVOKED remains the single terminal state, and the new holding
// state is reachable from neither direction of it.
func TestRevoked_StaysTerminal_WithUnlockRequested(t *testing.T) {
	assert.True(t, isTerminal(StatusRevoked))
	assert.False(t, isTerminal(StatusUnlockRequested))
	assert.False(t, canTransition(StatusRevoked, StatusUnlockRequested))
	assert.False(t, canTransition(StatusUnlockRequested, StatusRevoked),
		"REVOKED is reached via Revoke/canRevoke, ⛔ never through allowedTransitions")
	// ~~assert.True(t, canRevoke(StatusUnlockRequested), "a parked head may still be revoked")~~
	// 🔴 2026-08-26 (USER DECISION) — Revoke was REMOVED from the workflow, so a parked
	// head is no longer revocable either. Terminality of REVOKED is untouched above;
	// this line only ever described the ENTRANCE, which is now closed for every state.
	assert.False(t, canRevoke(StatusUnlockRequested), "2026-08-26: revoke was removed entirely")
}

func TestUnlockPredicates(t *testing.T) {
	tests := []struct {
		state                         string
		canRequest, canGrant, onEnter bool
	}{
		{StatusDraft, false, false, false},
		{StatusSubmitted, false, false, false},
		{StatusApproved, true, false, true},
		{StatusValidated, true, false, true},
		{StatusUnApproved, false, false, false},
		{StatusRejected, false, false, false},
		{StatusRevoked, false, false, false},
		{StatusUnlockRequested, false, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.state, func(t *testing.T) {
			assert.Equal(t, tc.canRequest, canRequestUnlock(tc.state), "canRequestUnlock")
			assert.Equal(t, tc.canGrant, canGrantUnlock(tc.state), "canGrantUnlock")
			assert.Equal(t, tc.onEnter, lockOnEnter(tc.state), "lockOnEnter")
		})
	}
}

// ---------------------------------------------------------------------------
// Entity behavior — Lock / IsEditable
// ---------------------------------------------------------------------------

func TestLock_StampsActorAndTime(t *testing.T) {
	e := newEntityAt(StatusApproved, false)
	require.False(t, e.IsLocked())

	e.Lock("user1")

	assert.True(t, e.IsLocked())
	require.NotNil(t, e.LockedBy())
	assert.Equal(t, "user1", *e.LockedBy())
	require.NotNil(t, e.LockedAt())
	assert.WithinDuration(t, time.Now(), *e.LockedAt(), 5*time.Second)
	assert.Equal(t, StatusApproved, e.EntryStatus(), "Lock must not move the workflow state")
}

// TestIsEditable_NullIsLockedMeansNotLocked pins the binding consequence of migration
// 000485: mbh_is_locked is NULL on every legacy row and NULL means NOT LOCKED. In Go
// that NULL surfaces as the zero-value false, which is exactly what a bare
// Reconstruct-ed entity carries — so it must be editable.
func TestIsEditable_NullIsLockedMeansNotLocked(t *testing.T) {
	e := newEntityAt(StatusValidated, false)
	assert.False(t, e.IsLocked(), "NULL in storage must read as not locked")
	assert.True(t, e.IsEditable(), "a NULL-lock legacy row must remain editable")

	// The same via the real hydration path, with the DTO reporting NULL → false.
	h := newEntityAt(StatusValidated, false)
	h.HydrateExtras(PersistedExtras{IsLocked: false})
	assert.True(t, h.IsEditable())
}

func TestIsEditable_LockedAndParked(t *testing.T) {
	assert.False(t, newLockedEntityAt(StatusApproved).IsEditable(), "locked head is not editable")
	assert.False(t, newLockedEntityAt(StatusValidated).IsEditable(), "locked head is not editable")

	parked := newEntityAt(StatusUnlockRequested, false)
	assert.False(t, parked.IsEditable(),
		"asking for an unlock is not the same as getting one — still not editable")

	deleted := newEntityAt(StatusDraft, false)
	require.NoError(t, deleted.SoftDelete("user1"))
	assert.False(t, deleted.IsEditable())
}

// TestUpdate_LockedHead_Rejected is the ⛔ no-silent-success guard: mutating a locked
// head must FAIL with ErrHeadLocked, not quietly drop the caller's edits.
func TestUpdate_LockedHead_Rejected(t *testing.T) {
	name := "changed"

	locked := newLockedEntityAt(StatusApproved)
	assert.ErrorIs(t, locked.Update(UpdateInput{MgtName: &name}, "user1"), ErrHeadLocked)
	assert.Nil(t, locked.MgtName(), "⛔ the edit must not have landed")

	parked := newEntityAt(StatusUnlockRequested, false)
	assert.ErrorIs(t, parked.Update(UpdateInput{MgtName: &name}, "user1"), ErrHeadLocked)

	// An unlocked head still updates normally — the guard must not break the base case.
	open := newEntityAt(StatusDraft, false)
	require.NoError(t, open.Update(UpdateInput{MgtName: &name}, "user1"))
	require.NotNil(t, open.MgtName())
	assert.Equal(t, "changed", *open.MgtName())
}

// ---------------------------------------------------------------------------
// Entity behavior — RequestUnlock
// ---------------------------------------------------------------------------

func TestRequestUnlock_ReasonMandatory(t *testing.T) {
	for _, reason := range []string{"", " ", "\t", "\n  \t "} {
		e := newLockedEntityAt(StatusValidated)
		assert.ErrorIsf(t, e.RequestUnlock("user1", reason), ErrReasonRequired,
			"whitespace-only reason %q must be refused", reason)
		assert.Equal(t, StatusValidated, e.EntryStatus(), "state must not have moved")
		assert.Nil(t, e.UnlockRequestedAt())
	}
}

func TestRequestUnlock_TrimsAndParks(t *testing.T) {
	for _, from := range []string{StatusApproved, StatusValidated} {
		t.Run(from, func(t *testing.T) {
			e := newLockedEntityAt(from)
			require.NoError(t, e.RequestUnlock("user1", "  fix the dozing  "))

			assert.Equal(t, StatusUnlockRequested, e.EntryStatus())
			assert.Equal(t, from, e.PreUnlockStatus(), "origin must be remembered")
			assert.True(t, e.IsLocked(), "head stays locked while parked")
			require.NotNil(t, e.UnlockRequestedBy())
			assert.Equal(t, "user1", *e.UnlockRequestedBy())
			require.NotNil(t, e.UnlockRequestedAt())
			require.NotNil(t, e.UnlockReason())
			assert.Equal(t, "fix the dozing", *e.UnlockReason(), "reason must be trimmed")
		})
	}
}

func TestRequestUnlock_IllegalStates(t *testing.T) {
	for _, from := range []string{
		StatusDraft, StatusSubmitted, StatusUnApproved,
		StatusRejected, StatusRevoked, StatusUnlockRequested,
	} {
		t.Run(from, func(t *testing.T) {
			e := newEntityAt(from, false)
			assert.ErrorIs(t, e.RequestUnlock("user1", "because"), ErrInvalidTransition)
			assert.Equal(t, from, e.EntryStatus())
		})
	}
}

// ---------------------------------------------------------------------------
// Entity behavior — GrantUnlock
// ---------------------------------------------------------------------------

// TestGrantUnlock_WithoutRequest_Refused covers both shapes of "nothing pending":
// a head that was never parked, and a head whose status says parked but which
// carries no request timestamp.
func TestGrantUnlock_WithoutRequest_Refused(t *testing.T) {
	for _, from := range []string{
		StatusDraft, StatusSubmitted, StatusApproved,
		StatusValidated, StatusUnApproved, StatusRejected, StatusRevoked,
	} {
		t.Run(from, func(t *testing.T) {
			e := newEntityAt(from, false)
			assert.ErrorIs(t, e.GrantUnlock("boss"), ErrUnlockNotRequested)
			assert.Equal(t, from, e.EntryStatus())
		})
	}

	t.Run("parked but no timestamp", func(t *testing.T) {
		e := newEntityAt(StatusUnlockRequested, false)
		assert.ErrorIs(t, e.GrantUnlock("boss"), ErrUnlockNotRequested)
	})
}

func TestGrantUnlock_UnlocksToDraft_PreservesReason(t *testing.T) {
	e := newLockedEntityAt(StatusValidated)
	require.NoError(t, e.RequestUnlock("user1", "wrong denier"))

	require.NoError(t, e.GrantUnlock("boss"))

	assert.Equal(t, StatusDraft, e.EntryStatus())
	assert.False(t, e.IsLocked())
	assert.True(t, e.IsEditable(), "the whole point of granting an unlock")
	assert.Nil(t, e.UnlockRequestedAt(), "pending markers cleared")
	assert.Nil(t, e.UnlockRequestedBy())
	require.NotNil(t, e.UnlockReason())
	assert.Equal(t, "wrong denier", *e.UnlockReason(),
		"⛔ the reason must NOT be erased — it is the trail of why the unlock happened")
	assert.NotNil(t, e.LockedAt(), "⛔ the lock trail stays readable after unlocking")

	// Granting twice must not succeed — the request is consumed.
	assert.ErrorIs(t, e.GrantUnlock("boss"), ErrUnlockNotRequested)
}

// ---------------------------------------------------------------------------
// Entity behavior — RejectUnlock
// ---------------------------------------------------------------------------

func TestRejectUnlock_ReturnsToOrigin_StaysLocked(t *testing.T) {
	for _, from := range []string{StatusApproved, StatusValidated} {
		t.Run(from, func(t *testing.T) {
			e := newLockedEntityAt(from)
			require.NoError(t, e.RequestUnlock("user1", "please"))

			require.NoError(t, e.RejectUnlock("boss", "  recipe is in production  "))

			assert.Equal(t, from, e.EntryStatus(), "must return EXACTLY to its origin")
			assert.True(t, e.IsLocked(), "a refused unlock cannot leave the recipe open")
			assert.False(t, e.IsEditable())
			assert.Equal(t, "recipe is in production", e.StateReason())
			assert.Nil(t, e.UnlockRequestedAt())
			assert.Empty(t, e.PreUnlockStatus(), "origin consumed")
		})
	}
}

func TestRejectUnlock_ReasonMandatory(t *testing.T) {
	for _, reason := range []string{"", "   ", "\t\n"} {
		e := newLockedEntityAt(StatusApproved)
		require.NoError(t, e.RequestUnlock("user1", "please"))
		assert.ErrorIs(t, e.RejectUnlock("boss", reason), ErrReasonRequired)
		assert.Equal(t, StatusUnlockRequested, e.EntryStatus(), "still parked")
	}
}

func TestRejectUnlock_WithoutRequest_Refused(t *testing.T) {
	e := newLockedEntityAt(StatusApproved)
	assert.ErrorIs(t, e.RejectUnlock("boss", "no"), ErrUnlockNotRequested)
}

// TestRejectUnlock_UnknownOrigin_Refused pins the ⛔ never-guess rule: a head loaded
// from storage whose workflow log holds no UNLOCK_REQUESTED row has no recoverable
// origin, and the domain must refuse rather than pick APPROVED or VALIDATED for it.
func TestRejectUnlock_UnknownOrigin_Refused(t *testing.T) {
	now := time.Now()
	e := newEntityAt(StatusUnlockRequested, false)
	e.isLocked = true
	e.unlockRequestedAt = &now
	// preUnlockStatus deliberately left empty — the log had nothing to restore.

	assert.ErrorIs(t, e.RejectUnlock("boss", "no"), ErrUnlockOriginUnknown)
	assert.Equal(t, StatusUnlockRequested, e.EntryStatus(), "still parked, nothing guessed")
}

// ---------------------------------------------------------------------------
// DeriveLockEffect — the single source of the persistence side effects
// ---------------------------------------------------------------------------

func TestDeriveLockEffect(t *testing.T) {
	tests := []struct {
		name           string
		from, to       string
		wantEvent      string
		wantSetLocked  *bool
		wantSetReq     bool
		wantClearReq   bool
		wantAutoRelock bool
	}{
		{
			name: "approve locks", from: StatusSubmitted, to: StatusApproved,
			wantEvent: LockEventLock, wantSetLocked: boolPtr(true),
		},
		{
			name: "validate locks", from: StatusApproved, to: StatusValidated,
			wantEvent: LockEventLock, wantSetLocked: boolPtr(true),
		},
		{
			name: "request parks and stays locked", from: StatusValidated, to: StatusUnlockRequested,
			wantEvent: LockEventUnlockRequest, wantSetReq: true,
		},
		{
			name: "grant unlocks and starts the relock clock",
			from: StatusUnlockRequested, to: StatusDraft,
			wantEvent: LockEventUnlockGrant, wantSetLocked: boolPtr(false),
			wantClearReq: true, wantAutoRelock: true,
		},
		{
			name: "reject relocks back to approved", from: StatusUnlockRequested, to: StatusApproved,
			wantEvent: LockEventUnlockReject, wantSetLocked: boolPtr(true), wantClearReq: true,
		},
		{
			name: "reject relocks back to validated", from: StatusUnlockRequested, to: StatusValidated,
			wantEvent: LockEventUnlockReject, wantSetLocked: boolPtr(true), wantClearReq: true,
		},
		// Transitions with no lock dimension at all — no log row, no column moved.
		{name: "submit", from: StatusDraft, to: StatusSubmitted},
		{name: "unapprove", from: StatusApproved, to: StatusUnApproved},
		{name: "reject submission", from: StatusSubmitted, to: StatusRejected},
		{name: "return to draft", from: StatusRejected, to: StatusDraft},
		{name: "revoke", from: StatusApproved, to: StatusRevoked},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveLockEffect(tc.from, tc.to)
			assert.Equal(t, tc.wantEvent, got.Event)
			assert.Equal(t, tc.wantSetLocked, got.SetLocked)
			assert.Equal(t, tc.wantSetReq, got.SetUnlockRequest)
			assert.Equal(t, tc.wantClearReq, got.ClearUnlockRequest)
			assert.Equal(t, tc.wantAutoRelock, got.SetAutoRelock)
		})
	}
}

// TestDeriveLockEffect_EventsAreCheckConstraintValues guards the ⛔ never-invent rule:
// every event this function can emit must be one of the five values migration 000485's
// CHECK on mbhl_event accepts. A sixth value would fail at INSERT time in production,
// not here — so it is pinned here instead.
func TestDeriveLockEffect_EventsAreCheckConstraintValues(t *testing.T) {
	allowed := map[string]bool{
		"LOCK": true, "UNLOCK_REQUEST": true, "UNLOCK_GRANT": true,
		"UNLOCK_REJECT": true, "RELOCK": true,
	}
	all := []string{
		StatusDraft, StatusSubmitted, StatusApproved, StatusValidated,
		StatusUnApproved, StatusRevoked, StatusRejected, StatusUnlockRequested,
	}
	for _, from := range all {
		for _, to := range all {
			ev := DeriveLockEffect(from, to).Event
			if ev == "" {
				continue
			}
			assert.Truef(t, allowed[ev],
				"%s -> %s emits %q, which chk on mbhl_event (000485) would REJECT", from, to, ev)
		}
	}
	// The constants themselves must match the CHECK list verbatim.
	for _, c := range []string{
		LockEventLock, LockEventUnlockRequest, LockEventUnlockGrant,
		LockEventUnlockReject, LockEventRelock,
	} {
		assert.Truef(t, allowed[c], "constant %q is not in the 000485 CHECK list", c)
	}
}

func boolPtr(b bool) *bool { return &b }
