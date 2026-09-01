package mbhead

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newEntityAt builds a bare Entity with the given entryStatus/isBoughtout, bypassing
// New/Reconstruct (neither wires entryStatus yet). Test-only helper for this package.
func newEntityAt(status string, boughtout bool) *Entity {
	return &Entity{entryStatus: status, isBoughtout: boughtout}
}

func TestWorkflow_DraftSubmitApproveValidate_HappyPath(t *testing.T) {
	e := newEntityAt(StatusDraft, false)

	require.NoError(t, e.Submit())
	assert.Equal(t, StatusSubmitted, e.EntryStatus())

	require.NoError(t, e.Approve())
	assert.Equal(t, StatusApproved, e.EntryStatus())

	startVersion := e.CurrentVersion()
	require.NoError(t, e.Validate())
	assert.Equal(t, StatusValidated, e.EntryStatus())
	assert.Equal(t, startVersion+1, e.CurrentVersion())
}

func TestWorkflow_BoughtoutDraftToValidated_Shortcut(t *testing.T) {
	e := newEntityAt(StatusDraft, true)

	require.NoError(t, e.Validate())
	assert.Equal(t, StatusValidated, e.EntryStatus())
	assert.Equal(t, int32(1), e.CurrentVersion())
}

// TestValidate_DraftNonBoughtout_AllowedByStateMap documents actual behavior rather than the
// checklist's literal wording ("Validate from Draft fails for non-boughtout"): the non-boughtout
// branch of Validate delegates to canTransition(e.entryStatus, StatusValidated), and
// allowedTransitions[StatusDraft] includes StatusValidated (added for the boughtout shortcut but
// not conditioned on isBoughtout). Per task-9-brief.md, Validate/state_machine.go are transcribed
// verbatim and must not be modified, so this test asserts the transition succeeds as coded — the
// isBoughtout gate for this path is enforced by the caller/handler layer per design.md §2.1, not
// by this domain method alone. See task-9-report.md for the discrepancy write-up.
func TestValidate_DraftNonBoughtout_AllowedByStateMap(t *testing.T) {
	e := newEntityAt(StatusDraft, false)

	require.NoError(t, e.Validate())
	assert.Equal(t, StatusValidated, e.EntryStatus())
}

// ~~TestValidate_SubmittedNonBoughtout_Fails~~ → TestValidate_SubmittedNonBoughtout_NowSucceeds.
//
// 🔴 REWRITTEN 2026-08-26 (USER DECISION, "Opsi A") — the old test pinned "SUBMITTED has
// no direct path to VALIDATED". That is exactly the rule the user's decision reverses:
// the Validate button was removed from the screen and MB product generation moved onto
// Approve, so pressing Approve must carry a SUBMITTED recipe all the way to VALIDATED.
// The test is kept (⛔ not deleted, ⛔ not skipped) with its assertion inverted, so a
// future edit that quietly re-closes the edge fails right here.
//
// A genuinely illegal Validate origin is asserted below so the method is still shown to
// refuse something — REJECTED has no path to VALIDATED and must not gain one.
func TestValidate_SubmittedNonBoughtout_NowSucceeds(t *testing.T) {
	e := newEntityAt(StatusSubmitted, false)

	require.NoError(t, e.Validate())
	assert.Equal(t, StatusValidated, e.EntryStatus())
	assert.Equal(t, int32(1), e.CurrentVersion())

	// Still refused: REJECTED goes back to DRAFT, never forward to VALIDATED.
	r := newEntityAt(StatusRejected, false)
	assert.ErrorIs(t, r.Validate(), ErrInvalidTransition)
	assert.Equal(t, StatusRejected, r.EntryStatus())
}

// TestValidate_Boughtout_ReachesValidatedViaSubmitApprove is the boughtout half of the
// Opsi A decision, and the risk the decision created.
//
// A boughtout recipe used to reach VALIDATED ONLY by the DRAFT shortcut, pressed through
// the Validate button. That button is gone, so it now travels Submit → Approve like every
// other recipe. This pins that the journey actually completes — ⛔ a boughtout recipe must
// never be stranded in SUBMITTED.
func TestValidate_Boughtout_ReachesValidatedViaSubmitApprove(t *testing.T) {
	e := newEntityAt(StatusDraft, true)

	require.NoError(t, e.Submit())
	require.Equal(t, StatusSubmitted, e.EntryStatus())
	require.NoError(t, e.Validate(), "a boughtout recipe must not be stranded in SUBMITTED")
	assert.Equal(t, StatusValidated, e.EntryStatus())
}

