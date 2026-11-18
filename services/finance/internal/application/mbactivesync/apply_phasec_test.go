package mbactivesync

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeHW struct {
	res     map[uuid.UUID]ItemResult
	confirm map[uuid.UUID]bool
	calls   []uuid.UUID
}

func (f *fakeHW) EnsureValidated(_ context.Context, h uuid.UUID, _ string, c bool) (ItemResult, error) {
	f.calls = append(f.calls, h)
	f.confirm[h] = c
	return f.res[h], nil
}

type orderWriter struct {
	cloneWriter
	order []string
	roots []GenerateRootInput
	genID uuid.UUID
	syncs []SyncSpinInput
	sups  []SupersedeInput
}

func (o *orderWriter) GenerateRootSpin(_ context.Context, in GenerateRootInput) (ItemResult, error) {
	o.order = append(o.order, "genroot")
	o.roots = append(o.roots, in)
	return ItemResult{Result: ResultApplied, SpinID: o.genID.String()}, nil
}

func (o *orderWriter) SyncExistingSpin(ctx context.Context, in SyncSpinInput) (ItemResult, error) {
	o.order = append(o.order, "sync")
	o.syncs = append(o.syncs, in)
	return o.fakeWriter.SyncExistingSpin(ctx, in)
}

func (o *orderWriter) DuplicateToTarget(ctx context.Context, in DuplicateTargetInput) (ItemResult, error) {
	o.order = append(o.order, "dup")
	return o.cloneWriter.DuplicateToTarget(ctx, in)
}

func (o *orderWriter) SupersedeExisting(_ context.Context, in SupersedeInput) (ItemResult, error) {
	o.order = append(o.order, "sup")
	o.sups = append(o.sups, in)
	return ItemResult{Result: ResultApplied}, nil
}

func supItem(oldSpin uuid.UUID, ref string) *ApplyItem {
	return &ApplyItem{
		Kind: applyKindHead, Key: "sup:" + ref, Action: string(KindSupersede), Phase: PhaseC,
		Header: []string{"old_spin_id", "new_spin_ref"}, Record: []string{oldSpin.String(), ref},
	}
}

func cRow(ln int, action Action, role Role, head uuid.UUID, source string) *ApplyItem {
	return &ApplyItem{Kind: applyKindRow, Key: "row", Ln: ln, SysID: "S", Action: string(action), Role: string(role),
		Phase: PhaseC, Source: source, HeadID: head}
}

func TestSelectItemsPhaseCOrder(t *testing.T) {
	src := uuid.New().String()
	items := []*ApplyItem{
		supItem(uuid.New(), "ln:1"),
		cRow(4, ActionDuplicateSpin, "", uuid.Nil, src),
		cRow(3, ActionGenerateSpin, RoleDup, uuid.Nil, "ln:1"),
		{Kind: applyKindHead, Key: "sh", Action: string(KindSupersedeHead), Phase: PhaseC},
		cRow(1, ActionGenerateSpin, RoleRoot, uuid.New(), ""),
	}
	sel := SelectItems(items, ApplyOptions{Phase: PhaseC})
	var got []int
	for _, it := range sel {
		got = append(got, phaseCRank(it))
	}
	assert.Equal(t, []int{phaseCRankRoot, phaseCRankDup, phaseCRankDuplicate, phaseCRankSupersede, phaseCRankSupersedeHead}, got)
}

func TestApplierPhaseCFlow(t *testing.T) {
	head, root, c1, c2 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	oldSpin := uuid.New()
	w := &orderWriter{cloneWriter: cloneWriter{clone: map[int]uuid.UUID{2: c1, 3: c2}}}
	hw := &fakeHW{confirm: map[uuid.UUID]bool{}, res: map[uuid.UUID]ItemResult{head: {Result: ResultApplied, SpinID: root.String()}}}
	items := []*ApplyItem{
		supItem(oldSpin, "ln:1"),
		cRow(3, ActionDuplicateSpin, "", uuid.Nil, "ln:1"),
		cRow(2, ActionGenerateSpin, RoleDup, uuid.Nil, "ln:1"),
		cRow(1, ActionGenerateSpin, RoleRoot, head, ""),
	}
	s := &memSink{}
	sum, err := NewApplier(w).WithHeadWorkflow(hw).Run(t.Context(), items, ApplyOptions{Phase: PhaseC, Actor: "me", Batch: 2}, s)
	require.NoError(t, err)
	assert.Equal(t, 4, sum.Processed)
	assert.Equal(t, []string{"sync", "dup", "dup", "sup"}, w.order)
	assert.True(t, hw.confirm[head], "paired with a phase C SUPERSEDE -> confirmSupersede")
	require.Len(t, w.syncs, 1)
	assert.Equal(t, root, w.syncs[0].SpinID)
	require.Len(t, w.ins, 2)
	assert.Equal(t, root, w.ins[0].SourceSpinID, "ln:1 resolved to the new root spin")
	assert.Equal(t, root, w.ins[1].SourceSpinID)
	require.Len(t, w.sups, 1)
	assert.Equal(t, root, w.sups[0].NewSpinID)
	assert.Equal(t, oldSpin, w.sups[0].OldSpinID)
	for _, o := range s.out {
		assert.NotEqual(t, ResultFailed, o.Result.Result, o.Err)
	}
}

