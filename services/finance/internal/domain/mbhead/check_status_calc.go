package mbhead

// Derived check-status values. Title Case EXACTLY as it appears in production —
// ⛔ lower case would be rejected by chk_mbh_check_status_calc (migration 000487).
const (
	// CheckStatusWaiting — the recipe is still awaiting approval (DRAFT/SUBMITTED).
	CheckStatusWaiting = "Waiting"
	// CheckStatusApproved — the recipe has been approved.
	CheckStatusApproved = "Approved"
	// CheckStatusBoughtout — the MB is a bought-out item, so it never runs the
	// approval chain the same way. Wins over every other rule.
	CheckStatusBoughtout = "Boughtout"
	// CheckStatusCurrent — ⛔ NOT PRODUCED by DeriveCheckStatus today. Reserved by
	// the CHECK constraint so a later phase needs no new migration. See below.
	CheckStatusCurrent = "Current"
	// CheckStatusOutdated — ⛔ NOT PRODUCED today (90-day threshold undecided).
	CheckStatusOutdated = "Outdated"
	// CheckStatusRejected — produced for the REJECTED workflow state (user decision
	// K-28, executed 2026-08-23). ⛔ Still outranked by Boughtout.
	CheckStatusRejected = "Rejected"
)

// DeriveCheckStatus computes the value of mst_mb_head.mbh_check_status_calc from
// the workflow state and the bought-out flag. It is a PURE function: same input,
// same output, no clock, no I/O.
//
// 🔴 IT NEVER TOUCHES mbh_check_status. That column is FROZEN as the Oracle import
// trace (user decision K-1, option 2). The derived value lives in a SEPARATE column.
//
// 🔴 A nil return means "UNDECIDED — do not write anything". It is deliberately NOT
// the same as writing NULL: callers MUST leave the stored value untouched, so a
// previously computed value is never silently erased by a state this engine has no
// rule for yet. Three states return nil today, and each is a recorded user gate,
// ⛔ not an oversight:
//
//   - VALIDATED    → would map to "Current" (design §c rule 6), but that mapping is
//     HYPOTHESIS H1 and the design says explicitly: do not implement before V-A1 is
//     run. 477 production rows already carry "Current" in the legacy column and its
//     provenance is unknown.
//   - UN_APPROVED  → design §c rule 8 offers "Waiting" as a GUESS. ⛔ Not decided.
//   - REVOKED      → design §c rule 8 offers "Rejected" as a GUESS. ⛔ Not decided.
//
// "Outdated" is unreachable for a further reason: it needs a time-based threshold
// (90 days is a PROPOSAL, not a decision) plus a cron job, which makes it non-pure
// and therefore out of this function's scope entirely. "Rejected" IS produced now —
// the REJECTED workflow state was added by user decisions K-2/K-28 (2026-08-23),
// closing gate G12-REJECT.
func DeriveCheckStatus(entryStatus string, isBoughtout bool) *string {
	// Rule 1 — Boughtout outranks everything. It describes the NATURE of the item
	// (purchased, not produced) rather than a stage of a process, and the entity
	// already treats bought-out heads as skipping the approval chain.
	if isBoughtout {
		v := CheckStatusBoughtout
		return &v
	}
	switch entryStatus {
	case StatusApproved:
		v := CheckStatusApproved
		return &v
	case StatusDraft, StatusSubmitted:
		v := CheckStatusWaiting
		return &v
	case StatusRejected:
		v := CheckStatusRejected
		return &v
	default:
		// StatusValidated, StatusUnApproved, StatusRevoked and any unknown value.
		return nil
	}
}
