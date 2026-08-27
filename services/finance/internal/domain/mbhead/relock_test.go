package mbhead

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// DeriveRelockEffect
// ---------------------------------------------------------------------------

func TestDeriveRelockEffect(t *testing.T) {
	tests := []struct {
		name      string
		toState   string
		wantEvent string
		wantLock  *bool
	}{
		{"back to approved", StatusApproved, LockEventRelock, boolPtr(true)},
		{"back to validated", StatusValidated, LockEventRelock, boolPtr(true)},
		{"draft is not lockable", StatusDraft, "", nil},
		{"submitted is not lockable", StatusSubmitted, "", nil},
		{"unlock_requested is not lockable", StatusUnlockRequested, "", nil},
		{"revoked is not lockable", StatusRevoked, "", nil},
		{"empty target", "", "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveRelockEffect(tc.toState)
			assert.Equal(t, tc.wantEvent, got.Event)
			if tc.wantLock == nil {
				assert.Nil(t, got.SetLocked)
			} else {
				require.NotNil(t, got.SetLocked)
				assert.Equal(t, *tc.wantLock, *got.SetLocked)
			}
			// A relock never re-arms the deadline it just consumed, and never touches
			// the pending-request markers — GrantUnlock already cleared those.
			assert.False(t, got.SetAutoRelock)
			assert.False(t, got.SetUnlockRequest)
			assert.False(t, got.ClearUnlockRequest)
		})
	}
}

// TestDeriveRelockEffect_EventIsCheckConstraintValue mirrors
// TestDeriveLockEffect_EventsAreCheckConstraintValues: RELOCK must be one of the five
// values migration 000485's CHECK on mbhl_event accepts, or the INSERT fails in
// production rather than here.
func TestDeriveRelockEffect_EventIsCheckConstraintValue(t *testing.T) {
	allowed := map[string]bool{
		"LOCK": true, "UNLOCK_REQUEST": true, "UNLOCK_GRANT": true,
		"UNLOCK_REJECT": true, "RELOCK": true,
	}
	all := []string{
		StatusDraft, StatusSubmitted, StatusApproved, StatusValidated,
		StatusUnApproved, StatusRevoked, StatusRejected, StatusUnlockRequested,
	}
	for _, to := range all {
		ev := DeriveRelockEffect(to).Event
		if ev == "" {
			continue
		}
		assert.Truef(t, allowed[ev],
			"relock to %s emits %q, which chk on mbhl_event (000485) would REJECT", to, ev)
	}
	assert.True(t, allowed[LockEventRelock], "LockEventRelock is not in the 000485 CHECK list")
}

// TestDeriveLockEffect_UnchangedByRelockPath pins that adding the relock path did ⛔ NOT
// move DeriveLockEffect's answer for any existing input pair. The relock path lives in a
// separate function precisely so this stays true; if DeriveLockEffect ever starts
// answering RELOCK, this test says so.
func TestDeriveLockEffect_UnchangedByRelockPath(t *testing.T) {
	all := []string{
		StatusDraft, StatusSubmitted, StatusApproved, StatusValidated,
		StatusUnApproved, StatusRevoked, StatusRejected, StatusUnlockRequested,
	}
	for _, from := range all {
		for _, to := range all {
			got := DeriveLockEffect(from, to)
			assert.NotEqualf(t, LockEventRelock, got.Event,
				"DeriveLockEffect(%s, %s) must not emit RELOCK — that is DeriveRelockEffect's job", from, to)
		}
	}
	// Spot-pin the four decided arms verbatim.
	assert.Equal(t, LockEventUnlockRequest, DeriveLockEffect(StatusApproved, StatusUnlockRequested).Event)
	assert.Equal(t, LockEventUnlockGrant, DeriveLockEffect(StatusUnlockRequested, StatusDraft).Event)
	assert.Equal(t, LockEventUnlockReject, DeriveLockEffect(StatusUnlockRequested, StatusValidated).Event)
	assert.Equal(t, LockEventLock, DeriveLockEffect(StatusSubmitted, StatusApproved).Event)
	assert.Equal(t, LockEventLock, DeriveLockEffect(StatusDraft, StatusApproved).Event)
	assert.Empty(t, DeriveLockEffect(StatusDraft, StatusSubmitted).Event)
}

