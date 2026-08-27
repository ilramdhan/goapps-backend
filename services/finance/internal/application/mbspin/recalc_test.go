// Package mbspin_test provides unit tests for MB Spin application layer handlers.
//
// ⛔ NO DATABASE. Every test here runs against in-memory fakes; anything that
// genuinely needs PostgreSQL belongs in an *_integration_test.go gated on
// INTEGRATION_TEST=true.
package mbspin_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbdozing"
	mbspindomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// --- fakes -------------------------------------------------------------------

// fakeRecalcRepo is an in-memory mbspin.RecalcRepository.
//
// children is keyed by parent id, so a test can build a GRANDPARENT -> PARENT ->
// CHILD tree and then prove the pass never descends into the third level: if the
// implementation recursed, the grandchild's key would be read.
type fakeRecalcRepo struct {
	children map[uuid.UUID][]*mbspindomain.Entity
	// readParents records every parent id ListAllChildren was called with. The
	// one-level guarantee (R13) is asserted against this.
	readParents []uuid.UUID
	applied     []mbspindomain.RecalcApplyInput
	applyErr    error
}

func (f *fakeRecalcRepo) ListAllChildren(_ context.Context, parentID uuid.UUID) ([]*mbspindomain.Entity, error) {
	f.readParents = append(f.readParents, parentID)
	return f.children[parentID], nil
}

func (f *fakeRecalcRepo) ApplyChildRecalc(_ context.Context, in mbspindomain.RecalcApplyInput) error {
	if f.applyErr != nil {
		return f.applyErr
	}
	f.applied = append(f.applied, in)
	return nil
}

// fakeImpactRepo returns a canned READ-ONLY product impact. It performs no
// calculation — mirroring the production repository, which only SELECTs.
type fakeImpactRepo struct {
	rows   []mbdozing.ImpactRow
	totals mbdozing.Totals
	calls  int
}

func (f *fakeImpactRepo) ImpactBySpin(_ context.Context, _ string, _ int) ([]mbdozing.ImpactRow, mbdozing.Totals, error) {
	f.calls++
	return f.rows, f.totals, nil
}

// --- helpers -----------------------------------------------------------------

func ptrF(v float64) *float64 { return &v }
func ptrI(v int) *int         { return &v }
func ptrS(v string) *string   { return &v }

// testNow is a fixed timestamp so reconstructed entities are deterministic.
func testNow() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }

// newSpin builds a spin with the denier/filament/dozing/status a recalc cares
// about. Reconstruct is used so the entity carries a caller-chosen id.
func newSpin(t *testing.T, id, headID uuid.UUID, name string, denier *float64, filament *int, dozing *float64, status *string) *mbspindomain.Entity {
	t.Helper()
	e := mbspindomain.Reconstruct(
		id, nil, nil, headID, name,
		denier, filament, dozing, nil,
		nil, nil,
		status, nil, nil, nil,
		nil, nil,
		true, testNow(), "tester", nil, nil, nil, nil,
	)
	return e
}

func rnd() *string { return ptrS(mbspindomain.StatusRnD) }

// --- tests -------------------------------------------------------------------

// TestRecalcApply_RnDChildIsRecalculated proves A6: an R&D child's dozing is
// rewritten with formula C-1.
func TestRecalcApply_RnDChildIsRecalculated(t *testing.T) {
	parentID, headID, childID := uuid.New(), uuid.New(), uuid.New()
	parent := newSpin(t, parentID, headID, "parent", ptrF(380), ptrI(108), ptrF(0.9), rnd())
	child := newSpin(t, childID, headID, "child", ptrF(500), ptrI(96), ptrF(0.5), rnd())

	rr := &fakeRecalcRepo{children: map[uuid.UUID][]*mbspindomain.Entity{parentID: {child}}}
	svc := mbspin.NewRecalcService(nil, rr, nil)

	res, err := svc.Apply(context.Background(), mbspin.ApplyInput{Parent: parent, Actor: "tester"})
	require.NoError(t, err)

	require.Len(t, res.Recalculated, 1)
	assert.Equal(t, childID, res.Recalculated[0].SpinID)
	// Golden value from mbdozing.ScaleLDR: 380/108 @ 0.9 -> 500/96.
	assert.InDelta(t, 0.7397296803562773, res.Recalculated[0].NewDozing, 1e-12)
	assert.Empty(t, res.Skipped)
}