func TestApplierPhaseCRootCases(t *testing.T) {
	head, gen := uuid.New(), uuid.New()
	t.Run("no workflow wired", func(t *testing.T) {
		w := &orderWriter{}
		s := &memSink{}
		_, err := NewApplier(w).Run(t.Context(), []*ApplyItem{cRow(1, ActionGenerateSpin, RoleRoot, head, "")}, ApplyOptions{Phase: PhaseC, Actor: "me"}, s)
		require.NoError(t, err)
		assert.Equal(t, ReasonNotWired, s.out[0].Result.Reason)
	})
	t.Run("missing head id", func(t *testing.T) {
		w := &orderWriter{}
		s := &memSink{}
		hw := &fakeHW{confirm: map[uuid.UUID]bool{}}
		_, err := NewApplier(w).WithHeadWorkflow(hw).Run(t.Context(), []*ApplyItem{cRow(1, ActionGenerateSpin, RoleRoot, uuid.Nil, "")}, ApplyOptions{Phase: PhaseC, Actor: "me"}, s)
		require.NoError(t, err)
		assert.Equal(t, ReasonBadPlanItem, s.out[0].Result.Reason)
		assert.Empty(t, hw.calls)
	})
	t.Run("cost product exists without spin -> GenerateRootSpin (K7)", func(t *testing.T) {
		w := &orderWriter{genID: gen}
		s := &memSink{}
		hw := &fakeHW{confirm: map[uuid.UUID]bool{}, res: map[uuid.UUID]ItemResult{head: {Result: ResultSkipped, Reason: ReasonAlreadyInTarget}}}
		_, err := NewApplier(w).WithHeadWorkflow(hw).Run(t.Context(), []*ApplyItem{cRow(1, ActionGenerateSpin, RoleRoot, head, "")}, ApplyOptions{Phase: PhaseC, Actor: "me"}, s)
		require.NoError(t, err)
		assert.Equal(t, []string{"genroot", "sync"}, w.order)
		assert.Equal(t, gen, w.syncs[0].SpinID)
		assert.False(t, hw.confirm[head])
	})
	t.Run("validate gate failure stops the row and DUP waits", func(t *testing.T) {
		w := &orderWriter{}
		s := &memSink{}
		hw := &fakeHW{confirm: map[uuid.UUID]bool{}, res: map[uuid.UUID]ItemResult{head: {Result: ResultFailed, Reason: ReasonValidateGate, Message: "x"}}}
		items := []*ApplyItem{cRow(1, ActionGenerateSpin, RoleRoot, head, ""), cRow(2, ActionGenerateSpin, RoleDup, uuid.Nil, "ln:1")}
		_, err := NewApplier(w).WithHeadWorkflow(hw).Run(t.Context(), items, ApplyOptions{Phase: PhaseC, Actor: "me"}, s)
		require.NoError(t, err)
		require.Len(t, s.out, 2)
		assert.Equal(t, ResultFailed, s.out[0].Result.Result)
		assert.Equal(t, ReasonValidateGate, s.out[0].Result.Reason)
		assert.Equal(t, ReasonSourceNotReady, s.out[1].Result.Reason)
		assert.Empty(t, w.order)
	})
}

func TestResolveNewSpinLn(t *testing.T) {
	st := newRunState(nil, nil)
	id := uuid.New()
	_, skip, err := st.resolveNewSpin("ln:7")
	require.NoError(t, err)
	assert.True(t, skip)
	st.record(7, id)
	got, skip, err := st.resolveNewSpin("ln:7")
	require.NoError(t, err)
	assert.False(t, skip)
	assert.Equal(t, id, got)
	_, skip, err = st.resolveNewSpin("")
	require.NoError(t, err)
	assert.True(t, skip)
	_, _, err = st.resolveNewSpin("ln:x")
	require.Error(t, err)
	_, _, err = st.resolveNewSpin("bogus")
	require.Error(t, err)
}

