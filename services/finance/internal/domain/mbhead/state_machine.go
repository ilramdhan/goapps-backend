package mbhead

// Workflow status constants for an MB head record.
const (
	StatusDraft      = "DRAFT"
	StatusSubmitted  = "SUBMITTED"
	StatusApproved   = "APPROVED"
	StatusValidated  = "VALIDATED"
	StatusUnApproved = "UN_APPROVED"
	StatusRevoked    = "REVOKED"
	StatusRejected   = "REJECTED"
	// StatusUnlockRequested is the P10 lock/unlock holding state (migration 000492).
	// A recipe that is locked (APPROVED or VALIDATED) parks here while an unlock
	// request awaits a decision. ⛔ It is NOT reachable from DRAFT — an editable
	// recipe has nothing to unlock.
	StatusUnlockRequested = "UNLOCK_REQUESTED"
)

var allowedTransitions = map[string]map[string]struct{}{
	StatusDraft: {StatusSubmitted: {}, StatusValidated: {}}, // Validated only via boughtout shortcut
	// 🔴 USER DECISION 2026-08-26 — "OPSI A". ~~{StatusApproved, StatusRejected}~~ was
	// WIDENED with StatusValidated. The user simplified the MB Recipe workflow to
	// DRAFT → SUBMITTED → APPROVED and asked for the Validate BUTTON to disappear, with
	// the MB product auto-generation moved onto Approve.
	//
	// ⚠ Validate could NOT simply be deleted. mb_head_repository.ListValidated() filters
	// WHERE mbh_entry_status = 'VALIDATED', and TWO engines read it: MB Push to Head
	// (application/mbpush) and Trigger MB Batch (application/mbbatch/dag.go). Stopping the
	// workflow at APPROVED would leave both permanently empty and kill the whole MB cost calculation.
	//
	// ⇒ Pressing Approve therefore lands the recipe DIRECTLY in VALIDATED (the gRPC
	// ApproveMBHead RPC drives ValidateHandler), which is why this edge exists. VALIDATED
	// stays alive in the database and ListValidated keeps finding rows.
	//
	// ⛔ StatusRejected stays: Reject is still offered from SUBMITTED.
	StatusSubmitted: {StatusApproved: {}, StatusRejected: {}, StatusValidated: {}},
	// P10: a locked APPROVED/VALIDATED head can be parked in UNLOCK_REQUESTED.
	//
	// 🔴 USER DECISION 2026-08-26 — the ~~APPROVED → UN_APPROVED~~ edge is GONE.
	// Un-approve was removed as a feature: the simplified workflow is
	// DRAFT → SUBMITTED → APPROVED (locked), and from SUBMITTED the only choices
	// are Approve and Reject. ⛔ Nothing may ENTER UN_APPROVED any more.
	// ⛔ 2026-08-26 (Opsi A) — the APPROVED → VALIDATED edge is deliberately KEPT even
	// though the normal path no longer stops at APPROVED. Production already holds legacy
	// rows parked in APPROVED, and the ValidateMBHead RPC (kept alive, only its BUTTON was
	// removed from the UI) is how they still move forward.
	StatusApproved:  {StatusValidated: {}, StatusUnlockRequested: {}},
	StatusValidated: {StatusUnlockRequested: {}},
	// 🔴 2026-08-26 — this EXIT edge is deliberately KEPT even though nothing can
	// enter UN_APPROVED any more. Production already holds rows parked in
	// UN_APPROVED from before the decision; removing the row would strand them with
	// no legal move at all. Reading and rescuing legacy rows stays possible; only
	// the entrance was closed.
	StatusUnApproved: {StatusApproved: {}},
	// 🔴 2026-08-31 — Unrevoke: an exit-only edge mirroring StatusUnApproved above.
	// Revoke itself stays removed as a feature (canRevoke always reports false), so
	// nothing may ENTER REVOKED any more, but legacy/newly-revoked rows must have a
	// legal way out. Super Admin (gated by permission finance.mb.head.unrevoke) can
	// send a REVOKED row back to DRAFT for editing and resubmission.
	StatusRevoked:  {StatusDraft: {}},
	StatusRejected: {StatusDraft: {}}, // Rejected work goes back to the author as a Draft
	// P10 exits: reject-unlock returns the head to whichever locked state it came
	// from (APPROVED or VALIDATED), grant-unlock opens it for editing as DRAFT.
	StatusUnlockRequested: {StatusApproved: {}, StatusValidated: {}, StatusDraft: {}},
}