// ~~TestUnApprove_RequiresReason~~ → TestUnApprove_IsRemoved_AlwaysRefuses.
//
// 🔴 REWRITTEN 2026-08-26 (USER DECISION) — Un-approve was REMOVED from the MB Recipe
// workflow. The old test pinned "an empty reason is rejected, a real one succeeds";
// that contract no longer exists, because UnApprove now refuses ⛔ regardless of the
// reason. The test is kept (⛔ not deleted, ⛔ not skipped) with its assertion inverted,
// so re-enabling the feature by accident fails here.
//
// The error is ErrInvalidTransition, ⛔ never ErrReasonRequired: the argument is not
// examined at all any more.
func TestUnApprove_IsRemoved_AlwaysRefuses(t *testing.T) {
	e := newEntityAt(StatusApproved, false)

	assert.ErrorIs(t, e.UnApprove(""), ErrInvalidTransition)
	assert.ErrorIs(t, e.UnApprove("quality issue"), ErrInvalidTransition)
	assert.Equal(t, StatusApproved, e.EntryStatus(), "a refused UnApprove must not mutate state")
	assert.Empty(t, e.StateReason(), "a refused UnApprove must not record a reason")
}

// ~~TestRevoke_FromAnyNonTerminalState~~ → TestRevoke_IsRemoved_RefusesFromEveryState.
//
// 🔴 REWRITTEN 2026-08-26 (USER DECISION) — Revoke was REMOVED from the MB Recipe
// workflow; the user's reasoning is that activating or deactivating a recipe belongs to
// the admin-facing active flag, so a terminal REVOKED status is not needed. The old test
// asserted the exact opposite of what the code must now do, so its table is reused with
// the expectation flipped rather than dropped.
func TestRevoke_IsRemoved_RefusesFromEveryState(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{"from draft", StatusDraft},
		{"from submitted", StatusSubmitted},
		{"from approved", StatusApproved},
		{"from validated", StatusValidated},
		{"from un_approved", StatusUnApproved},
		{"from rejected", StatusRejected},
		{"from unlock_requested", StatusUnlockRequested},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEntityAt(tt.status, false)

			assert.ErrorIs(t, e.Revoke("no longer needed"), ErrInvalidTransition)
			assert.Equal(t, tt.status, e.EntryStatus(), "a refused Revoke must not mutate state")
			assert.Empty(t, e.StateReason(), "a refused Revoke must not record a reason")
		})
	}
}

// TestRevoke_FromRevoked_Fails — unchanged by the 2026-08-26 removal, and kept
// deliberately: a row ALREADY sitting in REVOKED (production has some) must stay
// frozen. Only the way INTO REVOKED was closed, never the reading of rows there.
func TestRevoke_FromRevoked_Fails(t *testing.T) {
	e := newEntityAt(StatusRevoked, false)

	err := e.Revoke("try again")
	assert.ErrorIs(t, err, ErrInvalidTransition)
	assert.Equal(t, StatusRevoked, e.EntryStatus())
}

// ~~TestRevoke_RequiresReason~~ → TestRevoke_EmptyReason_AlsoRefuses.
//
// 🔴 REWRITTEN 2026-08-26 — the "reason is mandatory" contract is gone with the feature.
// An empty reason still fails, but now for a DIFFERENT reason: ErrInvalidTransition,
// ⛔ not ErrReasonRequired. Pinning the specific error keeps the two causes from being
// confused if the feature is ever revisited.
func TestRevoke_EmptyReason_AlsoRefuses(t *testing.T) {
	e := newEntityAt(StatusDraft, false)

	err := e.Revoke("")
	assert.ErrorIs(t, err, ErrInvalidTransition)
	assert.NotErrorIs(t, err, ErrReasonRequired)
	assert.Equal(t, StatusDraft, e.EntryStatus())
}

