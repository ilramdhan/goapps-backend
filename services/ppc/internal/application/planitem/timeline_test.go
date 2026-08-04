package planitem_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	planitemapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/planitem"
	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

// memRepo is an in-memory plan-item repository: enough to exercise the
// create/update timeline usecases without a database.
type memRepo struct {
	seq   int64
	items map[int64]*planitemdomain.PlanItem
	logs  []planitemdomain.LogEntry
	// coverage scripts CarryCoverage per plan item id; a missing entry means an
	// item with no work orders and no existing carry.
	coverage map[int64]planitemdomain.Coverage
	// candidates is what ListCarryCandidates returns, verbatim.
	candidates []*planitemdomain.CarryCandidate
}

func newMemRepo() *memRepo {
	return &memRepo{items: map[int64]*planitemdomain.PlanItem{}}
}

func (r *memRepo) Create(_ context.Context, e *planitemdomain.PlanItem) error {
	r.seq++
	assignID(e, r.seq)
	r.items[r.seq] = e
	return nil
}

// CreateBatch mirrors the Postgres batch write: ids are handed out in slice
// order and each pending batch-relative parent is stamped from the id assigned
// to that earlier item.
func (r *memRepo) CreateBatch(_ context.Context, items []*planitemdomain.PlanItem) error {
	ids := make([]int64, 0, len(items))
	for _, e := range items {
		if idx := e.PendingParentIndex(); idx != nil {
			if *idx >= len(ids) {
				return planitemdomain.ErrInvalidPendingParent
			}
			e.ResolvePendingParent(ids[*idx])
		}
		r.seq++
		assignID(e, r.seq)
		r.items[r.seq] = e
		ids = append(ids, r.seq)
	}
	return nil
}

// assignID rehydrates an entity with its generated id, preserving every other
// field, exactly as the repository's rehydratePlanItem does.
func assignID(e *planitemdomain.PlanItem, id int64) {
	*e = *planitemdomain.Reconstruct(planitemdomain.ReconstructParams{
		ID:                 id,
		CpmProductSysID:    e.CpmProductSysID(),
		Type:               e.Type(),
		DemandID:           e.DemandID(),
		ParentItemID:       e.ParentItemID(),
		QtyTarget:          e.QtyTarget(),
		Deadline:           e.Deadline(),
		RMSource:           e.RMSource(),
		Sequence:           e.Sequence(),
		Status:             e.Status(),
		MachineGroupID:     e.MachineGroupID(),
		PreferredMachineID: e.PreferredMachineID(),
		Month:              e.Month(),
		Notes:              e.Notes(),
		CreatedBy:          e.CreatedBy(),
		CreatedAt:          e.CreatedAt(),
		UpdatedAt:          e.UpdatedAt(),
		PlannedStartDate:   e.PlannedStartDate(),
		PlannedDuration:    e.PlannedDurationDays(),
		DurationSource:     e.DurationSource(),
		ShadeCode:          e.ShadeCode(),
		ShadeName:          e.ShadeName(),
		CarryFromItemID:    e.CarryFromItemID(),
		CarryAction:        e.CarryAction(),
	})
}

func (r *memRepo) GetByID(_ context.Context, id int64) (*planitemdomain.PlanItem, error) {
	e, ok := r.items[id]
	if !ok {
		return nil, planitemdomain.ErrNotFound
	}
	return e, nil
}

func (r *memRepo) List(_ context.Context, _ planitemdomain.ListFilter) ([]*planitemdomain.PlanItem, int64, error) {
	return nil, 0, nil
}

// Update records the change log only: GetByID hands back the live pointer, so
// the entity in the map is already the mutated one.
func (r *memRepo) Update(_ context.Context, _ *planitemdomain.PlanItem, changes []planitemdomain.LogEntry) error {
	r.logs = append(r.logs, changes...)
	return nil
}

func (r *memRepo) Delete(_ context.Context, id int64) error {
	delete(r.items, id)
	return nil
}

func (r *memRepo) ListForGantt(_ context.Context, _ planitemdomain.GanttFilter) ([]*planitemdomain.GanttRow, error) {
	return nil, nil
}

func (r *memRepo) ListCarryCandidates(_ context.Context, _, _ string) ([]*planitemdomain.CarryCandidate, error) {
	return r.candidates, nil
}

func (r *memRepo) CarryCoverage(_ context.Context, planItemID int64, _ string) (planitemdomain.Coverage, error) {
	return r.coverage[planItemID], nil
}

// fixedCapacity yields a constant per-day capacity for every product/group.
type fixedCapacity struct{ perDay float64 }

func (c fixedCapacity) DailyCapacity(_ context.Context, _, _ int64) (float64, error) {
	return c.perDay, nil
}

func deadline() time.Time { return time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC) }