// TestRecalcApply_NonRnDChildrenAreSkipped is the A7 case the user asked for:
// a child that is already ACTUAL must never be recalculated.
func TestRecalcApply_NonRnDChildrenAreSkipped(t *testing.T) {
	parentID, headID := uuid.New(), uuid.New()
	parent := newSpin(t, parentID, headID, "parent", ptrF(380), ptrI(108), ptrF(0.9), rnd())

	spinning := newSpin(t, uuid.New(), headID, "spinning", ptrF(500), ptrI(96), ptrF(0.5), ptrS("Spinning"))
	boughtout := newSpin(t, uuid.New(), headID, "boughtout", ptrF(500), ptrI(96), ptrF(0.5), ptrS("Boughtout"))
	noStatus := newSpin(t, uuid.New(), headID, "nostatus", ptrF(500), ptrI(96), ptrF(0.5), nil)
	emptyStatus := newSpin(t, uuid.New(), headID, "emptystatus", ptrF(500), ptrI(96), ptrF(0.5), ptrS(""))

	rr := &fakeRecalcRepo{children: map[uuid.UUID][]*mbspindomain.Entity{
		parentID: {spinning, boughtout, noStatus, emptyStatus},
	}}
	svc := mbspin.NewRecalcService(nil, rr, nil)

	res, err := svc.Apply(context.Background(), mbspin.ApplyInput{Parent: parent, Actor: "tester"})
	require.NoError(t, err)

	assert.Empty(t, res.Recalculated, "an actual child must never be recalculated")
	require.Len(t, res.Skipped, 4)

	byName := map[string]string{}
	for _, s := range res.Skipped {
		byName[s.MgtName] = s.Reason
	}
	assert.Equal(t, mbspindomain.SkipReasonStatusNotRnD, byName["spinning"])
	assert.Equal(t, mbspindomain.SkipReasonStatusNotRnD, byName["boughtout"])
	assert.Equal(t, mbspindomain.SkipReasonStatusAbsent, byName["nostatus"])
	assert.Equal(t, mbspindomain.SkipReasonStatusAbsent, byName["emptystatus"])

	// The write still happened (one audit row), but it carried no child updates.
	require.Len(t, rr.applied, 1)
	assert.Empty(t, rr.applied[0].Updates)
}

// TestRecalcApply_OneLevelOnly proves R13: a grandchild never participates.
func TestRecalcApply_OneLevelOnly(t *testing.T) {
	parentID, headID := uuid.New(), uuid.New()
	childID, grandchildID := uuid.New(), uuid.New()

	parent := newSpin(t, parentID, headID, "parent", ptrF(380), ptrI(108), ptrF(0.9), rnd())
	child := newSpin(t, childID, headID, "child", ptrF(500), ptrI(96), ptrF(0.5), rnd())
	grandchild := newSpin(t, grandchildID, headID, "grandchild", ptrF(600), ptrI(72), ptrF(0.4), rnd())

	rr := &fakeRecalcRepo{children: map[uuid.UUID][]*mbspindomain.Entity{
		parentID: {child},
		childID:  {grandchild},
	}}
	svc := mbspin.NewRecalcService(nil, rr, nil)

	res, err := svc.Apply(context.Background(), mbspin.ApplyInput{Parent: parent, Actor: "tester"})
	require.NoError(t, err)

	require.Len(t, res.Recalculated, 1)
	assert.Equal(t, childID, res.Recalculated[0].SpinID)
	// The decisive assertion: the child's own children were never read, so no
	// recursion took place.
	assert.Equal(t, []uuid.UUID{parentID}, rr.readParents)
	require.Len(t, rr.applied, 1)
	require.Len(t, rr.applied[0].Updates, 1)
	assert.NotEqual(t, grandchildID, rr.applied[0].Updates[0].SpinID)
}

