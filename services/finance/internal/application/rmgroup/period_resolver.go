package rmgroup

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// InheritedFromAnchor is the inheritedFrom marker returned by the period
// resolvers when no snapshot exists for the period or any earlier period, so
// the values come from the anchor cst_rm_group_head / cst_rm_group_detail row.
const InheritedFromAnchor = "ANCHOR"

// Carry-forward resolution (backlog1 design, Item 3). A period's effective
// head/detail config is resolved through one chain, shared by the update
// handlers, the period-aware read (GetHandler) and the calc engine
// (rmcost.CalculateHandlerV2):
//
//  1. exact snapshot row for the period             -> inheritedFrom ""
//  2. latest snapshot row with an earlier period    -> inheritedFrom "<YYYYMM>"
//  3. the anchor row                                -> inheritedFrom "ANCHOR"
//
// For branches 2 and 3 the returned snapshot is a fresh, not-yet-persisted
// working copy keyed on the requested period (new ID, Period = period), so a
// caller that patches and upserts it seeds the new period from the value the
// user was actually looking at.

// ResolveHeadSnapshot resolves the effective (head, period) snapshot through
// the carry-forward chain. See the chain description above.
func ResolveHeadSnapshot(
	ctx context.Context, repo rmgroup.Repository, head *rmgroup.Head, period string,
) (*rmgroup.HeadPeriodSnapshot, string, error) {
	snap, err := repo.GetHeadPeriodSnapshot(ctx, head.ID(), period)
	if err == nil {
		return snap, "", nil
	}
	if !errors.Is(err, rmgroup.ErrNotFound) {
		return nil, "", fmt.Errorf("load head period snapshot: %w", err)
	}

	prev, err := repo.GetLatestHeadPeriodSnapshotBefore(ctx, head.ID(), period)
	if err == nil {
		carried := prev.CarryForwardTo(period)
		return &carried, prev.Period, nil
	}
	if !errors.Is(err, rmgroup.ErrNotFound) {
		return nil, "", fmt.Errorf("load previous head period snapshot: %w", err)
	}

	fresh := rmgroup.NewHeadPeriodSnapshotFromHead(period, head)
	return &fresh, InheritedFromAnchor, nil
}

// ResolvedDetailSnapshot pairs a detail's effective snapshot for a period
// with where it was resolved from ("" exact, "ANCHOR", or the source YYYYMM).
type ResolvedDetailSnapshot struct {
	Snapshot      *rmgroup.DetailPeriodSnapshot
	InheritedFrom string
}

// ResolveDetailSnapshots resolves the effective snapshot for every detail of
// one head for period, in the same order as details. Exact rows are read per
// detail; the carry-forward lookup is one batched query per head, issued only
// when at least one detail has no exact row. All details must belong to
// headID.
func ResolveDetailSnapshots(
	ctx context.Context, repo rmgroup.Repository, headID uuid.UUID, details []*rmgroup.Detail, period string,
) ([]ResolvedDetailSnapshot, error) {
	out := make([]ResolvedDetailSnapshot, len(details))
	missing := make([]int, 0, len(details))
	for i, d := range details {
		snap, err := repo.GetDetailPeriodSnapshot(ctx, d.ID(), period)
		if err == nil {
			out[i] = ResolvedDetailSnapshot{Snapshot: snap}
			continue
		}
		if !errors.Is(err, rmgroup.ErrNotFound) {
			return nil, fmt.Errorf("load detail period snapshot: %w", err)
		}
		missing = append(missing, i)
	}
	if len(missing) == 0 {
		return out, nil
	}

	prevByDetail, err := repo.GetLatestDetailPeriodSnapshotsBefore(ctx, headID, period)
	if err != nil {
		return nil, fmt.Errorf("load previous detail period snapshots: %w", err)
	}
	for _, i := range missing {
		d := details[i]
		if prev, ok := prevByDetail[d.ID()]; ok && prev != nil {
			carried := prev.CarryForwardTo(period)
			out[i] = ResolvedDetailSnapshot{Snapshot: &carried, InheritedFrom: prev.Period}
			continue
		}
		fresh := rmgroup.NewDetailPeriodSnapshotFromDetail(period, d)
		out[i] = ResolvedDetailSnapshot{Snapshot: &fresh, InheritedFrom: InheritedFromAnchor}
	}
	return out, nil
}

// ResolveDetailSnapshot resolves the effective snapshot for a single detail —
// a convenience wrapper over ResolveDetailSnapshots for the update-item path.
func ResolveDetailSnapshot(
	ctx context.Context, repo rmgroup.Repository, detail *rmgroup.Detail, period string,
) (*rmgroup.DetailPeriodSnapshot, string, error) {
	res, err := ResolveDetailSnapshots(ctx, repo, detail.HeadID(), []*rmgroup.Detail{detail}, period)
	if err != nil {
		return nil, "", err
	}
	return res[0].Snapshot, res[0].InheritedFrom, nil
}

// FrozenHeadCopy returns the head snapshot to persist as the exact row for its period
// when a calculation freezes an inherited value (freeze-on-calc): provenance
// is marked is_backfilled = true, and carried_from_period is the source
// period, or nil when the value came from the anchor row. Returns nil when
// inheritedFrom is "" (an exact row already exists — nothing to freeze).
func FrozenHeadCopy(snap *rmgroup.HeadPeriodSnapshot, inheritedFrom, frozenBy string) *rmgroup.HeadPeriodSnapshot {
	if snap == nil || inheritedFrom == "" {
		return nil
	}
	frozen := *snap
	frozen.IsBackfilled = true
	frozen.CarriedFromPeriod = carriedFromPtr(inheritedFrom)
	frozen.CreatedBy = frozenBy
	frozen.UpdatedAt = nil
	frozen.UpdatedBy = nil
	return &frozen
}

// FrozenDetailCopy is the detail counterpart of FrozenHeadCopy.
func FrozenDetailCopy(snap *rmgroup.DetailPeriodSnapshot, inheritedFrom, frozenBy string) *rmgroup.DetailPeriodSnapshot {
	if snap == nil || inheritedFrom == "" {
		return nil
	}
	frozen := *snap
	frozen.IsBackfilled = true
	frozen.CarriedFromPeriod = carriedFromPtr(inheritedFrom)
	frozen.CreatedBy = frozenBy
	frozen.UpdatedAt = nil
	frozen.UpdatedBy = nil
	return &frozen
}

func carriedFromPtr(inheritedFrom string) *string {
	if inheritedFrom == "" || inheritedFrom == InheritedFromAnchor {
		return nil
	}
	p := inheritedFrom
	return &p
}
