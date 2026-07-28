package workorder

import (
	"context"

	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// MergeCandidateSource answers the merge-candidate query over plan items. It is
// optional (nil disables merging, and ListMergeCandidates then returns nothing)
// so the WO service keeps the same nil-safe contract as its other ports.
type MergeCandidateSource interface {
	// Subject loads one plan item's merge projection (product, machine group,
	// shade, deadline, qty, status).
	Subject(ctx context.Context, planItemID int64) (workorderdomain.MergeSubject, error)
	// Candidates lists plan items mergeable with anchor, excluding any already
	// linked to a work order.
	Candidates(ctx context.Context, anchor workorderdomain.MergeSubject, windowDays int32) ([]int64, error)
}

// ListMergeCandidates returns the plan-item ids that may join the anchor's work
// order. An unset or out-of-range window falls back to the 7-day default.
func (s *Service) ListMergeCandidates(ctx context.Context, anchorPlanItemID int64, windowDays int32) ([]int64, error) {
	if s.merge == nil {
		return nil, nil
	}
	anchor, err := s.merge.Subject(ctx, anchorPlanItemID)
	if err != nil {
		return nil, err
	}
	return s.merge.Candidates(ctx, anchor, normalizeMergeWindow(windowDays))
}

// normalizeMergeWindow clamps a caller-supplied deadline window.
func normalizeMergeWindow(windowDays int32) int32 {
	if windowDays <= 0 {
		return workorderdomain.DefaultMergeWindowDays
	}
	if windowDays > workorderdomain.MaxMergeWindowDays {
		return workorderdomain.MaxMergeWindowDays
	}
	return windowDays
}

// buildPlanItemLinks turns the anchor plus the planner's chosen extra plan
// items into the WO's link set, and returns the summed quantity.
//
// Every extra item is re-validated against the anchor with the domain
// predicate: the candidate list is a convenience for the UI, never a trust
// boundary. A contribution of zero means "use that plan item's own target".
func (s *Service) buildPlanItemLinks(ctx context.Context, cmd CreateCommand) ([]workorderdomain.PlanItemLink, float64, error) {
	if s.merge == nil || len(cmd.AdditionalPlanItemIDs) == 0 {
		return nil, cmd.QtyTarget, nil
	}
	anchor, err := s.merge.Subject(ctx, cmd.PlanItemID)
	if err != nil {
		return nil, 0, err
	}
	anchorQty := cmd.QtyTarget
	if anchorQty <= 0 {
		anchorQty = anchor.QtyTarget
	}
	links := []workorderdomain.PlanItemLink{{PlanItemID: cmd.PlanItemID, QtyContribution: anchorQty}}
	total := anchorQty

	for i, id := range cmd.AdditionalPlanItemIDs {
		candidate, err := s.merge.Subject(ctx, id)
		if err != nil {
			return nil, 0, err
		}
		if !workorderdomain.CanMerge(anchor, candidate, workorderdomain.DefaultMergeWindowDays) {
			return nil, 0, workorderdomain.ErrNotMergeable
		}
		qty := candidate.QtyTarget
		if i < len(cmd.QtyContributions) && cmd.QtyContributions[i] > 0 {
			qty = cmd.QtyContributions[i]
		}
		if qty <= 0 {
			return nil, 0, workorderdomain.ErrInvalidQty
		}
		links = append(links, workorderdomain.PlanItemLink{PlanItemID: id, QtyContribution: qty})
		total += qty
	}
	return links, total, nil
}
