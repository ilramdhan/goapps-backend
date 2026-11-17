package mbactivesync

import (
	"context"
	"errors"
	"fmt"
)

// Plan is the full output of --plan: one RowPlan per file row plus the head/spin plan.
type Plan struct {
	Rows  []*RowPlan // ln order
	Heads *HeadPlan
	Snap  *Snapshot
}

// Planner builds a Plan from a consistent snapshot and the parsed file rows.
type Planner struct {
	reader SnapshotReader
}

// NewPlanner returns a Planner reading through reader.
func NewPlanner(reader SnapshotReader) *Planner { return &Planner{reader: reader} }

// Build loads the snapshot (read-only) and plans file rows against it.
func (p *Planner) Build(ctx context.Context, rows []FileRow) (*Plan, error) {
	if p == nil || p.reader == nil {
		return nil, errors.New("planner has no snapshot reader")
	}
	snap, err := p.reader.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}
	return BuildPlan(rows, snap)
}

// BuildPlan is the pure part of Build. Order (design §3.5/§3.6):
// ClassifyRows -> PlanHeads -> GuardFGConflicts(exempt) -> SyncDecisions.
func BuildPlan(rows []FileRow, snap *Snapshot) (*Plan, error) {
	if snap == nil {
		return nil, errors.New("nil snapshot")
	}
	plans, err := ClassifyRows(rows, snap)
	if err != nil {
		return nil, fmt.Errorf("classify rows: %w", err)
	}
	hp := PlanHeads(plans, snap)
	GuardFGConflicts(plans, snap, hp.FGExempt())
	hp.SyncDecisions()
	return &Plan{Rows: plans, Heads: hp, Snap: snap}, nil
}
