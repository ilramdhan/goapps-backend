package mbhead

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeriveCheckStatus_DecidedMappings pins the three rules that ARE decided.
func TestDeriveCheckStatus_DecidedMappings(t *testing.T) {
	cases := []struct {
		name        string
		entryStatus string
		boughtout   bool
		want        string
	}{
		{"draft is waiting", StatusDraft, false, CheckStatusWaiting},
		{"submitted is waiting", StatusSubmitted, false, CheckStatusWaiting},
		{"approved is approved", StatusApproved, false, CheckStatusApproved},
		// Boughtout outranks every other rule — including states that would
		// otherwise return nil.
		{"boughtout beats draft", StatusDraft, true, CheckStatusBoughtout},
		{"boughtout beats approved", StatusApproved, true, CheckStatusBoughtout},
		{"boughtout beats validated", StatusValidated, true, CheckStatusBoughtout},
		{"boughtout beats revoked", StatusRevoked, true, CheckStatusBoughtout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveCheckStatus(tc.entryStatus, tc.boughtout)
			require.NotNil(t, got)
			assert.Equal(t, tc.want, *got)
		})
	}
}

// TestDeriveCheckStatus_UndecidedStatesReturnNil proves that the states whose
// mapping is still a USER GATE produce nil — "do not write" — rather than a
// guessed value. ⛔ Do not "fix" this test by giving these states a value; each
// one is an open decision recorded in the plan:
//   - VALIDATED → "Current" is hypothesis H1, blocked on verification V-A1.
//   - UN_APPROVED / REVOKED → design §c rule 8 offers only guesses.
func TestDeriveCheckStatus_UndecidedStatesReturnNil(t *testing.T) {
	for _, st := range []string{StatusValidated, StatusUnApproved, StatusRevoked, "SOMETHING_UNKNOWN", ""} {
		assert.Nil(t, DeriveCheckStatus(st, false), "state %q must stay undecided", st)
	}
}

// TestRecomputeCheckStatusCalc_UndecidedStateDoesNotErase is the reason
// DeriveCheckStatus returns a pointer instead of a string: moving into a state
// with no decided mapping must LEAVE the previously computed value alone. An
// undecided rule is not permission to wipe data.
func TestRecomputeCheckStatusCalc_UndecidedStateDoesNotErase(t *testing.T) {
	e := newEntityAt(StatusApproved, false)
	e.RecomputeCheckStatusCalc()
	require.NotNil(t, e.MBHCheckStatusCalc())
	assert.Equal(t, CheckStatusApproved, *e.MBHCheckStatusCalc())

	// APPROVED → VALIDATED: no decided mapping, so the value must survive untouched.
	require.NoError(t, e.Validate())
	require.NotNil(t, e.MBHCheckStatusCalc(), "an undecided state must not erase a computed value")
	assert.Equal(t, CheckStatusApproved, *e.MBHCheckStatusCalc())
}

// TestCheckStatusCalc_StartsNilAndStaysNilUntilComputed encodes the NULL semantics
// of the 207 legacy rows: never calculated is a real, permanent state, and it is
// nil — ⛔ never the empty string.
func TestCheckStatusCalc_StartsNilAndStaysNilUntilComputed(t *testing.T) {
	e := newEntityAt(StatusValidated, false)
	assert.Nil(t, e.MBHCheckStatusCalc(), "a legacy row that was never calculated is nil")

	e.RecomputeCheckStatusCalc()
	assert.Nil(t, e.MBHCheckStatusCalc(), "VALIDATED has no rule, so it stays nil — not \"\"")
}

// TestWorkflowTransitions_UpdateCheckStatusCalc walks the live workflow and asserts
// the derived value follows it.
func TestWorkflowTransitions_UpdateCheckStatusCalc(t *testing.T) {
	e := newEntityAt(StatusDraft, false)
	e.RecomputeCheckStatusCalc()
	require.Equal(t, CheckStatusWaiting, *e.MBHCheckStatusCalc())

	require.NoError(t, e.Submit())
	assert.Equal(t, CheckStatusWaiting, *e.MBHCheckStatusCalc())

	require.NoError(t, e.Approve())
	assert.Equal(t, CheckStatusApproved, *e.MBHCheckStatusCalc())
}