func ptrFloat64(v float64) *float64 { return &v }
func ptrInt32(v int32) *int32       { return &v }

// newService wires a service over an in-memory repo at 100 units/day, so a
// qty of 500 derives a 5-day duration.
func newService() (*planitemapp.Service, *memRepo) {
	repo := newMemRepo()
	return planitemapp.NewService(repo, nil, nil).WithCapacity(fixedCapacity{perDay: 100}), repo
}

func createCmd() planitemapp.CreateCommand {
	demandID := int64(10)
	return planitemapp.CreateCommand{
		CpmProductSysID: 100,
		Type:            planitemdomain.TypeIntermediate,
		ParentItemID:    &demandID,
		QtyTarget:       500,
		Deadline:        deadline(),
		RMSource:        planitemdomain.RMSourceStore,
		MachineGroupID:  7,
		CreatedBy:       1,
	}
}

func TestCreate_NoTimeline_DerivesDurationFromCapacity(t *testing.T) {
	svc, _ := newService()

	res, err := svc.Create(context.Background(), createCmd())
	require.NoError(t, err)

	item := res.Item
	require.NotNil(t, item.PlannedDurationDays())
	assert.Equal(t, int32(5), *item.PlannedDurationDays())
	assert.Equal(t, planitemdomain.DurationSourceDerived, item.DurationSource())
	// Anchored to the deadline: 5 inclusive days ending 2026-08-15.
	assert.Equal(t, time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC), *item.PlannedStartDate())
}

func TestCreate_ManualTimeline_IsNotOverwrittenByCapacity(t *testing.T) {
	svc, _ := newService()

	cmd := createCmd()
	cmd.Timeline = planitemdomain.TimelineParams{DurationDays: ptrInt32(9)}
	res, err := svc.Create(context.Background(), cmd)
	require.NoError(t, err)

	assert.Equal(t, int32(9), *res.Item.PlannedDurationDays())
	assert.Equal(t, planitemdomain.DurationSourceManual, res.Item.DurationSource())
}

func TestUpdate_QtyChange_RecomputesDerivedDuration(t *testing.T) {
	svc, repo := newService()
	_, err := svc.Create(context.Background(), createCmd())
	require.NoError(t, err)
	id := repo.seq

	item, err := svc.Update(context.Background(), planitemapp.UpdateCommand{
		ID:        id,
		QtyTarget: ptrFloat64(2000),
		ChangedBy: 1,
	})
	require.NoError(t, err)

	assert.Equal(t, int32(20), *item.PlannedDurationDays())
	assert.Equal(t, planitemdomain.DurationSourceDerived, item.DurationSource())
}

// The D-2 guarantee: a planner override survives later quantity edits.
func TestUpdate_QtyChange_LeavesManualTimelineUntouched(t *testing.T) {
	svc, repo := newService()
	cmd := createCmd()
	cmd.Timeline = planitemdomain.TimelineParams{DurationDays: ptrInt32(9)}
	_, err := svc.Create(context.Background(), cmd)
	require.NoError(t, err)
	id := repo.seq

	before := *repo.items[id].PlannedStartDate()

	item, err := svc.Update(context.Background(), planitemapp.UpdateCommand{
		ID:        id,
		QtyTarget: ptrFloat64(2000),
		ChangedBy: 1,
	})
	require.NoError(t, err)

	assert.Equal(t, int32(9), *item.PlannedDurationDays())
	assert.Equal(t, planitemdomain.DurationSourceManual, item.DurationSource())
	assert.Equal(t, before, *item.PlannedStartDate())
}

func TestUpdate_TimelineSwitchesDerivedItemToManual(t *testing.T) {
	svc, repo := newService()
	_, err := svc.Create(context.Background(), createCmd())
	require.NoError(t, err)
	id := repo.seq

	item, err := svc.Update(context.Background(), planitemapp.UpdateCommand{
		ID:        id,
		Timeline:  planitemdomain.TimelineParams{DurationDays: ptrInt32(3)},
		ChangedBy: 1,
	})
	require.NoError(t, err)

	assert.Equal(t, int32(3), *item.PlannedDurationDays())
	assert.Equal(t, planitemdomain.DurationSourceManual, item.DurationSource())
	assert.Equal(t, time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC), *item.PlannedStartDate())
}

// Without a capacity provider the derived duration falls back to one day
// rather than failing the create.
func TestCreate_NoCapacityProvider_FallsBackToMinimum(t *testing.T) {
	repo := newMemRepo()
	svc := planitemapp.NewService(repo, nil, nil)

	res, err := svc.Create(context.Background(), createCmd())
	require.NoError(t, err)

	assert.Equal(t, planitemdomain.MinDurationDays, *res.Item.PlannedDurationDays())
	assert.Equal(t, planitemdomain.DurationSourceDerived, res.Item.DurationSource())
}