func TestSubmit_InvalidFromNonDraft(t *testing.T) {
	e := newEntityAt(StatusApproved, false)

	err := e.Submit()
	assert.ErrorIs(t, err, ErrInvalidTransition)
	assert.Equal(t, StatusApproved, e.EntryStatus())
}

func TestApprove_InvalidFromDraft(t *testing.T) {
	e := newEntityAt(StatusDraft, false)

	err := e.Approve()
	assert.ErrorIs(t, err, ErrInvalidTransition)
	assert.Equal(t, StatusDraft, e.EntryStatus())
}

func TestApprove_FromUnApproved_RevalidatePath(t *testing.T) {
	e := newEntityAt(StatusUnApproved, false)

	require.NoError(t, e.Approve())
	assert.Equal(t, StatusApproved, e.EntryStatus())
}

// ---------------------------------------------------------------------------
// REJECTED — user decisions K-2 / K-3 / K-24, executed 2026-08-23.
// ---------------------------------------------------------------------------

// TestReject_RequiresReason — K-2 makes the reason MANDATORY, and an empty one must
// leave the state untouched. ⛔ ErrReasonRequired, not ErrInvalidTransition: the
// transition itself is legal, only the argument is missing.
func TestReject_RequiresReason(t *testing.T) {
	e := newEntityAt(StatusSubmitted, false)

	err := e.Reject("")
	assert.ErrorIs(t, err, ErrReasonRequired)
	assert.Equal(t, StatusSubmitted, e.EntryStatus())
	assert.Empty(t, e.StateReason())
}

// TestReject_FromSubmitted_Succeeds is the happy path of the new edge.
func TestReject_FromSubmitted_Succeeds(t *testing.T) {
	e := newEntityAt(StatusSubmitted, false)

	require.NoError(t, e.Reject("formula does not match the sample"))
	assert.Equal(t, StatusRejected, e.EntryStatus())
	assert.Equal(t, "formula does not match the sample", e.StateReason())
}

// TestReject_FromNonSubmitted_Fails — SUBMITTED is the ONLY origin of the reject edge.
func TestReject_FromNonSubmitted_Fails(t *testing.T) {
	for _, status := range []string{
		StatusDraft, StatusApproved, StatusValidated,
		StatusUnApproved, StatusRevoked, StatusRejected,
	} {
		t.Run(status, func(t *testing.T) {
			e := newEntityAt(status, false)

			err := e.Reject("nope")
			assert.ErrorIs(t, err, ErrInvalidTransition)
			assert.Equal(t, status, e.EntryStatus(), "a failed Reject must not mutate state")
		})
	}
}

// TestWorkflow_SubmittedToDraft_IsIllegal is the proof of K-3: the old dead edge
// SUBMITTED → DRAFT is GONE. Rejection now routes through REJECTED.
func TestWorkflow_SubmittedToDraft_IsIllegal(t *testing.T) {
	assert.False(t, canTransition(StatusSubmitted, StatusDraft),
		"K-3 removed the dead SUBMITTED → DRAFT edge; rejection goes via REJECTED")
}

// TestWorkflow_RejectedToDraft_IsLegal — the return path added by K-2, so a rejected
// head can be reworked by its author.
func TestWorkflow_RejectedToDraft_IsLegal(t *testing.T) {
	assert.True(t, canTransition(StatusRejected, StatusDraft),
		"K-2 added REJECTED → DRAFT so a rejected head can be reworked")
}

