package workorder

import (
	"context"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/application/notification"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// autoApproveActor is the synthetic user id recorded for auto-approvals.
const autoApproveActor int64 = 0

// AutoApproveResult summarizes one auto-approve sweep.
type AutoApproveResult struct {
	// Scanned is the number of pending WOs inspected.
	Scanned int
	// Skipped counts WOs left untouched because auto-approve is disabled.
	Skipped int
	// PCApproved counts PC sides auto-approved this sweep.
	PCApproved int
	// PMApproved counts PM sides auto-approved this sweep.
	PMApproved int
	// FullyApproved counts WOs that became APPROVED this sweep.
	FullyApproved int
}

// AutoApprovePending auto-approves the pending PC/PM side of WOs whose last
// update is at or before now.Add(-window), advancing the sequential PC→PM chain
// (PRD v1.2: auto 24h, PC then PM). WOs with auto-approve disabled are skipped so
// they never run without an explicit PM approval. A per-WO error is returned to
// the caller (the worker logs-not-crashes).
func (s *Service) AutoApprovePending(ctx context.Context, now time.Time, window time.Duration) (AutoApproveResult, error) {
	cutoff := now.Add(-window)
	pending, err := s.repo.ListPendingApprovals(ctx, cutoff)
	if err != nil {
		return AutoApproveResult{}, err
	}

	res := AutoApproveResult{Scanned: len(pending)}
	for _, wo := range pending {
		if wo.AutoApproveDisabled() {
			res.Skipped++
			continue
		}
		if err := s.autoApproveOne(ctx, wo, now, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (s *Service) autoApproveOne(ctx context.Context, wo *workorderdomain.WorkOrder, now time.Time, res *AutoApproveResult) error {
	if wo.Status() == workorderdomain.StatusSubmitted {
		if _, err := wo.ApprovePC(autoApproveActor, now); err != nil {
			return err
		}
		res.PCApproved++
	}
	fullyApproved, err := wo.ApprovePM(autoApproveActor, now)
	if err != nil {
		return err
	}
	res.PMApproved++
	if err := s.repo.Update(ctx, wo); err != nil {
		return err
	}
	if fullyApproved {
		res.FullyApproved++
		notification.Notify(ctx, s.notifier, notification.Message{
			Event:      notification.EventWOApproved,
			Subject:    "Work order auto-approved",
			Recipients: []string{"PPC"},
			EntityID:   wo.ID(),
		})
	}
	return nil
}
