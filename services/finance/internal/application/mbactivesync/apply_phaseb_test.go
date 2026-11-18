package mbactivesync

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectItemsPhaseBOrder(t *testing.T) {
	head := func(key string, kind HeadKind) *ApplyItem {
		return &ApplyItem{Kind: applyKindHead, Key: key, Action: string(kind), Phase: PhaseB}
	}
	items := []*ApplyItem{
		head("dh:1", KindDeactivateHead),
		head("suph:1", KindSupersedeHead),
		head("ds:1", KindDeactivateSpin),
		head("sup:2", KindSupersede),
		head("ds:2", KindDeactivateSpin),
		head("sup:1", KindSupersede),
		head("dh:2", KindDeactivateHead),
		{Kind: applyKindRow, Key: "row:9", Action: string(ActionKeep), Phase: PhaseA},
	}
	got := SelectItems(items, ApplyOptions{Phase: PhaseB})
	keys := make([]string, 0, len(got))
	for _, it := range got {
		keys = append(keys, it.Key)
	}
	// SUPERSEDE, DEACTIVATE_SPIN, SUPERSEDE_HEAD, DEACTIVATE_HEAD; plan order kept within a kind.
	assert.Equal(t, []string{"sup:2", "sup:1", "ds:1", "ds:2", "suph:1", "dh:1", "dh:2"}, keys)
}

func TestSelectItemsPhaseAKeepsOrder(t *testing.T) {
	items := []*ApplyItem{
		{Kind: applyKindHead, Key: "dh:1", Action: string(KindDeactivateHead), Phase: PhaseA},
		{Kind: applyKindHead, Key: "sup:1", Action: string(KindSupersede), Phase: PhaseA},
	}
	got := SelectItems(items, ApplyOptions{Phase: PhaseA})
	assert.Equal(t, "dh:1", got[0].Key)
	assert.Equal(t, "sup:1", got[1].Key)
}

type recWriter struct {
	fakeWriter
	got  []string
	sup  SupersedeInput
	suph SupersedeHeadInput
	rev  []RevertInput
	res  ItemResult
	err  error
}

func (r *recWriter) ret(n string) (ItemResult, error) {
	r.got = append(r.got, n)
	if r.res.Result == "" && r.err == nil {
		return ItemResult{Result: ResultApplied}, nil
	}
	return r.res, r.err
}
func (r *recWriter) SupersedeExisting(_ context.Context, in SupersedeInput) (ItemResult, error) {
	r.sup = in
	return r.ret("sup")
}
func (r *recWriter) SupersedeHeadOnly(_ context.Context, in SupersedeHeadInput) (ItemResult, error) {
	r.suph = in
	return r.ret("suph")
}
func (r *recWriter) DeactivateSpin(context.Context, DeactivateSpinInput) (ItemResult, error) {
	return r.ret("ds")
}
func (r *recWriter) DeactivateHead(context.Context, DeactivateHeadInput) (ItemResult, error) {
	return r.ret("dh")
}

func (r *recWriter) Revert(_ context.Context, in RevertInput) (ItemResult, error) {
	r.rev = append(r.rev, in)
	return r.ret("rev")
}

func bItem(kind HeadKind, oh, os, nh, ref string) *ApplyItem {
	return &ApplyItem{
		Kind: applyKindHead, Key: "k", Action: string(kind), Phase: PhaseB,
		Header: []string{"old_head_id", "old_spin_id", "new_head_id", "new_spin_ref"},
		Record: []string{oh, os, nh, ref},
	}
}

func TestDispatchPhaseB(t *testing.T) {
	a, b, c := uuid.New().String(), uuid.New().String(), uuid.New().String()
	cases := []struct {
		name       string
		it         *ApplyItem
		wantCall   string
		wantResult string
		wantReason string
	}{
		{"supersede", bItem(KindSupersede, a, b, c, "spin:"+c), "sup", ResultApplied, ""},
		{"supersede empty ref", bItem(KindSupersede, a, b, c, ""), "", ResultSkipped, "new_spin_not_ready"},
		{"supersede ln ref", bItem(KindSupersede, a, b, c, "ln:3"), "", ResultSkipped, "new_spin_not_ready"},
		{"supersede bad ref uuid", bItem(KindSupersede, a, b, c, "spin:xyz"), "", ResultFailed, ReasonBadPlanItem},
		{"supersede junk ref", bItem(KindSupersede, a, b, c, "foo"), "", ResultFailed, ReasonBadPlanItem},
		{"supersede bad old spin", bItem(KindSupersede, a, "nope", c, "spin:"+c), "", ResultFailed, ReasonBadPlanItem},
		{"supersede missing old spin", bItem(KindSupersede, a, "", c, "spin:"+c), "", ResultFailed, ReasonBadPlanItem},
		{"supersede head only", bItem(KindSupersedeHead, a, "", c, ""), "suph", ResultApplied, ""},
		{"supersede head bad new", bItem(KindSupersedeHead, a, "", "zz", ""), "", ResultFailed, ReasonBadPlanItem},
		{"deactivate spin", bItem(KindDeactivateSpin, a, b, "", ""), "ds", ResultApplied, ""},
		{"deactivate spin bad", bItem(KindDeactivateSpin, a, "x", "", ""), "", ResultFailed, ReasonBadPlanItem},
		{"deactivate head", bItem(KindDeactivateHead, a, "", "", ""), "dh", ResultApplied, ""},
		{"deactivate head bad", bItem(KindDeactivateHead, "q", "", "", ""), "", ResultFailed, ReasonBadPlanItem},
		{"report head", bItem(KindReportHead, a, "", "", ""), "", ResultSkipped, ReasonNotWired},
		{"unknown", bItem(HeadKind("WAT"), a, "", "", ""), "", ResultSkipped, ReasonNotWired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := &recWriter{}
			out := NewApplier(w).dispatch(context.Background(), tc.it, "me", newRunState(nil, nil))
			assert.Equal(t, tc.wantResult, out.Result.Result)
			assert.Equal(t, tc.wantReason, out.Result.Reason)
			if tc.wantCall == "" {
				assert.Empty(t, w.got)
			} else {
				assert.Equal(t, []string{tc.wantCall}, w.got)
			}
		})
	}
}