// ~~TestRevoke_FromRejected_IsAllowed pins K-24: REJECTED is ⛔ NOT terminal.~~
// → TestRejected_StillNotTerminal_ButNoLongerRevocable.
//
// 🔴 REWRITTEN 2026-08-26 — K-24's point was that REJECTED is not a dead end. That is
// STILL true and still asserted: a rejected head returns to DRAFT. What changed is the
// escape route the old test used to demonstrate it — Revoke — which no longer exists.
// The non-terminality claim survives; only the revoke half was inverted.
func TestRejected_StillNotTerminal_ButNoLongerRevocable(t *testing.T) {
	assert.False(t, isTerminal(StatusRejected), "K-24: REJECTED must not be terminal")
	assert.False(t, canRevoke(StatusRejected), "2026-08-26: revoke was removed entirely")

	// The surviving way out of REJECTED is the K-29 return to DRAFT.
	e := newEntityAt(StatusSubmitted, false)
	require.NoError(t, e.Reject("wrong shade"))
	assert.ErrorIs(t, e.Revoke("order cancelled"), ErrInvalidTransition)
	assert.Equal(t, StatusRejected, e.EntryStatus())

	require.NoError(t, e.ReturnToDraft(""))
	assert.Equal(t, StatusDraft, e.EntryStatus())
	assert.Equal(t, "wrong shade", e.StateReason(), "K-29: the rejection reason survives")
}

// ---------------------------------------------------------------------------
// ReturnToDraft — user decision K-29, executed 2026-08-23.
// ---------------------------------------------------------------------------

// TestReturnToDraft_EmptyReason_PreservesRejectionReason is the LOAD-BEARING test for
// K-29: an empty reason must ⛔ NOT clear stateReason, so the author still reads why
// the MB was rejected while reworking it.
func TestReturnToDraft_EmptyReason_PreservesRejectionReason(t *testing.T) {
	e := newEntityAt(StatusSubmitted, false)
	require.NoError(t, e.Reject("formula does not match the sample"))

	require.NoError(t, e.ReturnToDraft(""))
	assert.Equal(t, StatusDraft, e.EntryStatus())
	assert.Equal(t, "formula does not match the sample", e.StateReason(),
		"K-29: an empty reason must preserve the original rejection reason")
}

// TestReturnToDraft_WithReason_OverwritesStateReason — a non-empty reason wins.
func TestReturnToDraft_WithReason_OverwritesStateReason(t *testing.T) {
	e := newEntityAt(StatusSubmitted, false)
	require.NoError(t, e.Reject("formula does not match the sample"))

	require.NoError(t, e.ReturnToDraft("alasan baru"))
	assert.Equal(t, StatusDraft, e.EntryStatus())
	assert.Equal(t, "alasan baru", e.StateReason())
}

// TestReturnToDraft_FromNonRejected_Fails — REJECTED is the ONLY legal origin.
func TestReturnToDraft_FromNonRejected_Fails(t *testing.T) {
	for _, status := range []string{
		StatusDraft, StatusSubmitted, StatusApproved,
		StatusValidated, StatusUnApproved, StatusRevoked,
	} {
		t.Run(status, func(t *testing.T) {
			e := newEntityAt(status, false)

			err := e.ReturnToDraft("rework please")
			assert.ErrorIs(t, err, ErrInvalidTransition)
			assert.Equal(t, status, e.EntryStatus(), "a failed ReturnToDraft must not mutate state")
			assert.Empty(t, e.StateReason(), "a failed ReturnToDraft must not write a reason")
		})
	}
}

// TestReturnToDraft_RecomputesCheckStatusCalc proves RecomputeCheckStatusCalc really runs:
// mbh_check_status_calc must leave 'Rejected' behind once the head is a DRAFT again.
func TestReturnToDraft_RecomputesCheckStatusCalc(t *testing.T) {
	e := newEntityAt(StatusSubmitted, false)
	require.NoError(t, e.Reject("wrong shade"))
	require.NotNil(t, e.MBHCheckStatusCalc())
	require.Equal(t, CheckStatusRejected, *e.MBHCheckStatusCalc())

	require.NoError(t, e.ReturnToDraft(""))
	require.NotNil(t, e.MBHCheckStatusCalc())
	assert.NotEqual(t, CheckStatusRejected, *e.MBHCheckStatusCalc(),
		"check_status_calc must not stay 'Rejected' after returning to DRAFT")
	assert.Equal(t, CheckStatusWaiting, *e.MBHCheckStatusCalc())
}

// ---------------------------------------------------------------------------
// Unrevoke — user decision 2026-08-31, mirroring K-29's ReturnToDraft above.
// ---------------------------------------------------------------------------

