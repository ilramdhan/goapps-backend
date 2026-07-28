package workorder_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// ---------------------------------------------------------------------------
// Fixture: the plan-03 exit-criteria scenario.
//
// Four customer contracts for the SAME product in four different finished
// shades (red/yellow/blue/pink). Each cascades to a tty-level plan item, and at
// the tty level every item is undyed — shade NL. Those four tty items are the
// merge candidates; the four FG items are not mergeable with each other.
// ---------------------------------------------------------------------------

const (
	shadeNatural = "NL"
	statusDraft  = "DRAFT"
)

// mergeStubSource is an in-memory MergeCandidateSource over a fixed plan-item
// set, applying the real domain predicate so the fixture exercises the same
// rule the SQL implements.
type mergeStubSource struct {
	subjects map[int64]workorderdomain.MergeSubject
	// linked marks plan items already covered by a WO; the SQL excludes these
	// via NOT EXISTS, so the stub must too.
	linked map[int64]bool
}

func (s *mergeStubSource) Subject(_ context.Context, planItemID int64) (workorderdomain.MergeSubject, error) {
	subj, ok := s.subjects[planItemID]
	if !ok {
		return workorderdomain.MergeSubject{}, workorderdomain.ErrNotFound
	}
	return subj, nil
}

func (s *mergeStubSource) Candidates(
	_ context.Context, anchor workorderdomain.MergeSubject, windowDays int32,
) ([]int64, error) {
	ids := []int64{}
	for id, subj := range s.subjects {
		if s.linked[id] {
			continue
		}
		if workorderdomain.CanMerge(anchor, subj, windowDays) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// mergeFixture builds the four-contract scenario: ids 101-104 are the FG items
// (four distinct shades), ids 201-204 the tty items they cascade to (all NL).
func mergeFixture() *mergeStubSource {
	deadline := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	fgShades := []string{"RED", "YEL", "BLU", "PNK"}
	src := &mergeStubSource{
		subjects: make(map[int64]workorderdomain.MergeSubject, 8),
		linked:   map[int64]bool{},
	}
	for i, shade := range fgShades {
		fgID := int64(101 + i)
		ttyID := int64(201 + i)
		src.subjects[fgID] = workorderdomain.MergeSubject{
			PlanItemID: fgID, ProductSysID: 900, MachineGroupID: 5,
			ShadeCode: shade, Deadline: deadline, QtyTarget: 1000, Status: statusDraft,
		}
		src.subjects[ttyID] = workorderdomain.MergeSubject{
			PlanItemID: ttyID, ProductSysID: 901, MachineGroupID: 7,
			ShadeCode: shadeNatural, Deadline: deadline, QtyTarget: 1000, Status: statusDraft,
		}
	}
	return src
}

// mergeStubRepo records created WOs and enforces the one-WO-per-plan-item rule
// the same way the unique index uq_wpl_plan_item does in Postgres.
type mergeStubRepo struct {
	woStubRepo
	created []*workorderdomain.WorkOrder
	linked  map[int64]int64 // plan item -> wo id
	nextID  int64
}

func newMergeStubRepo() *mergeStubRepo {
	return &mergeStubRepo{linked: map[int64]int64{}, nextID: 1}
}

func (r *mergeStubRepo) Create(_ context.Context, e *workorderdomain.WorkOrder) error {
	for _, l := range e.PlanItemLinks() {
		if _, exists := r.linked[l.PlanItemID]; exists {
			return workorderdomain.ErrPlanItemAlreadyLinked
		}
	}
	e.SetID(r.nextID)
	r.nextID++
	for _, l := range e.PlanItemLinks() {
		r.linked[l.PlanItemID] = e.ID()
	}
	r.created = append(r.created, e)
	return nil
}

func (r *mergeStubRepo) GetByID(_ context.Context, id int64) (*workorderdomain.WorkOrder, error) {
	for _, e := range r.created {
		if e.ID() == id {
			return e, nil
		}
	}
	return nil, workorderdomain.ErrNotFound
}

func createMergedWO(
	t *testing.T, svc *workorderapp.Service, anchor int64, extras []int64,
) (*workorderdomain.WorkOrder, error) {
	t.Helper()
	return svc.Create(context.Background(), workorderapp.CreateCommand{
		AreaCode:   "TXT",
		PlanItemID: anchor,
		MachineID:  2,
		CrhHeadID:  10,
		CrhVersion: 1,
		// A merge test is not a lot test: supply a lot so the create path takes
		// the manual branch (lot validation is disabled when Lots is nil).
		LotNo:                 "TXT0001-26",
		QtyTarget:             1000,
		Deadline:              time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		CreatedBy:             1,
		AdditionalPlanItemIDs: extras,
	})
}

// ---------------------------------------------------------------------------
// T3.8 functional gate
// ---------------------------------------------------------------------------

func TestListMergeCandidates_TtyAnchorReturnsThreeSiblings(t *testing.T) {
	src := mergeFixture()
	svc := workorderapp.NewService(newMergeStubRepo(), workorderapp.Deps{Merge: src})

	ids, err := svc.ListMergeCandidates(context.Background(), 201, 0)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{202, 203, 204}, ids,
		"the three sibling tty items (all NL) must be mergeable with the tty anchor")
}

func TestListMergeCandidates_FGAnchorReturnsNone(t *testing.T) {
	src := mergeFixture()
	svc := workorderapp.NewService(newMergeStubRepo(), workorderapp.Deps{Merge: src})

	ids, err := svc.ListMergeCandidates(context.Background(), 101, 0)
	require.NoError(t, err)
	assert.Empty(t, ids, "FG items carry four distinct shades and must never merge")
}

func TestListMergeCandidates_ExcludesAlreadyLinkedItems(t *testing.T) {
	src := mergeFixture()
	src.linked[203] = true
	svc := workorderapp.NewService(newMergeStubRepo(), workorderapp.Deps{Merge: src})

	ids, err := svc.ListMergeCandidates(context.Background(), 201, 0)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{202, 204}, ids)
}

