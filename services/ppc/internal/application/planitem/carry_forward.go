package planitem

import (
	"context"
	"slices"
	"time"

	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

// ProcessPlanCarryForwardCommand carries inputs for a month-start plan-item
// carry-forward. Shaped after demand's ProcessCarryForwardCommand, minus the
// split children and the deferral the plan-item lifecycle has no state for.
type ProcessPlanCarryForwardCommand struct {
	SourcePlanItemID int64
	Action           string
	TargetMonth      string
	NewDeadline      *time.Time
	CarryQty         *float64
	ActedBy          int64
}

// ListCarryCandidates returns plan items eligible for carry-forward out of
// sourceMonth, each decorated with how much of it work orders already cover and
// whether targetMonth already holds a row carried from it.
func (s *Service) ListCarryCandidates(ctx context.Context, sourceMonth, targetMonth string) ([]*planitemdomain.CarryCandidate, error) {
	return s.repo.ListCarryCandidates(ctx, sourceMonth, targetMonth)
}

// ProcessPlanCarryForward executes one carry-forward action against a source
// plan item, returning the plan item created in the target month (nil for
// CANCEL, which creates nothing).
//
// Validation order mirrors demand's ProcessCarryForward: load the source,
// reject it if its own status makes it ineligible, then dispatch on the action.
// The extra step this has and demand does not is coverage — a plan item's claim
// on its demand is partly consumed the moment a work order is raised against
// it, so how much is left to carry cannot be read off the entity alone.
func (s *Service) ProcessPlanCarryForward(ctx context.Context, cmd ProcessPlanCarryForwardCommand) (*planitemdomain.PlanItem, error) {
	source, err := s.repo.GetByID(ctx, cmd.SourcePlanItemID)
	if err != nil {
		return nil, err
	}
	if !planitemdomain.IsValidCarryAction(cmd.Action) {
		return nil, planitemdomain.ErrInvalidCarryAction
	}
	if !source.IsCarryCandidate() {
		return nil, planitemdomain.ErrNotCarryCandidate
	}
	if cmd.Action == planitemdomain.CarryActionCancel {
		return nil, s.carryCancel(ctx, source, cmd.ActedBy)
	}
	return s.carryIntoMonth(ctx, source, cmd)
}

// carryIntoMonth handles the two actions that create a plan item in the target
// month. Both are guarded by the same three checks, so they share one path.
func (s *Service) carryIntoMonth(
	ctx context.Context, source *planitemdomain.PlanItem, cmd ProcessPlanCarryForwardCommand,
) (*planitemdomain.PlanItem, error) {
	if cmd.TargetMonth == source.Month() {
		return nil, planitemdomain.ErrSameMonth
	}
	coverage, err := s.repo.CarryCoverage(ctx, source.ID(), cmd.TargetMonth)
	if err != nil {
		return nil, err
	}
	// Re-running the same target month must not duplicate (S-2.4). The candidate
	// list already flags this, but the list is a convenience, not a trust
	// boundary. Scoped to the requested target month: Aug→Sep must not block a
	// later, legitimate Aug→Oct — that one is bounded by QtyCarriedAway instead.
	if slices.Contains(coverage.CarriedToMonths, cmd.TargetMonth) {
		return nil, planitemdomain.ErrAlreadyCarried
	}

	candidate := &planitemdomain.CarryCandidate{Item: source, Coverage: coverage}
	uncovered := candidate.QtyUncovered()
	if uncovered <= 0 {
		return nil, planitemdomain.ErrNothingToCarry
	}

	qty, err := carryQty(cmd, uncovered)
	if err != nil {
		return nil, err
	}
	return s.createCarriedItem(ctx, source, cmd, qty)
}

// carryQty resolves how much to carry: the whole uncovered remainder for
// CARRY_AS_IS, or the requested amount for PARTIAL_CARRY, which may not exceed
// it. Asking for more than is uncovered is exactly the double-count S-2.2
// forbids, so it is rejected rather than clamped.
func carryQty(cmd ProcessPlanCarryForwardCommand, uncovered float64) (float64, error) {
	if cmd.Action != planitemdomain.CarryActionPartial {
		return uncovered, nil
	}
	if cmd.CarryQty == nil || *cmd.CarryQty <= 0 {
		return 0, planitemdomain.ErrInvalidQty
	}
	if *cmd.CarryQty > uncovered {
		return 0, planitemdomain.ErrCarryQtyExceedsUncovered
	}
	return *cmd.CarryQty, nil
}

// createCarriedItem builds and persists the target-month plan item cloned from
// source.
//
// No cascade is run. The source item's own cascade already produced the
// upstream INTERMEDIATE items, and those are themselves carry candidates of the
// same month — re-cascading here would generate a second upstream chain for
// production that is already planned.
func (s *Service) createCarriedItem(
	ctx context.Context, source *planitemdomain.PlanItem, cmd ProcessPlanCarryForwardCommand, qty float64,
) (*planitemdomain.PlanItem, error) {
	fromID := source.ID()
	child, err := planitemdomain.New(planitemdomain.NewParams{
		CpmProductSysID: source.CpmProductSysID(),
		Type:            source.Type(),
		DemandID:        source.DemandID(),
		ParentItemID:    source.ParentItemID(),
		QtyTarget:       qty,
		Deadline:        carryDeadline(source, cmd.NewDeadline),
		RMSource:        source.RMSource(),
		ShadeCode:       source.ShadeCode(),
		ShadeName:       source.ShadeName(),
		MachineGroupID:  source.MachineGroupID(),
		// Machine intent is preserved (S-2.2): the planner already decided which
		// machine should run this, and that decision does not expire with the month.
		PreferredMachineID: source.PreferredMachineID(),
		Month:              cmd.TargetMonth,
		// Carry-forward is the documented legitimate month override
		// (entity.go resolveMonth): the remainder is parked in a later month than
		// its own deadline implies.
		MonthOverride:   true,
		Notes:           source.Notes(),
		CreatedBy:       cmd.ActedBy,
		CarryFromItemID: &fromID,
		CarryAction:     cmd.Action,
	})
	if err != nil {
		return nil, err
	}
	s.applyDerivedTimeline(ctx, child)
	if err := s.repo.Create(ctx, child); err != nil {
		return nil, err
	}

	// The source is deliberately NOT closed. Unlike a demand — whose remaining
	// qty is a single running balance that MarkCarriedOver hands to its child —
	// a plan item's claim is tracked through wo_plan_item_link, and the source
	// may still be legitimately worked in its own month. Closing it would also
	// break the work orders already raised against it. The carry link on the
	// child is the record that the work moved.
	return child, nil
}

// carryCancel closes a plan item without carrying it, recording the decision in
// the plan-change log so the month-start run is auditable (S-2.4).
func (s *Service) carryCancel(ctx context.Context, source *planitemdomain.PlanItem, actedBy int64) error {
	before := source.Status()
	if err := source.Close(); err != nil {
		return err
	}
	return s.repo.Update(ctx, source, []planitemdomain.LogEntry{{
		Field:     "status",
		Before:    before,
		After:     source.Status(),
		ChangedBy: actedBy,
		Reason:    "Closed at month start instead of being carried forward",
	}})
}

// carryDeadline keeps the source deadline unless the planner supplied a new one.
func carryDeadline(source *planitemdomain.PlanItem, override *time.Time) time.Time {
	if override != nil && !override.IsZero() {
		return *override
	}
	return source.Deadline()
}