// TestRecalcApply_TooManyChildren proves the fan-out cap (>20 candidates).
func TestRecalcApply_TooManyChildren(t *testing.T) {
	parentID, headID := uuid.New(), uuid.New()
	parent := newSpin(t, parentID, headID, "parent", ptrF(380), ptrI(108), ptrF(0.9), rnd())

	kids := make([]*mbspindomain.Entity, 0, mbspindomain.MaxRecalcChildren+1)
	for i := 0; i <= mbspindomain.MaxRecalcChildren; i++ {
		kids = append(kids, newSpin(t, uuid.New(), headID, "child", ptrF(500), ptrI(96), ptrF(0.5), rnd()))
	}
	rr := &fakeRecalcRepo{children: map[uuid.UUID][]*mbspindomain.Entity{parentID: kids}}
	svc := mbspin.NewRecalcService(nil, rr, nil)

	res, err := svc.Apply(context.Background(), mbspin.ApplyInput{Parent: parent, Actor: "tester"})

	assert.Nil(t, res)
	assert.ErrorIs(t, err, mbspindomain.ErrTooManyChildren)
	assert.Empty(t, rr.applied, "a rejected fan-out must write nothing")
}

// TestRecalcApply_ExactlyAtCapIsAllowed pins the boundary: 20 is fine, 21 is not.
func TestRecalcApply_ExactlyAtCapIsAllowed(t *testing.T) {
	parentID, headID := uuid.New(), uuid.New()
	parent := newSpin(t, parentID, headID, "parent", ptrF(380), ptrI(108), ptrF(0.9), rnd())

	kids := make([]*mbspindomain.Entity, 0, mbspindomain.MaxRecalcChildren)
	for i := 0; i < mbspindomain.MaxRecalcChildren; i++ {
		kids = append(kids, newSpin(t, uuid.New(), headID, "child", ptrF(500), ptrI(96), ptrF(0.5), rnd()))
	}
	rr := &fakeRecalcRepo{children: map[uuid.UUID][]*mbspindomain.Entity{parentID: kids}}
	svc := mbspin.NewRecalcService(nil, rr, nil)

	res, err := svc.Apply(context.Background(), mbspin.ApplyInput{Parent: parent, Actor: "tester"})
	require.NoError(t, err)
	assert.Len(t, res.Recalculated, mbspindomain.MaxRecalcChildren)
}