func TestListMergeCandidates_WindowClampedToDefault(t *testing.T) {
	src := mergeFixture()
	// Push one sibling 20 days out: outside the 7-day default, inside a 30-day ask.
	far := src.subjects[204]
	far.Deadline = far.Deadline.AddDate(0, 0, 20)
	src.subjects[204] = far
	svc := workorderapp.NewService(newMergeStubRepo(), workorderapp.Deps{Merge: src})

	defaults, err := svc.ListMergeCandidates(context.Background(), 201, 0)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{202, 203}, defaults, "unset window must default to 7 days")

	wide, err := svc.ListMergeCandidates(context.Background(), 201, 30)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{202, 203, 204}, wide)
}

func TestListMergeCandidates_NilPortReturnsNothing(t *testing.T) {
	svc := workorderapp.NewService(newMergeStubRepo(), workorderapp.Deps{})
	ids, err := svc.ListMergeCandidates(context.Background(), 201, 0)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

// ---------------------------------------------------------------------------
// Plan-03 exit criterion 1: four contracts -> four tty items -> ONE work order
// with the summed quantity.
// ---------------------------------------------------------------------------

func TestCreateWorkOrder_FourContractsMergeIntoOneWO(t *testing.T) {
	src := mergeFixture()
	repo := newMergeStubRepo()
	svc := workorderapp.NewService(repo, workorderapp.Deps{Merge: src})

	entity, err := createMergedWO(t, svc, 201, []int64{202, 203, 204})
	require.NoError(t, err)

	assert.Len(t, repo.created, 1, "the four tty items must produce exactly one work order")
	assert.Equal(t, float64(4000), entity.QtyTarget(), "qty must be the sum of the contributions")
	require.Len(t, entity.PlanItemLinks(), 4)
	assert.Equal(t, int64(201), entity.PlanItemID(), "the anchor stays the plan item the planner started from")

	ids := make([]int64, 0, 4)
	for _, l := range entity.PlanItemLinks() {
		ids = append(ids, l.PlanItemID)
		assert.Positive(t, l.QtyContribution)
	}
	assert.ElementsMatch(t, []int64{201, 202, 203, 204}, ids)
}

func TestCreateWorkOrder_ContributionsOverrideDefaults(t *testing.T) {
	src := mergeFixture()
	repo := newMergeStubRepo()
	svc := workorderapp.NewService(repo, workorderapp.Deps{Merge: src})

	entity, err := svc.Create(context.Background(), workorderapp.CreateCommand{
		AreaCode: "TXT", PlanItemID: 201, MachineID: 2, CrhHeadID: 10, CrhVersion: 1,
		LotNo:     "TXT0001-26",
		QtyTarget: 1000, Deadline: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC), CreatedBy: 1,
		AdditionalPlanItemIDs: []int64{202, 203},
		// Second entry left at zero: falls back to that plan item's own target.
		QtyContributions: []float64{250, 0},
	})
	require.NoError(t, err)
	assert.Equal(t, float64(2250), entity.QtyTarget())
}

