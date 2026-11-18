package mbactivesync

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeWriter struct {
	calls  []SyncSpinInput
	fn     func(n int, in SyncSpinInput) (ItemResult, error)
	cancel func(n int)
}

func (f *fakeWriter) SyncExistingSpin(_ context.Context, in SyncSpinInput) (ItemResult, error) {
	f.calls = append(f.calls, in)
	n := len(f.calls)
	if f.cancel != nil {
		f.cancel(n)
	}
	if f.fn != nil {
		return f.fn(n, in)
	}
	return ItemResult{Result: ResultApplied, SpinID: in.SpinID.String(), BeforeJSON: "{}", AfterJSON: "{\"a\":1}"}, nil
}

func (f *fakeWriter) DuplicateToTarget(context.Context, DuplicateTargetInput) (ItemResult, error) {
	return ItemResult{Result: ResultSkipped, Reason: ReasonNotWired}, nil
}

func (f *fakeWriter) GenerateRootSpin(context.Context, GenerateRootInput) (ItemResult, error) {
	return ItemResult{Result: ResultSkipped, Reason: ReasonNotWired}, nil
}

func (f *fakeWriter) SupersedeExisting(context.Context, SupersedeInput) (ItemResult, error) {
	return ItemResult{Result: ResultSkipped, Reason: ReasonNotWired}, nil
}

func (f *fakeWriter) SupersedeHeadOnly(context.Context, SupersedeHeadInput) (ItemResult, error) {
	return ItemResult{Result: ResultSkipped, Reason: ReasonNotWired}, nil
}

func (f *fakeWriter) DeactivateSpin(context.Context, DeactivateSpinInput) (ItemResult, error) {
	return ItemResult{Result: ResultSkipped, Reason: ReasonNotWired}, nil
}

func (f *fakeWriter) DeactivateHead(context.Context, DeactivateHeadInput) (ItemResult, error) {
	return ItemResult{Result: ResultSkipped, Reason: ReasonNotWired}, nil
}

func (f *fakeWriter) Revert(context.Context, RevertInput) (ItemResult, error) {
	return ItemResult{Result: ResultSkipped, Reason: ReasonNotWired}, nil
}

type memSink struct {
	out []ApplyOutcome
	err error
}

func (m *memSink) Record(o ApplyOutcome) error {
	if m.err != nil {
		return m.err
	}
	m.out = append(m.out, o)
	return nil
}

func rowItem(ln int, sysid, action string, ph Phase) *ApplyItem {
	return &ApplyItem{
		Kind: applyKindRow, Key: "row:" + string(rune('0'+ln)), Ln: ln, SysID: sysid, Action: action, Phase: ph,
		Header: []string{"ln", "decision"}, Record: []string{"x", "APPLY"},
		Spin: SyncSpinInput{SpinID: uuid.New(), SubActions: []string{"SET_LDR"}},
	}
}

func TestLoadApplyItemsAndSelect(t *testing.T) {
	rows := "ln;sysid;action;phase;sub_actions;item_key;decision;head_id;spin_id;source_spin_id;target_fg;target_shade_code;target_shade_name;target_den;target_fil;target_cs;target_ldr\n"
	id := uuid.New().String()
	rows = strings.ReplaceAll(rows, ";", ",")
	rows += "2,S2,KEEP,A,SET_LDR;ACT_SPIN," + "row:2,APPLY,," + id + ",,12,SC,Shade,,,CS,1.5\n"
	rows += "1,S1,UPDATE_SHADE,A,SET_SHADE,row:1,APPLY,," + id + ",,,SC,Shade,,,,\n"
	rows += "3,S1,KEEP,A,,row:3,REVIEW,," + id + ",,,,,,,,\n"
	rows += "4,S3,REACTIVATE,D,ACT_SPIN,row:4,APPLY,," + id + ",,,,,,,,\n"
	rows += "x,S4,KEEP,A,,row:5,APPLY,,,,,,,,,,\n"
	heads := "item_key,kind,old_head_id,old_spin_id,new_head_id,new_spin_ref,phase,decision\nsup:a:b,SUPERSEDE,,,,,B,APPLY\n"
	rc, err := ReadPlanCSV(strings.NewReader(rows), RowsRequiredColumns)
	require.NoError(t, err)
	hc, err := ReadPlanCSV(strings.NewReader(heads), HeadsRequiredColumns)
	require.NoError(t, err)
	items, err := LoadApplyItems(rc, hc)
	require.NoError(t, err)
	require.Len(t, items, 5) // 4 APPLY rows + 1 head

	selA := SelectItems(items, ApplyOptions{Phase: PhaseA})
	require.Len(t, selA, 3)
	assert.Equal(t, []int{1, 2, 0}, []int{selA[0].Ln, selA[1].Ln, selA[2].Ln}) // sysid order; bad ln last in S4
	assert.Equal(t, []string{"SET_LDR", "ACT_SPIN"}, selA[1].Spin.SubActions)
	require.NotNil(t, selA[1].Spin.TargetFG)
	assert.EqualValues(t, 12, *selA[1].Spin.TargetFG)
	assert.InDelta(t, 1.5, *selA[1].Spin.TargetLDR, 1e-9)
	assert.NotEmpty(t, selA[2].bad)
	assert.Len(t, SelectItems(items, ApplyOptions{Phase: PhaseA, OnlySysID: []string{"S2"}}), 1)
	assert.Len(t, SelectItems(items, ApplyOptions{Phase: PhaseB}), 1)
}