// TestNew_SeedsCheckStatusCalcFromDraft — a freshly created head is persisted with
// mbh_entry_status DRAFT (column DEFAULT), so its derived value must already be
// Waiting rather than nil.
func TestNew_SeedsCheckStatusCalcFromDraft(t *testing.T) {
	e, err := New(NewParams{MBCosting: "MB-CSC-1", CreatedBy: "tester"})
	require.NoError(t, err)
	require.NotNil(t, e.MBHCheckStatusCalc())
	assert.Equal(t, CheckStatusWaiting, *e.MBHCheckStatusCalc())

	b, err := New(NewParams{MBCosting: "MB-CSC-2", CreatedBy: "tester", IsBoughtout: true})
	require.NoError(t, err)
	require.NotNil(t, b.MBHCheckStatusCalc())
	assert.Equal(t, CheckStatusBoughtout, *b.MBHCheckStatusCalc())
}

// TestCheckStatusCalc_NeverWritesFrozenOracleColumn is the guard for user decision
// K-1 option 2: the derivation engine must never touch mbh_check_status.
func TestCheckStatusCalc_NeverWritesFrozenOracleColumn(t *testing.T) {
	oracle := "Current"
	e := newEntityAt(StatusDraft, false)
	e.mbhCheckStatus = &oracle

	e.RecomputeCheckStatusCalc()
	require.NoError(t, e.Submit())
	require.NoError(t, e.Approve())

	require.NotNil(t, e.MBHCheckStatus())
	assert.Equal(t, "Current", *e.MBHCheckStatus(), "the frozen Oracle trace must be untouched")
	assert.Equal(t, CheckStatusApproved, *e.MBHCheckStatusCalc())
}

// TestNew_DoesNotWriteFrozenOracleColumnFromDerivation — creating a head must not
// invent a value for the frozen column either.
func TestNew_DoesNotWriteFrozenOracleColumnFromDerivation(t *testing.T) {
	e, err := New(NewParams{MBCosting: "MB-CSC-3", CreatedBy: "tester"})
	require.NoError(t, err)
	assert.Nil(t, e.MBHCheckStatus(), "derivation must never populate the Oracle column")
}

// TestDeriveCheckStatus_Rejected pins user decision K-28: the REJECTED workflow
// state derives "Rejected" — EXCEPT for a boughtout head, where rule 1 still wins
// because Boughtout describes the NATURE of the item, not its workflow stage.
func TestDeriveCheckStatus_Rejected(t *testing.T) {
	got := DeriveCheckStatus(StatusRejected, false)
	require.NotNil(t, got)
	assert.Equal(t, CheckStatusRejected, *got)
	assert.Equal(t, "Rejected", *got, "must match chk_mbh_check_status_calc Title Case")

	bo := DeriveCheckStatus(StatusRejected, true)
	require.NotNil(t, bo)
	assert.Equal(t, CheckStatusBoughtout, *bo, "boughtout outranks REJECTED")
}

// TestRecomputeCheckStatusCalc_RejectSetsRejected walks the entity method so the
// derived column follows the new transition, not just the pure function.
func TestRecomputeCheckStatusCalc_RejectSetsRejected(t *testing.T) {
	e := newEntityAt(StatusDraft, false)
	e.RecomputeCheckStatusCalc()
	require.Equal(t, CheckStatusWaiting, *e.MBHCheckStatusCalc())

	require.NoError(t, e.Submit())
	require.NoError(t, e.Reject("out of spec"))
	require.NotNil(t, e.MBHCheckStatusCalc())
	assert.Equal(t, CheckStatusRejected, *e.MBHCheckStatusCalc())

	// A boughtout head that gets rejected keeps "Boughtout".
	b := newEntityAt(StatusSubmitted, true)
	require.NoError(t, b.Reject("out of spec"))
	require.NotNil(t, b.MBHCheckStatusCalc())
	assert.Equal(t, CheckStatusBoughtout, *b.MBHCheckStatusCalc())
}