func TestCreateWorkOrder_RejectsNonMergeableExtras(t *testing.T) {
	src := mergeFixture()
	svc := workorderapp.NewService(newMergeStubRepo(), workorderapp.Deps{Merge: src})

	// 101 is an FG item: different product, machine group and shade. The client
	// may ask for it; the server re-checks the predicate and refuses.
	_, err := createMergedWO(t, svc, 201, []int64{101})
	require.ErrorIs(t, err, workorderdomain.ErrNotMergeable)
}

func TestCreateWorkOrder_SinglePlanItemStillLinksItself(t *testing.T) {
	src := mergeFixture()
	repo := newMergeStubRepo()
	svc := workorderapp.NewService(repo, workorderapp.Deps{Merge: src})

	entity, err := createMergedWO(t, svc, 201, nil)
	require.NoError(t, err)
	require.Len(t, entity.PlanItemLinks(), 1, "an unmerged WO still owns one link row")
	assert.Equal(t, int64(201), entity.PlanItemLinks()[0].PlanItemID)
	assert.Equal(t, float64(1000), entity.QtyTarget())
}

// ---------------------------------------------------------------------------
// Plan-03 exit criterion 2: a plan item can never be linked to two WOs.
// ---------------------------------------------------------------------------

func TestCreateWorkOrder_PlanItemCannotBeDoubleLinked(t *testing.T) {
	src := mergeFixture()
	repo := newMergeStubRepo()
	svc := workorderapp.NewService(repo, workorderapp.Deps{Merge: src})

	_, err := createMergedWO(t, svc, 201, []int64{202})
	require.NoError(t, err)

	// A second WO trying to claim 202 (already covered) must be refused. The
	// candidate query would not have offered it, but a hand-crafted request can.
	_, err = createMergedWO(t, svc, 203, []int64{202})
	require.ErrorIs(t, err, workorderdomain.ErrPlanItemAlreadyLinked)
	assert.Len(t, repo.created, 1, "the rejected WO must not be persisted")
}

func TestCreateWorkOrder_AnchorCannotBeDoubleLinked(t *testing.T) {
	src := mergeFixture()
	repo := newMergeStubRepo()
	svc := workorderapp.NewService(repo, workorderapp.Deps{Merge: src})

	_, err := createMergedWO(t, svc, 201, nil)
	require.NoError(t, err)

	_, err = createMergedWO(t, svc, 201, nil)
	require.ErrorIs(t, err, workorderdomain.ErrPlanItemAlreadyLinked)
}

func TestCreateWorkOrder_DuplicateExtraRejected(t *testing.T) {
	src := mergeFixture()
	svc := workorderapp.NewService(newMergeStubRepo(), workorderapp.Deps{Merge: src})

	// The anchor repeated in the extras list would link the same item twice.
	_, err := createMergedWO(t, svc, 201, []int64{202, 202})
	require.ErrorIs(t, err, workorderdomain.ErrDuplicatePlanItemLink)
}