// TestUnrevoke_FromRevoked_Succeeds is the LOAD-BEARING happy path: REVOKED -> DRAFT.
func TestUnrevoke_FromRevoked_Succeeds(t *testing.T) {
	e := newEntityAt(StatusRevoked, false)

	require.NoError(t, e.Unrevoke("rework please"))
	assert.Equal(t, StatusDraft, e.EntryStatus())
	assert.Equal(t, "rework please", e.StateReason())
}

// TestUnrevoke_EmptyReason_PreservesPriorReason — an empty reason must NOT clear
// stateReason, so the trail of why it was revoked stays readable (principle U-2).
func TestUnrevoke_EmptyReason_PreservesPriorReason(t *testing.T) {
	e := newEntityAt(StatusRevoked, false)
	e.stateReason = "quality failed inspection"

	require.NoError(t, e.Unrevoke(""))
	assert.Equal(t, StatusDraft, e.EntryStatus())
	assert.Equal(t, "quality failed inspection", e.StateReason(),
		"an empty reason must preserve the original revoke reason")
}

// TestUnrevoke_WithReason_OverwritesStateReason — a non-empty reason wins.
func TestUnrevoke_WithReason_OverwritesStateReason(t *testing.T) {
	e := newEntityAt(StatusRevoked, false)
	e.stateReason = "quality failed inspection"

	require.NoError(t, e.Unrevoke("alasan baru"))
	assert.Equal(t, StatusDraft, e.EntryStatus())
	assert.Equal(t, "alasan baru", e.StateReason())
}

// TestUnrevoke_FromNonRevoked_Fails — REVOKED is the ONLY legal origin. This also
// guards the 2026-08-31 fix: Unrevoke must NOT be reachable from REJECTED or
// UNLOCK_REQUESTED, even though allowedTransitions routes both of those to DRAFT
// too (for ReturnToDraft and grant-unlock respectively).
func TestUnrevoke_FromNonRevoked_Fails(t *testing.T) {
	for _, status := range []string{
		StatusDraft, StatusSubmitted, StatusApproved,
		StatusValidated, StatusUnApproved, StatusRejected, StatusUnlockRequested,
	} {
		t.Run(status, func(t *testing.T) {
			e := newEntityAt(status, false)

			err := e.Unrevoke("rework please")
			assert.ErrorIs(t, err, ErrInvalidTransition)
			assert.Equal(t, status, e.EntryStatus(), "a failed Unrevoke must not mutate state")
			assert.Empty(t, e.StateReason(), "a failed Unrevoke must not write a reason")
		})
	}
}

// TestReturnToDraft_FromRevoked_Fails guards the 2026-08-31 fix directly: before it,
// ReturnToDraft used the generic canTransition(from, StatusDraft) check, which also
// permitted REVOKED once that edge was added for Unrevoke — silently letting a holder
// of finance.mb.head.submit (ReturnToDraft's permission) unrevoke a REVOKED row
// without ever holding finance.mb.head.unrevoke (Unrevoke's dedicated, Super-Admin-only
// permission). ReturnToDraft must stay REJECTED-only.
func TestReturnToDraft_FromRevoked_Fails(t *testing.T) {
	e := newEntityAt(StatusRevoked, false)

	err := e.ReturnToDraft("rework please")
	assert.ErrorIs(t, err, ErrInvalidTransition)
	assert.Equal(t, StatusRevoked, e.EntryStatus())
	assert.Empty(t, e.StateReason())
}

// TestUnrevoke_RecomputesCheckStatusCalc proves RecomputeCheckStatusCalc really runs
// on Unrevoke too, mirroring TestReturnToDraft_RecomputesCheckStatusCalc above.
func TestUnrevoke_RecomputesCheckStatusCalc(t *testing.T) {
	e := newEntityAt(StatusRevoked, false)

	require.NoError(t, e.Unrevoke(""))
	require.NotNil(t, e.MBHCheckStatusCalc())
	assert.Equal(t, CheckStatusWaiting, *e.MBHCheckStatusCalc())
}

