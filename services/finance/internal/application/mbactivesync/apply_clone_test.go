package mbactivesync

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cloneWriter struct {
	fakeWriter
	ins   []DuplicateTargetInput
	clone map[int]uuid.UUID
}

func (c *cloneWriter) DuplicateToTarget(_ context.Context, in DuplicateTargetInput) (ItemResult, error) {
	c.ins = append(c.ins, in)
	id := c.clone[in.Ln]
	if in.OnCloned != nil {
		if err := in.OnCloned(ItemResult{Result: ResultPartial, CloneSpinID: id.String()}); err != nil {
			return ItemResult{}, err
		}
	}
	return ItemResult{Result: ResultApplied, SpinID: id.String(), CloneSpinID: id.String()}, nil
}

func TestLoadPrevClones(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	f1 := "item_key,ln,clone_spin_id\nrow:1,1," + a.String() + "\nrow:2,2,\nsup:x,,\n"
	f2 := "ln,clone_spin_id\n1," + a.String() + "\n3," + b.String() + "\n"
	m, err := LoadPrevClones(strings.NewReader(f1), strings.NewReader(f2))
	require.NoError(t, err)
	assert.Equal(t, map[int]uuid.UUID{1: a, 3: b}, m)

	_, err = LoadPrevClones(strings.NewReader("ln,clone_spin_id\n1," + a.String() + "\n1," + b.String() + "\n"))
	require.Error(t, err)
	_, err = LoadPrevClones(strings.NewReader("ln\n1\n"))
	require.Error(t, err)
}

func TestApplierCloneRows(t *testing.T) {
	src, c1, c2, prevC := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	w := &cloneWriter{clone: map[int]uuid.UUID{1: c1, 2: c2, 5: prevC}}
	mk := func(ln int, action, role, source string) *ApplyItem {
		return &ApplyItem{Kind: applyKindRow, Key: "row:x", Ln: ln, SysID: "S", Action: action, Role: role, Phase: PhaseC, Source: source}
	}
	items := []*ApplyItem{
		mk(1, string(ActionGenerateSpin), string(RoleDup), "ln:9"), // root ln 9 created in an earlier run (prev)
		mk(2, string(ActionDuplicateSpin), "", src.String()),
		mk(3, string(ActionGenerateSpin), string(RoleDup), "ln:8"), // root not created yet
		mk(4, string(ActionDuplicateSpin), "", "nope"),
		mk(5, string(ActionDuplicateSpin), "", src.String()),
	}
	s := &memSink{}
	_, err := NewApplier(w).Run(t.Context(), items, ApplyOptions{Phase: PhaseC, Actor: "me", PrevClones: map[int]uuid.UUID{9: src, 5: prevC}}, s)
	require.NoError(t, err)
	require.Len(t, w.ins, 3)
	assert.Equal(t, src, w.ins[0].SourceSpinID)
	assert.Equal(t, "me", w.ins[0].Target.Actor)
	assert.ElementsMatch(t, []uuid.UUID{src, prevC}, w.ins[0].ExcludeClones)
	assert.ElementsMatch(t, []uuid.UUID{src, prevC, c1}, w.ins[1].ExcludeClones, "clone resolved earlier in the run is claimed")
	assert.Equal(t, prevC, w.ins[2].CloneSpinID)
	var results []string
	for _, o := range s.out {
		results = append(results, o.Result.Result+"/"+o.Result.Reason)
	}
	assert.Equal(t, []string{"PARTIAL/", "APPLIED/", "SKIPPED/" + ReasonSourceNotReady, "PARTIAL/", "APPLIED/",
		"FAILED/" + ReasonBadPlanItem, "PARTIAL/", "APPLIED/"}, results)
}

func TestCSVResultSinkManualItems(t *testing.T) {
	var rb, mb bytes.Buffer
	s := NewCSVResultSink(&rb, nil).WithManual(&mb)
	it := rowItem(7, "S7", string(ActionDuplicateSpin), PhaseC)
	require.NoError(t, s.Record(ApplyOutcome{Item: it, Result: ItemResult{Result: ResultApplied}}))
	require.NoError(t, s.Record(ApplyOutcome{Item: it, Result: ItemResult{Result: ResultFailed, Reason: ReasonPartialClone,
		CloneSpinID: "c", ManualReason: ReasonPartialCloneInactiveHead}}))
	require.NoError(t, s.Close())
	m, err := ReadPlanCSV(&mb, ManualItemsHeader)
	require.NoError(t, err)
	require.Len(t, m.Records, 1)
	assert.Equal(t, ReasonPartialCloneInactiveHead, m.Get(0, "reason"))
	assert.Equal(t, "c", m.Get(0, "clone_spin_id"))

	var empty bytes.Buffer
	require.NoError(t, NewCSVResultSink(&bytes.Buffer{}, nil).WithManual(&empty).Close())
	assert.True(t, strings.HasPrefix(empty.String(), "item_key,"))
}
