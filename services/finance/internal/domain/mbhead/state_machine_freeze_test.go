package mbhead

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkflow_LegalTransitionMatrix_Frozen is the K11 regression guard for the
// MB recipe work (plan §5 P5).
//
// 🔴 Its purpose is to FREEZE state_machine.go, ⛔ not to change it. This table
// restates every transition the state map allows TODAY, so any later edit that
// quietly widens or narrows the workflow fails here.
//
// 🔴 UPDATED 2026-08-23 — K-2/K-3 are no longer blocked; they were EXECUTED:
//   - K-2: StatusRejected now exists. New edges SUBMITTED → REJECTED (reason
//     mandatory) and REJECTED → DRAFT.
//   - K-3: the dead edge SUBMITTED → DRAFT was REMOVED — rejection now goes
//     through REJECTED, not straight back to DRAFT.
//   - K-24: REJECTED is ⛔ NOT terminal — it can still be Revoked. Only REVOKED
//     is terminal.
//
// 🔴 UPDATED 2026-08-25 (P10) — StatusUnlockRequested EXISTS now, and with it three
// new edges plus one widened row:
//   - APPROVED  → UNLOCK_REQUESTED (new)
//   - VALIDATED → UNLOCK_REQUESTED (new; VALIDATED was a dead end before)
//   - UNLOCK_REQUESTED → {APPROVED, VALIDATED, DRAFT} (new row)
//
// ~~⛔ Nothing was REMOVED. Every edge frozen by the 2026-08-23 baseline is still
// listed below unchanged; REVOKED is still the only terminal state.~~
//
// 🔴 UPDATED 2026-08-26 (USER DECISION) — this is the FIRST narrowing of the matrix,
// and it was deliberate, ⛔ not drift. The user simplified the MB Recipe workflow to
// DRAFT (editable) → SUBMITTED (not editable) → APPROVED (locked); from SUBMITTED the
// only actions are Approve and Reject, and a locked recipe offers only Request Unlock.
// Two features were removed outright:
//   - Un-approve — the edge APPROVED → UN_APPROVED is GONE. A locked recipe is
//     reopened through Request Unlock instead.
//   - Revoke — canRevoke now ALWAYS returns false, so ⛔ nothing can enter REVOKED.
//     The user's reasoning: activating/deactivating a recipe is an admin concern served
//     by the active flag, so a terminal REVOKED status is not needed.
//
// ⛔ What was ⛔ NOT removed, and must not be: the StatusUnApproved and StatusRevoked
// CONSTANTS, the isTerminal() rule, and the exit edge UN_APPROVED → APPROVED.
// Production already holds rows parked in those two statuses; deleting the constants
// would make them unreadable, and deleting the exit edge would strand the UN_APPROVED
// ones with no legal move. Only the ENTRANCES were closed.
//
// 🔴 UPDATED 2026-08-26 (USER DECISION, "Opsi A") — a SECOND deliberate change on the
// same day, and this one WIDENS one row:
//   - SUBMITTED → VALIDATED (new). The user removed the Validate BUTTON and moved the MB
//     product auto-generation onto Approve, so pressing Approve must land the recipe
//     directly in VALIDATED. The gRPC ApproveMBHead RPC drives ValidateHandler to do it.
//
// ⚠ Why VALIDATED could not simply be dropped from the workflow:
// mb_head_repository.ListValidated() filters WHERE mbh_entry_status = 'VALIDATED', and
// TWO engines read it — MB Push to Head (application/mbpush) and Trigger MB Batch
// (application/mbbatch/dag.go). A workflow ending at APPROVED would leave both empty
// forever and stop MB costing. VALIDATED therefore stays a live status; only the button
// that used to reach it is gone.
//
// ⛔ NOTHING was removed by this change, and specifically ⛔ NOT SUBMITTED → APPROVED and
// ⛔ NOT APPROVED → VALIDATED. Legacy production rows sit in APPROVED, the ValidateMBHead
// RPC is still alive to move them on, and cmd/backfill-mb-validate still walks rows
// through APPROVED on purpose.
//
// The matrix below is the NEW frozen baseline for the 2026-08-26 workflow.
//
// Read as: from -> the complete set of legal targets. Anything not listed is
// illegal.
func TestWorkflow_LegalTransitionMatrix_Frozen(t *testing.T) {
	all := []string{
		StatusDraft, StatusSubmitted, StatusApproved,
		StatusValidated, StatusUnApproved, StatusRevoked, StatusRejected,
		StatusUnlockRequested,
	}

	legal := map[string]map[string]bool{
		StatusDraft: {StatusSubmitted: true, StatusValidated: true},
		// Opsi A 2026-08-26: ~~{StatusApproved, StatusRejected}~~ WIDENED with
		// StatusValidated — Approve now lands the recipe straight in VALIDATED.
		StatusSubmitted: {StatusApproved: true, StatusRejected: true, StatusValidated: true},
		// 2026-08-26: ~~StatusUnApproved: true~~ removed — Un-approve is gone.
		// Kept on purpose (Opsi A): legacy rows already in APPROVED still need this exit.
		StatusApproved:  {StatusValidated: true, StatusUnlockRequested: true},
		StatusValidated: {StatusUnlockRequested: true},
		// Kept on purpose: the EXIT from UN_APPROVED still works for legacy rows even
		// though nothing can enter that state any more.
		StatusUnApproved: {StatusApproved: true},
		StatusRevoked:    {},
		StatusRejected:   {StatusDraft: true},
		StatusUnlockRequested: {
			StatusApproved: true, StatusValidated: true, StatusDraft: true,
		},
	}

	for _, from := range all {
		for _, to := range all {
			want := legal[from][to]
			assert.Equalf(t, want, canTransition(from, to),
				"transition %s -> %s changed; ⛔ state_machine.go must stay frozen", from, to)
		}
	}
}