func TestDispatchPhaseBSupersedeFieldsAndErrors(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	w := &recWriter{}
	NewApplier(w).dispatch(context.Background(), bItem(KindSupersede, a.String(), b.String(), "", "spin:"+c.String()), "me", newRunState(nil, nil))
	assert.Equal(t, SupersedeInput{OldHeadID: a, OldSpinID: b, NewSpinID: c, Actor: "me"}, w.sup)

	w = &recWriter{err: errors.New("boom")}
	out := NewApplier(w).dispatch(context.Background(), bItem(KindDeactivateHead, a.String(), "", "", ""), "me", newRunState(nil, nil))
	assert.Equal(t, ResultFailed, out.Result.Result)
	assert.Equal(t, ReasonDBError, out.Result.Reason)
	assert.Equal(t, "boom", out.Err)

	empty := &emptyWriter{}
	out = NewApplier(empty).dispatch(context.Background(), bItem(KindDeactivateHead, a.String(), "", "", ""), "me", newRunState(nil, nil))
	assert.Equal(t, ResultFailed, out.Result.Result)
	assert.NotEmpty(t, out.Err)
}

type emptyWriter struct{ recWriter }

func (e *emptyWriter) DeactivateHead(context.Context, DeactivateHeadInput) (ItemResult, error) {
	return ItemResult{}, nil
}

func TestSelectItemsPhaseDOrder(t *testing.T) {
	head := func(key string, kind HeadKind) *ApplyItem {
		return &ApplyItem{Kind: applyKindHead, Key: key, Action: string(kind), Phase: PhaseD}
	}
	row := func(key string) *ApplyItem {
		return &ApplyItem{Kind: applyKindRow, Key: key, Action: string(ActionReactivate), Phase: PhaseD}
	}
	items := []*ApplyItem{head("suph:1", KindSupersedeHead), head("sup:1", KindSupersede), row("row:2"), row("row:1"),
		{Kind: applyKindRow, Key: "row:A", Action: string(ActionKeep), Phase: PhaseA}}
	var keys []string
	for _, it := range SelectItems(items, ApplyOptions{Phase: PhaseD}) {
		keys = append(keys, it.Key)
	}
	assert.Equal(t, []string{"row:2", "row:1", "sup:1", "suph:1"}, keys)
}

func TestDispatchPhaseDReactivateAndSupersede(t *testing.T) {
	w := &recWriter{}
	a := NewApplier(w)
	spin := uuid.New()
	row := &ApplyItem{Kind: applyKindRow, Key: "row:1", Action: string(ActionReactivate), Phase: PhaseD,
		Spin: SyncSpinInput{SpinID: spin, SubActions: []string{SubActSpin, SubActHead}}}
	out := a.dispatch(context.Background(), row, "me", newRunState(nil, nil))
	assert.Equal(t, ResultApplied, out.Result.Result)
	// Routing: REACTIVATE goes to SyncExistingSpin with ACT_SPIN|ACT_HEAD and the actor.
	require.Len(t, w.calls, 1)
	assert.Equal(t, spin, w.calls[0].SpinID)
	assert.Equal(t, []string{SubActSpin, SubActHead}, w.calls[0].SubActions)
	assert.Equal(t, "me", w.calls[0].Actor)
	assert.Empty(t, w.got, "no other writer method for REACTIVATE")

	oldHead, oldSpin, newHead, newSpin := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	it := bItem(KindSupersede, oldHead.String(), oldSpin.String(), newHead.String(), "spin:"+newSpin.String())
	it.Phase = PhaseD
	out = a.dispatch(context.Background(), it, "me", newRunState(nil, nil))
	assert.Equal(t, ResultApplied, out.Result.Result)
	assert.Equal(t, SupersedeInput{OldHeadID: oldHead, OldSpinID: oldSpin, NewSpinID: newSpin, Actor: "me"}, w.sup)

	it = bItem(KindSupersedeHead, oldHead.String(), "", newHead.String(), "")
	it.Phase = PhaseD
	out = a.dispatch(context.Background(), it, "me", newRunState(nil, nil))
	assert.Equal(t, ResultApplied, out.Result.Result)
	assert.Equal(t, SupersedeHeadInput{OldHeadID: oldHead, NewHeadID: newHead, Actor: "me"}, w.suph)
	assert.Equal(t, []string{"sup", "suph"}, w.got)

	// Deactivate kinds are not phase D actions.
	it = bItem(KindDeactivateHead, uuid.NewString(), "", "", "")
	it.Phase = PhaseD
	out = a.dispatch(context.Background(), it, "me", newRunState(nil, nil))
	assert.Equal(t, ReasonNotWired, out.Result.Reason)
	assert.Equal(t, []string{"sup", "suph"}, w.got)
}