// TestCanForceUnvalidate_OnlyValidatedQualifies table-drives canForceUnvalidate across
// every known status: only StatusValidated must report true (Bulk MB Head Regenerate,
// Phase B/G) — this predicate is deliberately kept OUT of allowedTransitions (see the
// doc comment on canForceUnvalidate), so it needs its own direct coverage rather than
// relying on the generic canTransition table-driven tests above.
func TestCanForceUnvalidate_OnlyValidatedQualifies(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{StatusDraft, false},
		{StatusSubmitted, false},
		{StatusApproved, false},
		{StatusValidated, true},
		{StatusUnApproved, false},
		{StatusRevoked, false},
		{StatusRejected, false},
		{StatusUnlockRequested, false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, canForceUnvalidate(tt.status))
		})
	}
}

// TestForceUnvalidate_FromValidated_Succeeds proves the happy path: VALIDATED -> DRAFT,
// the lock is cleared unconditionally, and a supplied reason is stored.
func TestForceUnvalidate_FromValidated_Succeeds(t *testing.T) {
	e := newEntityAt(StatusValidated, false)
	e.isLocked = true
	requestedAt := time.Now()
	requestedBy := "someone"
	e.unlockRequestedAt = &requestedAt
	e.unlockRequestedBy = &requestedBy

	require.NoError(t, e.ForceUnvalidate("bulk regenerate"))
	assert.Equal(t, StatusDraft, e.EntryStatus())
	assert.False(t, e.IsLocked(), "ForceUnvalidate must clear the lock unconditionally")
	assert.Nil(t, e.UnlockRequestedAt(), "ForceUnvalidate must clear any pending unlock request")
	assert.Nil(t, e.UnlockRequestedBy())
	assert.Equal(t, "bulk regenerate", e.StateReason())
}

// TestForceUnvalidate_EmptyReason_PreservesPriorReason mirrors Unrevoke/ReturnToDraft:
// an empty reason must NOT clear stateReason (principle U-2).
func TestForceUnvalidate_EmptyReason_PreservesPriorReason(t *testing.T) {
	e := newEntityAt(StatusValidated, false)
	e.stateReason = "validated by QA"

	require.NoError(t, e.ForceUnvalidate(""))
	assert.Equal(t, StatusDraft, e.EntryStatus())
	assert.Equal(t, "validated by QA", e.StateReason(),
		"an empty reason must preserve the prior stateReason")
}

// TestForceUnvalidate_ClearsLock_EvenWithoutReason proves the lock/unlock-request
// clearing is unconditional and independent of whether a reason was supplied.
func TestForceUnvalidate_ClearsLock_EvenWithoutReason(t *testing.T) {
	e := newEntityAt(StatusValidated, false)
	e.isLocked = true

	require.NoError(t, e.ForceUnvalidate(""))
	assert.False(t, e.IsLocked())
}

// TestForceUnvalidate_FromNonValidated_Fails proves VALIDATED is the ONLY legal
// origin: every other status must be refused with ErrInvalidTransition and must not
// mutate any state.
func TestForceUnvalidate_FromNonValidated_Fails(t *testing.T) {
	for _, status := range []string{
		StatusDraft, StatusSubmitted, StatusApproved,
		StatusUnApproved, StatusRevoked, StatusRejected, StatusUnlockRequested,
	} {
		t.Run(status, func(t *testing.T) {
			e := newEntityAt(status, false)
			e.isLocked = true

			err := e.ForceUnvalidate("bulk regenerate")
			assert.ErrorIs(t, err, ErrInvalidTransition)
			assert.Equal(t, status, e.EntryStatus(), "a failed ForceUnvalidate must not mutate state")
			assert.True(t, e.IsLocked(), "a failed ForceUnvalidate must not touch the lock")
			assert.Empty(t, e.StateReason(), "a failed ForceUnvalidate must not write a reason")
		})
	}
}

// TestForceUnvalidate_RecomputesCheckStatusCalc mirrors TestUnrevoke_RecomputesCheckStatusCalc.
func TestForceUnvalidate_RecomputesCheckStatusCalc(t *testing.T) {
	e := newEntityAt(StatusValidated, false)

	require.NoError(t, e.ForceUnvalidate(""))
	require.NotNil(t, e.MBHCheckStatusCalc())
	assert.Equal(t, CheckStatusWaiting, *e.MBHCheckStatusCalc())
}