// TestWorkflow_RevokeAndTerminality_Frozen pins the Revoke rule, which lives
// outside allowedTransitions. ~~Legal from every non-terminal state, and terminal
// once reached. 🔴 REJECTED is in the non-terminal list on purpose (K-24): a
// rejected head may still be revoked.~~
//
// 🔴 REWRITTEN 2026-08-26 (USER DECISION) — Revoke was REMOVED as a feature, so the
// rule this test freezes has INVERTED: canRevoke must now report false from EVERY
// state, with no exception. ⛔ The test was not deleted and not skipped — it still
// guards the same predicate, it just pins the opposite answer, so a future edit that
// quietly re-opens the way into REVOKED fails right here.
//
// isTerminal is asserted SEPARATELY and is deliberately UNCHANGED: REVOKED is still
// the one terminal status, because production rows already sit in it.
func TestWorkflow_RevokeAndTerminality_Frozen(t *testing.T) {
	for _, from := range []string{
		StatusDraft, StatusSubmitted, StatusApproved,
		StatusValidated, StatusUnApproved, StatusRejected,
		StatusUnlockRequested, StatusRevoked,
	} {
		assert.Falsef(t, canRevoke(from),
			"2026-08-26: revoke was removed; it must stay illegal from %s", from)
	}

	// Terminality itself is untouched — only the way INTO REVOKED was closed.
	for _, from := range []string{
		StatusDraft, StatusSubmitted, StatusApproved,
		StatusValidated, StatusUnApproved, StatusRejected, StatusUnlockRequested,
	} {
		assert.Falsef(t, isTerminal(from), "%s must not be terminal", from)
	}
	assert.True(t, isTerminal(StatusRevoked), "REVOKED must stay terminal for legacy rows")
}

// TestWorkflow_RemovedEdges_StayClosed_2026_08_26 is the positive statement of the
// user decision: the two edges that were switched off must stay off. It is separate
// from the matrix test on purpose — the matrix says "these are the legal edges", this
// one says "and these two specifically must never come back".
func TestWorkflow_RemovedEdges_StayClosed_2026_08_26(t *testing.T) {
	assert.False(t, canTransition(StatusApproved, StatusUnApproved),
		"Un-approve was removed 2026-08-26; APPROVED → UN_APPROVED must stay closed")

	for _, from := range []string{
		StatusDraft, StatusSubmitted, StatusApproved, StatusValidated,
		StatusUnApproved, StatusRejected, StatusUnlockRequested,
	} {
		assert.Falsef(t, canRevoke(from),
			"Revoke was removed 2026-08-26; %s must not be revocable", from)
	}

	// The legacy ESCAPE hatch stays open — this is the half that must ⛔ NOT be
	// "tidied away" together with the removed entrance.
	assert.True(t, canTransition(StatusUnApproved, StatusApproved),
		"legacy UN_APPROVED rows must keep their way out")
}

