package planitem

import (
	"context"
	"math"

	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

// CapacityProvider resolves the daily production capacity, in product units per
// day, available to a plan item's machine group for a given product.
//
// Implemented by the product-machine-capacity repository. May be nil, and a
// zero or negative result is not an error: capacity master data is incomplete
// for many product/machine pairs, and planning must not block on it.
type CapacityProvider interface {
	DailyCapacity(ctx context.Context, cpmProductSysID, machineGroupID int64) (float64, error)
}

// derivedDurationFor computes the planned duration for a plan item, in whole
// days, clamped to [MinDurationDays, MaxDurationDays]. A missing capacity
// provider, a lookup failure, or a non-positive capacity all yield the minimum
// duration rather than an error — planning must not block on incomplete
// capacity master data.
func (s *Service) derivedDurationFor(ctx context.Context, qty float64, cpmProductSysID, machineGroupID int64) int32 {
	if s.capacity == nil || qty <= 0 {
		return planitemdomain.MinDurationDays
	}
	perDay, err := s.capacity.DailyCapacity(ctx, cpmProductSysID, machineGroupID)
	if err != nil || perDay <= 0 {
		return planitemdomain.MinDurationDays
	}
	days := math.Ceil(qty / perDay)
	if days < float64(planitemdomain.MinDurationDays) {
		return planitemdomain.MinDurationDays
	}
	if days > float64(planitemdomain.MaxDurationDays) {
		return planitemdomain.MaxDurationDays
	}
	return int32(days) //nolint:gosec // clamped to [Min,Max]DurationDays above
}

// applyDerivedTimeline stamps a system-derived duration onto an item. It is a
// no-op for MANUAL items, so a planner override survives later quantity edits.
func (s *Service) applyDerivedTimeline(ctx context.Context, item *planitemdomain.PlanItem) {
	if !item.IsDurationDerived() {
		return
	}
	item.ApplyDerivedDuration(
		s.derivedDurationFor(ctx, item.QtyTarget(), item.CpmProductSysID(), item.MachineGroupID()),
	)
}