func TestApplierRun(t *testing.T) {
	ctx := t.Context()
	items := []*ApplyItem{
		rowItem(1, "S1", "KEEP", PhaseA), rowItem(2, "S2", "UPDATE_LDR", PhaseA),
		rowItem(3, "S3", "UPDATE_SHADE", PhaseA), rowItem(4, "S4", "REACTIVATE", PhaseD),
	}
	t.Run("continue on error and actor", func(t *testing.T) {
		w := &fakeWriter{fn: func(n int, in SyncSpinInput) (ItemResult, error) {
			switch n {
			case 1:
				return ItemResult{}, errors.New("boom")
			case 2:
				return ItemResult{Result: ResultSkipped, Reason: ReasonAlreadyInTarget}, nil
			}
			return ItemResult{Result: ResultApplied}, nil
		}}
		s := &memSink{}
		sum, err := NewApplier(w).Run(ctx, items, ApplyOptions{Phase: PhaseA, Actor: "bob"}, s)
		require.NoError(t, err)
		require.Len(t, w.calls, 3)
		assert.Equal(t, "bob", w.calls[0].Actor)
		assert.Equal(t, 3, sum.Processed)
		assert.Equal(t, map[string]int{ResultFailed: 1, ResultSkipped: 1, ResultApplied: 1}, sum.ByResult)
		assert.Equal(t, ResultFailed, s.out[0].Result.Result)
		assert.Equal(t, ReasonDBError, s.out[0].Result.Reason)
		assert.Contains(t, s.out[0].Err, "boom")
	})
	t.Run("limit and batch", func(t *testing.T) {
		w := &fakeWriter{}
		var logs []string
		sum, err := NewApplier(w).Run(ctx, items, ApplyOptions{Phase: PhaseA, Actor: "a", Limit: 2, Batch: 2,
			Logf: func(f string, a ...any) { logs = append(logs, f) }}, &memSink{})
		require.NoError(t, err)
		assert.Equal(t, 2, sum.Processed)
		assert.Equal(t, "limit", sum.Stopped)
		assert.Len(t, w.calls, 2)
	})
	t.Run("ctx cancel stops", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		w := &fakeWriter{cancel: func(n int) {
			if n == 1 {
				cancel()
			}
		}}
		s := &memSink{}
		sum, err := NewApplier(w).Run(cctx, items, ApplyOptions{Phase: PhaseA, Actor: "a"}, s)
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, "cancelled", sum.Stopped)
		assert.Len(t, s.out, 1)
	})
	t.Run("sink error stops", func(t *testing.T) {
		w := &fakeWriter{}
		sum, err := NewApplier(w).Run(ctx, items, ApplyOptions{Phase: PhaseA, Actor: "a"}, &memSink{err: errors.New("disk full")})
		require.Error(t, err)
		assert.Equal(t, "sink_error", sum.Stopped)
		assert.Len(t, w.calls, 1)
	})
	t.Run("validation and unwired", func(t *testing.T) {
		_, err := NewApplier(&fakeWriter{}).Run(ctx, items, ApplyOptions{Phase: PhaseA}, &memSink{})
		require.Error(t, err)
		_, err = NewApplier(nil).Run(ctx, items, ApplyOptions{Phase: PhaseA, Actor: "a"}, &memSink{})
		require.Error(t, err)
		w := &fakeWriter{}
		s := &memSink{}
		hd := &ApplyItem{Kind: applyKindHead, Key: "sup:x", Action: "SUPERSEDE", Phase: PhaseA}
		_, err = NewApplier(w).Run(ctx, []*ApplyItem{hd}, ApplyOptions{Phase: PhaseA, Actor: "a"}, s)
		require.NoError(t, err)
		assert.Empty(t, w.calls)
		assert.Equal(t, ReasonNotWired, s.out[0].Result.Reason)
	})
	t.Run("fixed time", func(t *testing.T) {
		fixed := time.Date(2026, 11, 18, 1, 2, 3, 0, time.UTC)
		s := &memSink{}
		_, err := NewApplier(&fakeWriter{}).Run(ctx, items[:1], ApplyOptions{Phase: PhaseA, Actor: "a", Now: func() time.Time { return fixed }}, s)
		require.NoError(t, err)
		assert.Equal(t, fixed, s.out[0].AppliedAt)
	})
}

func TestCSVResultSink(t *testing.T) {
	var rb, eb bytes.Buffer
	sink := NewCSVResultSink(&rb, &eb)
	it := rowItem(1, "S1", "KEEP", PhaseA)
	require.NoError(t, sink.Record(ApplyOutcome{Item: it, Actor: "a", AppliedAt: time.Unix(0, 0), Result: ItemResult{Result: ResultApplied, BeforeJSON: "{\"x\":1}"}}))
	require.NoError(t, sink.Record(ApplyOutcome{Item: it, Actor: "a", Result: ItemResult{Result: ResultFailed, Reason: ReasonDBError}, Err: "db_error: boom"}))
	require.NoError(t, sink.Close())
	res, err := ReadPlanCSV(&rb, ResultHeader)
	require.NoError(t, err)
	require.Len(t, res.Records, 2)
	assert.Equal(t, ResultApplied, res.Get(0, "result"))
	assert.Equal(t, "{\"x\":1}", res.Get(0, "before_json"))
	errs, err := ReadPlanCSV(&eb, []string{"ln", "decision", "error"})
	require.NoError(t, err)
	require.Len(t, errs.Records, 1)
	assert.Equal(t, "db_error: boom", errs.Get(0, "error"))

	var empty bytes.Buffer
	s2 := NewCSVResultSink(&empty, nil)
	require.NoError(t, s2.Close())
	assert.True(t, strings.HasPrefix(empty.String(), "item_key,"))
}