// TestWorkflow_OpsiA_ApproveLandsInValidated_2026_08_26 pins the CORE of the user's
// "Opsi A" decision, stated as its own test rather than left implicit in the matrix.
//
// The decision: the Validate button is gone from the screen and MB product generation
// moved onto Approve, so Approve must carry the recipe all the way to VALIDATED.
//
// 🔴 The second assertion is the one that protects the money. ListValidated() selects
// WHERE mbh_entry_status = 'VALIDATED' and feeds MB Push to Head AND Trigger MB Batch. If
// a future edit made Approve stop at APPROVED, both engines would silently see zero
// candidates. This test therefore checks the entity's own status is exactly the string
// those queries look for — ⛔ deliberately WITHOUT touching a database.
func TestWorkflow_OpsiA_ApproveLandsInValidated_2026_08_26(t *testing.T) {
	// The state map must allow the move at all.
	assert.True(t, canTransition(StatusSubmitted, StatusValidated),
		"Opsi A: Approve drives SUBMITTED → VALIDATED; this edge must stay open")

	for _, boughtout := range []bool{false, true} {
		e := newEntityAt(StatusDraft, boughtout)
		require.NoError(t, e.Submit())
		require.Equal(t, StatusSubmitted, e.EntryStatus())

		// What pressing Approve does under Opsi A: the RPC drives ValidateHandler, whose
		// domain call is Validate().
		require.NoError(t, e.Validate(), "boughtout=%v must be able to reach VALIDATED via Submit → Approve", boughtout)

		assert.Equal(t, StatusValidated, e.EntryStatus(),
			"boughtout=%v: Approve must land in VALIDATED, not stop at APPROVED", boughtout)
		// The exact string ListValidated() filters on. If this ever drifts, MB Push to
		// Head and Trigger MB Batch both go blank.
		assert.Equal(t, "VALIDATED", e.EntryStatus(),
			"ListValidated() filters on the literal 'VALIDATED'; a recipe Approve produced must match it")
		assert.True(t, e.IsLocked(), "VALIDATED is a lockOnEnter state; the entity must report it as locked")
		assert.Equal(t, int32(1), e.CurrentVersion(), "validating bumps the version exactly once")
	}
}

// TestWorkflow_OpsiA_LegacyApprovedPathSurvives is the other half of the decision: rows
// that are ALREADY in APPROVED (production has them, and cmd/backfill-mb-validate still
// produces them) must keep their way to VALIDATED through the surviving ValidateMBHead
// RPC. ⛔ Widening SUBMITTED must not have been paid for by narrowing APPROVED.
func TestWorkflow_OpsiA_LegacyApprovedPathSurvives(t *testing.T) {
	assert.True(t, canTransition(StatusApproved, StatusValidated),
		"legacy APPROVED rows must keep their way to VALIDATED")

	e := newEntityAt(StatusApproved, false)
	require.NoError(t, e.Validate())
	assert.Equal(t, StatusValidated, e.EntryStatus())
}

