package workorder

import (
	"context"
	"time"

	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// monthLayout is the wire form of a planning month. A WO's month is derived
// from its deadline, so this layout both parses the requested target month and
// renders a deadline back into a month for comparison.
const monthLayout = "2006-01"

// ineligibleWOStatuses maps status codes that may never be carried to a
// human-readable reason. Everything not in this map is eligible — including
// SUBMITTED and PC_APPROVED, whose mid-approval state does not block creating
// a continuation (the source is untouched).
var ineligibleWOStatuses = map[string]string{
	workorderdomain.StatusDraft:     "is still a draft — confirm it first",
	workorderdomain.StatusCompleted: "production is already complete",
	workorderdomain.StatusClosed:    "production is closed and its final quantity is locked",
	workorderdomain.StatusRejected:  "was rejected — create a new plan item instead",
	workorderdomain.StatusCancelled: "was cancelled — create a new plan item instead",
}

// ProcessWorkOrderCarryForwardCommand carries inputs for a month-start WO
// carry-forward.
type ProcessWorkOrderCarryForwardCommand struct {
	SourceWOID  int64
	TargetMonth string
	LotNo       string
	CarryQty    *float64
	ActedBy     int64
}

// ListWorkOrderCarryCandidates returns WOs eligible to be carried from sourceMonth
// into targetMonth. Ineligible rows are included with their reason (S-2.3).
func (s *Service) ListWorkOrderCarryCandidates(ctx context.Context, sourceMonth, targetMonth string) ([]*workorderdomain.CarryCandidate, error) {
	return s.woCarryRepo.ListCandidates(ctx, sourceMonth, targetMonth)
}

// ProcessWorkOrderCarryForward carries one WO into targetMonth as a
// CONTINUATION via CreateWOReference, reusing the existing continuation path.
func (s *Service) ProcessWorkOrderCarryForward(ctx context.Context, cmd ProcessWorkOrderCarryForwardCommand) (*workorderdomain.WorkOrder, error) {
	src, err := s.repo.GetByID(ctx, cmd.SourceWOID)
	if err != nil {
		return nil, err
	}
	if reason, exists := ineligibleWOStatuses[src.Status()]; exists {
		return nil, workorderdomain.NewCarryIneligibleError(cmd.SourceWOID, reason)
	}
	if err := validateCarryTargetMonth(src.Deadline(), cmd.TargetMonth); err != nil {
		return nil, err
	}
	already, err := s.woCarryRepo.IsAlreadyCarriedInto(ctx, cmd.SourceWOID, cmd.TargetMonth)
	if err != nil {
		return nil, err
	}
	if already {
		return nil, workorderdomain.ErrAlreadyCarriedIntoMonth
	}
	coverage, err := s.woCarryRepo.CarryCoverage(ctx, cmd.SourceWOID)
	if err != nil {
		return nil, err
	}
	candidate := &workorderdomain.CarryCandidate{WO: src, Coverage: coverage}
	remaining := candidate.QtyRemaining()
	if remaining <= 0 {
		return nil, workorderdomain.ErrNothingToCarry
	}
	qty := remaining
	if cmd.CarryQty != nil && *cmd.CarryQty > 0 {
		if *cmd.CarryQty > remaining {
			return nil, workorderdomain.ErrCarryQtyExceedsRemaining
		}
		qty = *cmd.CarryQty
	}
	return s.CreateWOReference(ctx, CreateWOReferenceCommand{
		SourceWOID: cmd.SourceWOID,
		RefType:    workorderdomain.RefTypeContinuation,
		LotNo:      cmd.LotNo,
		QtyTarget:  qty,
		Deadline:   carryDeadlineWO(src.Deadline(), cmd.TargetMonth),
		MachineID:  nil, // inherit source machine
		CreatedBy:  cmd.ActedBy,
	})
}

// validateCarryTargetMonth rejects a target month that is not strictly later
// than the source WO's own month.
//
// A WO has no month column: both the candidate query and the already-carried
// check derive it as TO_CHAR(wo_deadline,'YYYY-MM'). So the deadline is the
// month, and a carry into that same month produces a second WO whose deadline
// lands right back in the source month — the candidate list would then offer the
// continuation as a fresh candidate for the same move, and the planner could
// walk the whole remaining qty into an unbounded chain of same-month WOs.
// Carrying backwards is worse: it pushes work into a month whose production has
// already been reported and, for a closed month, whose costs are settled.
//
// The date-level comparison in carryDeadlineWO does not cover this — it only
// pushes a deadline forward to the first of the target month, and silently
// returns the source deadline unchanged when the month string will not parse.
func validateCarryTargetMonth(sourceDeadline time.Time, targetMonth string) error {
	target, err := time.Parse(monthLayout, targetMonth)
	if err != nil {
		return workorderdomain.ErrInvalidTargetMonth
	}
	// Compare as formatted months, not as instants: the deadline carries a
	// time-of-day and a location, and "2026-09" > "2026-08" lexicographically for
	// every zero-padded YYYY-MM.
	if target.Format(monthLayout) <= sourceDeadline.Format(monthLayout) {
		return workorderdomain.ErrCarryTargetNotLater
	}
	return nil
}

// carryDeadlineWO resolves a carry-forward deadline.
func carryDeadlineWO(source time.Time, targetMonth string) time.Time {
	target, err := time.Parse(monthLayout, targetMonth)
	if err != nil {
		return source
	}
	targetFirst := time.Date(target.Year(), target.Month(), 1, 0, 0, 0, 0, source.Location())
	if source.Before(targetFirst) {
		return targetFirst
	}
	return source
}