func canTransition(from, to string) bool {
	if from == to {
		return false
	}
	// ~~Revoke is legal from any non-terminal state, checked separately by callers.~~
	// 🔴 USER DECISION 2026-08-26 — Revoke was removed as a feature; canRevoke now
	// always reports false. See its doc comment below.
	targets, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, ok = targets[to]
	return ok
}

func isTerminal(status string) bool {
	return status == StatusRevoked
}

// canRevoke reports whether `from` may be revoked. ~~Legal from every non-terminal
// state.~~
//
// 🔴 USER DECISION 2026-08-26 — Revoke was REMOVED as a feature and this predicate
// now ALWAYS returns false. The user's reasoning: activating or de-activating a
// recipe is an admin concern handled by the is_active flag, so a terminal REVOKED
// status is not needed.
//
// ⛔ The StatusRevoked constant and isTerminal() are deliberately NOT deleted:
// production already holds rows sitting in REVOKED, and dropping the constant would
// make those rows unreadable. What was switched off is the ability to ENTER REVOKED,
// never the ability to read a row that is already there.
//
// The parameter is kept so the signature still documents the question being asked.
func canRevoke(_ string) bool {
	return false
}

// canForceUnvalidate reports whether `from` may be force-unvalidated (Bulk MB Head
// Regenerate, Phase B, Super-Admin only). Only VALIDATED qualifies.
//
// 🔴 This is a SEPARATE gate, deliberately ⛔ NOT added to allowedTransitions. Adding
// StatusValidated → StatusDraft there would also legalize it for the normal
// single-item flow, which must stay the 2-step RequestUnlock/GrantUnlock dance — see
// state_machine_freeze_test.go for the frozen matrix this must not disturb. The bulk
// force path is a distinct, separately-gated operation (finance.mb.head.bulkunvalidate)
// and gets its own predicate, mirroring how canAutoRelock stays outside the map too.
func canForceUnvalidate(from string) bool {
	return from == StatusValidated
}

// canRequestUnlock reports whether an unlock may be requested from `from`. Only the
// two states that lockOnEnter locks (APPROVED, VALIDATED) qualify — anything else
// is either already editable or not a lockable state at all.
func canRequestUnlock(from string) bool {
	return from == StatusApproved || from == StatusValidated
}

// canGrantUnlock reports whether a pending unlock request may be granted or rejected.
// Only UNLOCK_REQUESTED qualifies: without a parked request there is nothing to decide.
func canGrantUnlock(from string) bool {
	return from == StatusUnlockRequested
}

// lockOnEnter reports whether entering `state` must lock the recipe (P10). APPROVED
// and VALIDATED are the two states whose recipe content is considered settled;
// editing them again requires an unlock decision first.
func lockOnEnter(state string) bool {
	return state == StatusApproved || state == StatusValidated
}

// canAutoRelock reports whether an expired unlock window may be closed by moving
// `from` back to `to` (user decision K-57 option (a)): only DRAFT → APPROVED and
// DRAFT → VALIDATED qualify.
//
// 🔴 This is a SEPARATE gate, deliberately ⛔ NOT part of allowedTransitions and
// ⛔ NOT consulted by canTransition. Widening allowedTransitions[StatusDraft] with
// APPROVED would also legalize approving a DRAFT head directly, skipping SUBMITTED
// entirely — that breaks the workflow and would break the frozen matrix in
// state_machine_freeze_test.go. The auto-relock path is not a user-initiated
// transition; it is the reversal of one, and it gets its own predicate so the
// user-facing matrix stays exactly as it is.
func canAutoRelock(from, to string) bool {
	return from == StatusDraft && lockOnEnter(to)
}