// TestWorkflow_LockOnEnter_EntityAgreesWithPersistence pins the 2026-08-26 fix to the
// long-standing defect where Approve() and Validate() moved the status but left the
// in-memory isLocked flag false — so the gRPC response, built from that entity, reported
// mbhIsLocked=false about a row the SQL layer had just locked.
//
// 🔴 It asserts the entity against lockOnEnter, the SAME predicate DeriveLockEffect
// consults to decide the SQL clause. Agreeing with that predicate is exactly what "the
// entity and the persisted column cannot disagree" means, and it is why the fix cannot
// double-write or contradict the SQL: the entity does not produce the SQL at all.
func TestWorkflow_LockOnEnter_EntityAgreesWithPersistence(t *testing.T) {
	// Approve → APPROVED (lockOnEnter true).
	a := newEntityAt(StatusSubmitted, false)
	require.NoError(t, a.Approve())
	assert.True(t, lockOnEnter(a.EntryStatus()))
	assert.True(t, a.IsLocked(), "entering APPROVED must lock the entity, matching DeriveLockEffect")
	assert.False(t, a.IsEditable())
	assert.NotNil(t, a.LockedAt())
	// ⛔ No actor is invented in the domain: mbh_locked_by is written by lockClauses from
	// the transition's real actorUserID.
	assert.Nil(t, a.LockedBy(), "the domain must not invent a locking actor")

	// Validate → VALIDATED (lockOnEnter true).
	v := newEntityAt(StatusSubmitted, false)
	require.NoError(t, v.Validate())
	assert.True(t, lockOnEnter(v.EntryStatus()))
	assert.True(t, v.IsLocked(), "entering VALIDATED must lock the entity, matching DeriveLockEffect")

	// The negative case: Submit enters SUBMITTED, which lockOnEnter reports false for, so
	// the entity must stay unlocked. ⛔ The fix must not lock on every transition.
	s := newEntityAt(StatusDraft, false)
	require.NoError(t, s.Submit())
	assert.False(t, lockOnEnter(s.EntryStatus()))
	assert.False(t, s.IsLocked(), "SUBMITTED is not a lockOnEnter state; it must not lock")
	assert.True(t, s.IsEditable())
}

// TestWorkflow_TransitionMethods_StillWork walks the entity-level methods so the
// matrix above cannot drift from the behavior callers actually see.
// 🔴 REWRITTEN 2026-08-26 — the old body walked DRAFT → SUBMITTED → APPROVED →
// UN_APPROVED → APPROVED → VALIDATED and then revoked a head to prove terminality.
// Both of those steps used features the user removed, so the walk now follows the
// simplified workflow instead, and the removed methods are asserted to REFUSE.
func TestWorkflow_TransitionMethods_StillWork(t *testing.T) {
	// The workflow the user kept: DRAFT → SUBMITTED → APPROVED → VALIDATED.
	e := newEntityAt(StatusDraft, false)
	require.NoError(t, e.Submit())
	require.NoError(t, e.Approve())
	assert.Equal(t, StatusApproved, e.EntryStatus())
	require.NoError(t, e.Validate())
	assert.Equal(t, StatusValidated, e.EntryStatus())

	// ~~Reason is mandatory on both reason-bearing transitions.~~
	// 2026-08-26: UnApprove and Revoke were removed. They now refuse from EVERY state
	// and with EVERY argument — including a non-empty reason, which used to succeed.
	// ⛔ ErrInvalidTransition, ⛔ never ErrReasonRequired: the argument no longer gets
	// as far as being examined.
	for _, status := range []string{
		StatusDraft, StatusSubmitted, StatusApproved, StatusValidated,
		StatusUnApproved, StatusRejected, StatusRevoked, StatusUnlockRequested,
	} {
		u := newEntityAt(status, false)
		assert.ErrorIs(t, u.UnApprove("wrong recipe"), ErrInvalidTransition, "UnApprove from %s", status)
		assert.ErrorIs(t, u.UnApprove(""), ErrInvalidTransition, "UnApprove from %s", status)
		assert.Equal(t, status, u.EntryStatus(), "a refused UnApprove must not mutate state")

		r := newEntityAt(status, false)
		assert.ErrorIs(t, r.Revoke("cancelled"), ErrInvalidTransition, "Revoke from %s", status)
		assert.ErrorIs(t, r.Revoke(""), ErrInvalidTransition, "Revoke from %s", status)
		assert.Equal(t, status, r.EntryStatus(), "a refused Revoke must not mutate state")
	}

	// Terminality of a PRE-EXISTING revoked row is unchanged — such rows exist in
	// production and must stay frozen even though nothing new can reach that state.
	r := newEntityAt(StatusRevoked, false)
	assert.ErrorIs(t, r.Submit(), ErrInvalidTransition)
	assert.ErrorIs(t, r.Approve(), ErrInvalidTransition)
	assert.ErrorIs(t, r.Validate(), ErrInvalidTransition)
}
