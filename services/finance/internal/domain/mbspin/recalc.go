// Package mbspin provides domain logic for Melange Batch Spin (child of MB Head) management.
package mbspin

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Skip reasons reported for a child spin that the recalc pass deliberately left
// alone (rule A7).
//
// ⚠ These two strings are the WHOLE vocabulary — user decision K-46(a) fixed the
// proto field `MBSpinRecalcSkipped.reason` as a plain string carrying exactly
// STATUS_NOT_RND or STATUS_ABSENT. ⛔ Do not invent a third code without a new
// user decision.
const (
	// SkipReasonStatusNotRnD marks a child whose mbs_status is present but is not
	// "R and D" (Spinning / Boughtout / anything else). It carries an ACTUAL
	// production value that must never be overwritten by a derived one.
	SkipReasonStatusNotRnD = "STATUS_NOT_RND"
	// SkipReasonStatusAbsent marks a child with no mbs_status at all. An absent
	// status is itself a skip reason: it is treated as non-recalculable rather
	// than optimistically assumed to be R&D.
	SkipReasonStatusAbsent = "STATUS_ABSENT"
)

// SkipReasonFor returns the A7 skip reason for a child spin that is NOT a recalc
// candidate. It is only meaningful for non-candidates: an R&D child is a
// candidate and is never skipped for a status reason.
//
// Pure function, no I/O — the status vocabulary lives here so exactly one place
// decides what "actual" means.
func SkipReasonFor(status *string) string {
	if status == nil || *status == "" {
		return SkipReasonStatusAbsent
	}
	return SkipReasonStatusNotRnD
}

// ChildRecalcUpdate is one child spin whose dozing the recalc pass recomputed.
//
// It carries ONLY the dozing value: the recalc chain stops at the child spin
// (decision D24). ⛔ Nothing downstream of the spin — no yarn product, no
// cost_product_cost row — is recalculated, so no product identifier appears here.
type ChildRecalcUpdate struct {
	// SpinID is the child spin being written.
	SpinID uuid.UUID
	// NewDozing is the value produced by mbdozing.ScaleLDR (formula C-1).
	NewDozing float64
}

// RecalcApplyInput is the payload of one recalc OPERATION.
//
// One call == one operation == ONE mst_mb_workflow_log row (plan §P8 "Jejak"),
// ⛔ never one row per child, however many children Updates carries.
type RecalcApplyInput struct {
	// ParentSpinID is the spin whose change triggered the pass. Recorded for the
	// audit row; the parent's own dozing is written by the normal Update path.
	ParentSpinID uuid.UUID
	// HeadID is mbs_mbh_id of the parent — the mbwl_mbh_id of the audit row.
	// Required: mst_mb_workflow_log.mbwl_mbh_id is NOT NULL with an FK to
	// mst_mb_head.
	HeadID uuid.UUID
	// Actor lands in mbs_last_recalc_by / updated_by / mbwl_actor_user_id.
	Actor string
	// At is the single timestamp stamped on every touched row, so one operation
	// is identifiable by one instant.
	At time.Time
	// Updates are the children to write. May be empty: an operation that
	// recalculated nothing still writes its audit row, because "nothing was
	// eligible" is itself the fact worth keeping.
	Updates []ChildRecalcUpdate
	// LogReason is the human-readable mbwl_reason text.
	LogReason string
	// LogMeta is the mbwl_meta JSONB document, already marshaled. Empty means
	// the column stays NULL.
	LogMeta string
}

// RecalcRepository is the WRITE side of the child-recalc pass, kept separate
// from Repository on purpose: widening Repository would force every existing
// test double to grow methods it never exercises, and the read/classify half of
// the pass already has what it needs there (ListChildren).
//
// ⛔ ISOLATION: no method here may reach the calc-engine v2 (rmcost). The chain
// stops at the child spin (D24).
type RecalcRepository interface {
	// ListAllChildren returns EVERY non-deleted direct child of a parent spin,
	// regardless of status — the superset of Repository.ListChildren.
	//
	// It exists so the caller can REPORT the non-candidates (A7) it skipped;
	// ListChildren alone cannot, because it filters them out. ⛔ Still one level
	// deep: like ListChildren it never recurses, so grandchildren are absent by
	// construction (R13).
	ListAllChildren(ctx context.Context, parentID uuid.UUID) ([]*Entity, error)

	// ApplyChildRecalc writes one recalc operation transactionally: the new
	// dozing plus mbs_last_recalc_at/by on each child in Updates, and exactly ONE
	// mst_mb_workflow_log row for the whole operation.
	ApplyChildRecalc(ctx context.Context, in RecalcApplyInput) error
}
