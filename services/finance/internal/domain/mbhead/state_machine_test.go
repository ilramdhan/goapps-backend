package mbhead

import (
	"testing"

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