// TestRecalcApply_SingleWorkflowLogPerOperation proves the audit trail is
// per-OPERATION, not per-child, and carries the K-18 mbwl_meta document.
func TestRecalcApply_SingleWorkflowLogPerOperation(t *testing.T) {
	parentID, headID := uuid.New(), uuid.New()
	kids := []*mbspindomain.Entity{
		newSpin(t, uuid.New(), headID, "c1", ptrF(500), ptrI(96), ptrF(0.5), rnd()),
		newSpin(t, uuid.New(), headID, "c2", ptrF(450), ptrI(72), ptrF(0.5), rnd()),
		newSpin(t, uuid.New(), headID, "c3", ptrF(600), ptrI(144), ptrF(0.5), rnd()),
	}
	rr := &fakeRecalcRepo{children: map[uuid.UUID][]*mbspindomain.Entity{parentID: kids}}
	// The impact repo is READ-ONLY; its counts land in mbwl_meta as a PREVIEW.
	parentWithCode := mbspindomain.Reconstruct(
		parentID, nil, ptrS("ORION-1"), headID, "parent",
		ptrF(380), ptrI(108), ptrF(1.1), nil, nil, nil,
		rnd(), nil, nil, nil, nil, nil,
		true, testNow(), "tester", nil, nil, nil, nil,
	)
	ir := &fakeImpactRepo{totals: mbdozing.Totals{TotalAffected: 7, TotalLocked: 2}}
	svc := mbspin.NewRecalcService(nil, rr, ir)

	res, err := svc.Apply(context.Background(), mbspin.ApplyInput{
		Parent: parentWithCode, OldDozing: ptrF(0.9), Actor: "tester",
	})
	require.NoError(t, err)
	require.Len(t, res.Recalculated, 3)

	require.Len(t, rr.applied, 1, "three children must still produce exactly ONE audit row")
	in := rr.applied[0]
	assert.Equal(t, headID, in.HeadID)
	assert.Equal(t, parentID, in.ParentSpinID)

	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(in.LogMeta), &meta))
	assert.Equal(t, "DOZING_CHANGED", meta["event"])
	assert.InDelta(t, 0.9, meta["old"], 1e-12)
	assert.InDelta(t, 1.1, meta["new"], 1e-12)
	assert.InDelta(t, float64(7), meta["affected_products"], 1e-12)
	assert.InDelta(t, float64(2), meta["locked"], 1e-12)
}

// TestRecalcApply_IncompleteOperandsAreNotWritten proves a child with a missing
// operand is left alone rather than written with a guessed number — and that it
// does NOT enter the two-value skip vocabulary.
func TestRecalcApply_IncompleteOperandsAreNotWritten(t *testing.T) {
	parentID, headID := uuid.New(), uuid.New()
	parent := newSpin(t, parentID, headID, "parent", ptrF(380), ptrI(108), ptrF(0.9), rnd())
	noDenier := newSpin(t, uuid.New(), headID, "no-denier", nil, ptrI(96), ptrF(0.5), rnd())

	rr := &fakeRecalcRepo{children: map[uuid.UUID][]*mbspindomain.Entity{parentID: {noDenier}}}
	svc := mbspin.NewRecalcService(nil, rr, nil)

	res, err := svc.Apply(context.Background(), mbspin.ApplyInput{Parent: parent, Actor: "tester"})
	require.NoError(t, err)

	assert.Empty(t, res.Recalculated)
	assert.Empty(t, res.Skipped)
	assert.Len(t, res.Incomplete, 1)
}

// TestRecalcPreview_WritesNothing proves the duplicate path's preview is
// read-only.
func TestRecalcPreview_WritesNothing(t *testing.T) {
	parentID, headID := uuid.New(), uuid.New()
	parent := newSpin(t, parentID, headID, "parent", ptrF(380), ptrI(108), ptrF(0.9), rnd())
	actual := newSpin(t, uuid.New(), headID, "actual", ptrF(500), ptrI(96), ptrF(0.5), ptrS("Spinning"))

	rr := &fakeRecalcRepo{children: map[uuid.UUID][]*mbspindomain.Entity{parentID: {actual}}}
	svc := mbspin.NewRecalcService(nil, rr, nil)

	res, err := svc.Preview(context.Background(), parent)
	require.NoError(t, err)
	assert.Empty(t, res.Recalculated)
	assert.Len(t, res.Skipped, 1)
	assert.Empty(t, rr.applied, "preview must not write")
}

// --- update hook: trigger conditions ------------------------------------------

// newUpdateFixture wires an UpdateHandler onto a real RecalcService whose child
// reads and writes are served by fakes, so the TRIGGER decision can be observed
// without a database.
func newUpdateFixture(t *testing.T, stored *mbspindomain.Entity, children []*mbspindomain.Entity) (*mbspin.UpdateHandler, *MockRepository, *fakeRecalcRepo) {
	t.Helper()
	repo := new(MockRepository)
	repo.On("GetByID", mock.Anything, stored.ID()).Return(stored, nil)
	repo.On("Update", mock.Anything, mock.AnythingOfType("*mbspin.Entity")).Return(nil)

	rr := &fakeRecalcRepo{children: map[uuid.UUID][]*mbspindomain.Entity{stored.ID(): children}}
	return mbspin.NewUpdateHandlerWithRecalc(repo, mbspin.NewRecalcService(repo, rr, nil)), repo, rr
}