// ---------------------------------------------------------------------------
// canAutoRelock — the separate gate that keeps allowedTransitions untouched
// ---------------------------------------------------------------------------

func TestCanAutoRelock(t *testing.T) {
	assert.True(t, canAutoRelock(StatusDraft, StatusApproved))
	assert.True(t, canAutoRelock(StatusDraft, StatusValidated))
	assert.False(t, canAutoRelock(StatusDraft, StatusSubmitted))
	assert.False(t, canAutoRelock(StatusSubmitted, StatusApproved))
	assert.False(t, canAutoRelock(StatusUnlockRequested, StatusApproved))
	assert.False(t, canAutoRelock(StatusApproved, StatusApproved))
}

// TestCanAutoRelock_DoesNotWidenCanTransition is the whole point of the separate gate:
// DRAFT → APPROVED must still be ILLEGAL as a user-initiated transition (it would skip
// SUBMITTED), even though the auto-relock path performs exactly that move.
func TestCanAutoRelock_DoesNotWidenCanTransition(t *testing.T) {
	assert.False(t, canTransition(StatusDraft, StatusApproved),
		"DRAFT → APPROVED must stay illegal for users — approving without submitting breaks the workflow")
	assert.True(t, canAutoRelock(StatusDraft, StatusApproved))
}

// ---------------------------------------------------------------------------
// Entity.AutoRelock
// ---------------------------------------------------------------------------

func TestAutoRelock_ReturnsToOriginAndLocks(t *testing.T) {
	for _, target := range []string{StatusApproved, StatusValidated} {
		t.Run(target, func(t *testing.T) {
			e := newEntityAt(StatusDraft, false)
			reason := "customer changed the shade"
			e.unlockReason = &reason
			e.preUnlockStatus = target

			require.NoError(t, e.AutoRelock(target))
			assert.Equal(t, target, e.EntryStatus())
			assert.True(t, e.IsLocked())
			assert.False(t, e.IsEditable())
			require.NotNil(t, e.LockedBy())
			assert.Equal(t, SystemActorID, *e.LockedBy())
			assert.NotNil(t, e.LockedAt())
			// U-2: the reason the unlock was asked for is ⛔ never erased.
			require.NotNil(t, e.UnlockReason())
			assert.Equal(t, reason, *e.UnlockReason())
			assert.Empty(t, e.PreUnlockStatus())
		})
	}
}

func TestAutoRelock_RejectsIllegalTarget(t *testing.T) {
	for _, target := range []string{StatusDraft, StatusSubmitted, StatusRejected, StatusRevoked, StatusUnApproved, StatusUnlockRequested, ""} {
		t.Run("target="+target, func(t *testing.T) {
			e := newEntityAt(StatusDraft, false)
			err := e.AutoRelock(target)
			require.ErrorIs(t, err, ErrUnlockOriginUnknown)
			// ⛔ Nothing moved: a refused relock leaves the head exactly as it was.
			assert.Equal(t, StatusDraft, e.EntryStatus())
			assert.False(t, e.IsLocked())
		})
	}
}

func TestAutoRelock_RejectsAlreadyLockedHead(t *testing.T) {
	e := newLockedEntityAt(StatusDraft)
	err := e.AutoRelock(StatusApproved)
	require.ErrorIs(t, err, ErrHeadLocked)
	assert.Equal(t, StatusDraft, e.EntryStatus())
}

func TestAutoRelock_RejectsHeadNotInDraft(t *testing.T) {
	for _, from := range []string{StatusSubmitted, StatusUnlockRequested, StatusRejected, StatusUnApproved} {
		t.Run("from="+from, func(t *testing.T) {
			e := newEntityAt(from, false)
			err := e.AutoRelock(StatusApproved)
			require.ErrorIs(t, err, ErrInvalidTransition)
			assert.Equal(t, from, e.EntryStatus())
			assert.False(t, e.IsLocked())
		})
	}
}

// TestSystemActorID_FitsColumn pins the sentinel against the storage it is written to:
// mbhl_actor_user_id is VARCHAR(20) NOT NULL (migration 000485).
func TestSystemActorID_FitsColumn(t *testing.T) {
	assert.NotEmpty(t, SystemActorID, "NOT NULL column — the sentinel must not be empty")
	assert.LessOrEqual(t, len(SystemActorID), 20, "mbhl_actor_user_id is VARCHAR(20)")
}