func TestApplierRootKeepsGenerationImage(t *testing.T) {
	head, root := uuid.New(), uuid.New()
	w := &orderWriter{}
	hw := &fakeHW{confirm: map[uuid.UUID]bool{}, res: map[uuid.UUID]ItemResult{head: {
		Result: ResultApplied, SpinID: root.String(), Message: "validated",
		AfterJSON: `{"supersede_batch_id":"B1","deactivated_head_ids":["h1"]}`,
	}}}
	s := &memSink{}
	_, err := NewApplier(w).WithHeadWorkflow(hw).Run(t.Context(), []*ApplyItem{cRow(1, ActionGenerateSpin, RoleRoot, head, "")}, ApplyOptions{Phase: PhaseC, Actor: "me"}, s)
	require.NoError(t, err)
	require.Len(t, s.out, 1)
	assert.Contains(t, s.out[0].Result.AfterJSON, "supersede_batch_id")
	assert.Contains(t, s.out[0].Result.AfterJSON, "generate")
	assert.Contains(t, s.out[0].Result.Message, "validated")
}

func TestApplierRootBadSpinID(t *testing.T) {
	head := uuid.New()
	hw := &fakeHW{confirm: map[uuid.UUID]bool{}, res: map[uuid.UUID]ItemResult{head: {Result: ResultApplied, SpinID: "not-a-uuid"}}}
	s := &memSink{}
	_, err := NewApplier(&orderWriter{}).WithHeadWorkflow(hw).Run(t.Context(), []*ApplyItem{cRow(1, ActionGenerateSpin, RoleRoot, head, "")}, ApplyOptions{Phase: PhaseC, Actor: "me"}, s)
	require.NoError(t, err)
	assert.Equal(t, ResultFailed, s.out[0].Result.Result)
	assert.Equal(t, ReasonDBError, s.out[0].Result.Reason)
}

func TestLoadPrevRootsResolveWithoutRootRow(t *testing.T) {
	root, clone, oldSpin := uuid.New(), uuid.New(), uuid.New()
	csv := "item_key,ln,sysid,action,phase,result,reason,spin_id,clone_spin_id\n" +
		"row:1,1,S,GENERATE_SPIN,C,APPLIED,," + root.String() + ",\n" +
		"row:2,2,S,GENERATE_SPIN,C,APPLIED,," + clone.String() + "," + clone.String() + "\n"
	roots, err := LoadPrevRoots(strings.NewReader(csv))
	require.NoError(t, err)
	assert.Equal(t, map[int]uuid.UUID{1: root}, roots)

	w := &orderWriter{cloneWriter: cloneWriter{clone: map[int]uuid.UUID{3: uuid.New()}}}
	items := []*ApplyItem{supItem(oldSpin, "ln:1"), cRow(3, ActionDuplicateSpin, "", uuid.Nil, "ln:1")}
	s := &memSink{}
	_, err = NewApplier(w).Run(t.Context(), items, ApplyOptions{Phase: PhaseC, Actor: "me", PrevRoots: roots}, s)
	require.NoError(t, err)
	require.Len(t, w.sups, 1)
	assert.Equal(t, root, w.sups[0].NewSpinID)
	require.Len(t, w.ins, 1)
	assert.Equal(t, root, w.ins[0].SourceSpinID)
}

// P2h-4c: the ROOT sync carries the row spec (target_den/fil/cs) so the writer moves the
// generated root off the head spec.
func TestApplierRootSyncCarriesRowSpec(t *testing.T) {
	head, gen := uuid.New(), uuid.New()
	w := &orderWriter{genID: gen}
	s := &memSink{}
	hw := &fakeHW{confirm: map[uuid.UUID]bool{}, res: map[uuid.UUID]ItemResult{head: {Result: ResultSkipped, Reason: ReasonAlreadyInTarget}}}
	it := cRow(1, ActionGenerateSpin, RoleRoot, head, "")
	den, fil := 380.0, 144
	it.TargetDen, it.TargetFil = &den, &fil
	it.Spin.TargetCS = "TBL"
	_, err := NewApplier(w).WithHeadWorkflow(hw).Run(t.Context(), []*ApplyItem{it}, ApplyOptions{Phase: PhaseC, Actor: "me"}, s)
	require.NoError(t, err)
	require.Len(t, w.syncs, 1)
	in := w.syncs[0]
	require.True(t, in.SyncSpec())
	assert.InDelta(t, 380, *in.TargetDen, 1e-9)
	assert.Equal(t, 144, *in.TargetFil)
	assert.Equal(t, "TBL", in.TargetCS)
}
