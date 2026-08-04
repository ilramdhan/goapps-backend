package workorder

import (
	"errors"
	"fmt"
)

// Domain errors for work-order operations.
var (
	// ErrNotFound is returned when a work order is not found.
	ErrNotFound = errors.New("work order not found")
	// ErrInvalidArea is returned when the area code is not TXT/SPG/TWT.
	ErrInvalidArea = errors.New("invalid area: must be TXT, SPG, or TWT")
	// ErrInvalidQty is returned when the target quantity is not positive.
	ErrInvalidQty = errors.New("invalid quantity: must be greater than zero")
	// ErrInvalidDeadline is returned when the deadline is empty.
	ErrInvalidDeadline = errors.New("invalid deadline")
	// ErrEmptyLot is returned when the lot number is empty.
	ErrEmptyLot = errors.New("lot number cannot be empty")
	// ErrInvalidProdCategory is returned when the production category is unknown.
	ErrInvalidProdCategory = errors.New("invalid production category")
	// ErrInvalidMachine is returned when the machine id is not positive.
	ErrInvalidMachine = errors.New("invalid machine id")
	// ErrInvalidPlanItem is returned when a work order covers no valid plan item.
	ErrInvalidPlanItem = errors.New("invalid plan item: a work order must cover at least one plan item with a positive quantity")
	// ErrDuplicatePlanItemLink is returned when the same plan item is listed
	// twice for one work order.
	ErrDuplicatePlanItemLink = errors.New("invalid merge: the same plan item is listed more than once")
	// ErrAnchorNotLinked is returned when the anchor plan item is missing from
	// the link set.
	ErrAnchorNotLinked = errors.New("invalid merge: the anchor plan item is not among the linked plan items")
	// ErrPlanItemAlreadyLinked is returned when a plan item is already covered
	// by another work order — a plan item can never be double-linked.
	ErrPlanItemAlreadyLinked = errors.New("invalid merge: plan item is already linked to a work order")
	// ErrNotMergeable is returned when a plan item does not satisfy the merge
	// predicate against the anchor (product, machine group, shade, deadline).
	ErrNotMergeable = errors.New("invalid merge: plan item is not mergeable with the anchor")
	// ErrInvalidRoute is returned when the route snapshot reference is invalid.
	ErrInvalidRoute = errors.New("invalid route: crh_head_id and crh_version required")
	// ErrInvalidRefType is returned when a WO reference type is not TEMPLATE/CONTINUATION.
	ErrInvalidRefType = errors.New("invalid reference type: must be TEMPLATE or CONTINUATION")
	// ErrIllegalTransition is returned when a status transition is not allowed.
	ErrIllegalTransition = errors.New("invalid work order status transition")
	// ErrInvalidApprovalSide is returned when an approval side is not PC/PM.
	ErrInvalidApprovalSide = errors.New("invalid approval side: must be PC or PM")
	// ErrNotSubmitted is returned when an approval targets a non-submitted WO.
	ErrNotSubmitted = errors.New("invalid operation: work order is not submitted")
	// ErrPCApprovalRequired is returned when PM approves before PC (sequential
	// approval, PRD v1.2).
	ErrPCApprovalRequired = errors.New("invalid operation: PC approval required before PM")
	// ErrMachineAreaMismatch is returned when the machine area does not match
	// the WO area code.
	ErrMachineAreaMismatch = errors.New("invalid machine: area does not match work order area")
	// ErrLotNotFound is returned when the WO lot is not in lot_master. The
	// message names the register route because an unknown lot is far more often
	// a typo than a genuinely new lot, and a silently-created one would carry no
	// standard weights for the ETL to price bobbins against.
	//
	// WIRE BEHAVIOR CHANGED DELIBERATELY: the previous text contained the words
	// "not found", which made domainErrorToBaseResponse classify it 404. This one
	// does not, so it classifies 400. That is the correct code -- the work order
	// is the resource being created and the lot is a rejected *field value* on
	// it, which is a validation failure rather than a missing resource. The 404
	// was an artifact of substring matching on the word "found", never a
	// decision. The 400 is pinned by TestLotErrors_MapToDistinctClientMessages so
	// it cannot drift back silently.
	ErrLotNotFound = errors.New("invalid lot: this " + CausePhraseLotNotRegistered +
		" — register it under Production Plan > Masters > Lots, or leave the lot blank to have one generated")
	// ErrLotSpecUnavailable is returned when a lot cannot be generated because
	// one of its required inputs is unknown. Registering a lot without them
	// would leave the ETL unable to convert bobbin counts into kilograms, so the
	// work order is rejected instead.
	//
	// It is the WRAPPER for the specific causes below, never returned on its own
	// from the generate path. Call sites that only care that generation failed
	// keep working through errors.Is; call sites that want to tell the planner
	// what to fix read the specific cause.
	ErrLotSpecUnavailable = errors.New("cannot generate lot")
	// ErrLotGenerationUnavailable is returned when lot generation is requested
	// but no lot provisioner is configured. Unlike the three causes in
	// lot_errors.go this is a deployment problem, not missing master data — but
	// the planner's workaround is still a master action (enter an existing lot),
	// so it carries a cause phrase and points at lot master like the rest.
	ErrLotGenerationUnavailable = errors.New("cannot generate lot: " + CausePhraseGenerationUnavailable +
		" — enter a lot number registered in lot master")
	// ErrEmptyReason is returned when a required reason (reject/adjust) is empty.
	ErrEmptyReason = errors.New("reason cannot be empty")
	// ErrNotEditable is returned when a header edit targets a non-DRAFT WO
	// without a revision reason.
	ErrNotEditable = errors.New("invalid operation: work order is not editable")
	// ErrActualNotFound is returned when a production actual row is not found.
	ErrActualNotFound = errors.New("work order production actual not found")
	// ErrNotDeletable is returned when deleting a non-DRAFT work order is attempted.
	ErrNotDeletable = errors.New("cannot delete work order: only DRAFT work orders can be deleted")

	// ── Carry-forward ────────────────────────────────────────────────────────

	// ErrWONotEligibleForCarry is returned when a WO's status makes it
	// permanently ineligible for carry-forward.
	ErrWONotEligibleForCarry = errors.New("this work order cannot be carried forward")
	// ErrAlreadyCarriedIntoMonth is returned when the source WO has already
	// been carried into the requested target month.
	ErrAlreadyCarriedIntoMonth = errors.New("this work order has already been carried into the target month")
	// ErrNothingToCarry is returned when a WO has no remaining qty to carry.
	ErrNothingToCarry = errors.New("nothing to carry: qty target already covered by production actual")
	// ErrCarryQtyExceedsRemaining is returned when the requested carry qty
	// exceeds what is left to produce.
	ErrCarryQtyExceedsRemaining = errors.New("carry quantity exceeds the remaining quantity")
	// ErrCarryTargetNotLater is returned when the target month is not strictly
	// later than the source WO's own month. A WO has no month column — the month
	// is TO_CHAR(wo_deadline,'YYYY-MM') — so a same-month carry produces a second
	// WO the candidate list then offers as a fresh candidate, and a backwards
	// carry moves work into a month whose production has already been reported.
	ErrCarryTargetNotLater = errors.New(
		"invalid target month: it must be later than the work order's own month")
	// ErrInvalidTargetMonth is returned when the target month is not YYYY-MM.
	ErrInvalidTargetMonth = errors.New("invalid target month: expected the form YYYY-MM")
)

// CarryIneligibleError wraps a WO that is permanently blocked from carry-forward
// with a reason, so the gRPC handler can surface it without collapsing all
// ineligibility conditions into one sentinel.
type CarryIneligibleError struct {
	WOID   int64
	Reason string
}

// Error implements error.
func (e CarryIneligibleError) Error() string {
	return fmt.Sprintf("work order %d cannot be carried forward: %s", e.WOID, e.Reason)
}

// NewCarryIneligibleError builds a CarryIneligibleError.
func NewCarryIneligibleError(woID int64, reason string) error {
	return CarryIneligibleError{WOID: woID, Reason: reason}
}