// TestUpdateHandler_RecalcNotTriggeredByNonDozingField is the negative case the
// user called out: renaming a spin must not cascade into its children.
func TestUpdateHandler_RecalcNotTriggeredByNonDozingField(t *testing.T) {
	parentID, headID := uuid.New(), uuid.New()
	parent := newSpin(t, parentID, headID, "parent", ptrF(380), ptrI(108), ptrF(0.9), rnd())
	child := newSpin(t, uuid.New(), headID, "child", ptrF(500), ptrI(96), ptrF(0.5), rnd())

	h, _, rr := newUpdateFixture(t, parent, []*mbspindomain.Entity{child})

	newName := "renamed"
	res, err := h.HandleWithRecalc(context.Background(), mbspin.UpdateCommand{
		ID: parentID, MgtName: &newName, UpdatedBy: "tester",
	})
	require.NoError(t, err)
	assert.Nil(t, res.Recalc, "a non-dozing field must not start a recalc")
	assert.Empty(t, rr.readParents, "children must not even be read")
	assert.Empty(t, rr.applied)
}

// TestUpdateHandler_RecalcNotTriggeredWhenValueUnchanged guards the idempotent
// PUT: resending the same dozing is not a CHANGE.
func TestUpdateHandler_RecalcNotTriggeredWhenValueUnchanged(t *testing.T) {
	parentID, headID := uuid.New(), uuid.New()
	parent := newSpin(t, parentID, headID, "parent", ptrF(380), ptrI(108), ptrF(0.9), rnd())
	child := newSpin(t, uuid.New(), headID, "child", ptrF(500), ptrI(96), ptrF(0.5), rnd())

	h, _, rr := newUpdateFixture(t, parent, []*mbspindomain.Entity{child})

	res, err := h.HandleWithRecalc(context.Background(), mbspin.UpdateCommand{
		ID: parentID, Denier: ptrF(380), Filament: ptrI(108), Dozing: ptrF(0.9), UpdatedBy: "tester",
	})
	require.NoError(t, err)
	assert.Nil(t, res.Recalc, "resending identical values is not a change")
	assert.Empty(t, rr.applied)
}

// TestUpdateHandler_RecalcTriggeredByDozingFields proves each of the three
// trigger fields fires the cascade on its own.
func TestUpdateHandler_RecalcTriggeredByDozingFields(t *testing.T) {
	cases := []struct {
		name string
		cmd  mbspin.UpdateCommand
	}{
		{"denier changed", mbspin.UpdateCommand{Denier: ptrF(420)}},
		{"filament changed", mbspin.UpdateCommand{Filament: ptrI(144)}},
		{"dozing changed", mbspin.UpdateCommand{Dozing: ptrF(1.4)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parentID, headID := uuid.New(), uuid.New()
			parent := newSpin(t, parentID, headID, "parent", ptrF(380), ptrI(108), ptrF(0.9), rnd())
			child := newSpin(t, uuid.New(), headID, "child", ptrF(500), ptrI(96), ptrF(0.5), rnd())

			h, _, rr := newUpdateFixture(t, parent, []*mbspindomain.Entity{child})

			cmd := tc.cmd
			cmd.ID = parentID
			cmd.UpdatedBy = "tester"
			res, err := h.HandleWithRecalc(context.Background(), cmd)
			require.NoError(t, err)

			require.NotNil(t, res.Recalc)
			assert.Len(t, res.Recalc.Recalculated, 1)
			require.Len(t, rr.applied, 1)
			assert.Equal(t, "tester", rr.applied[0].Actor)
		})
	}
}
