package mbactivesync

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const revertHdr = "item_key,ln,sysid,action,phase,result,reason,spin_id,clone_spin_id,head_id,before_json,after_json,message,actor,applied_at\n"

func TestLoadRevertItems_ReverseOrderAndFilters(t *testing.T) {
	s1, s2, c := uuid.New(), uuid.New(), uuid.New()
	f1 := revertHdr +
		"row:1,1,S1,KEEP,A,APPLIED,," + s1.String() + ",,,{}," + `"{""a"":1}"` + ",,me,t\n" +
		"row:2,2,S2,UPDATE_LDR,A,SKIPPED,already_in_target," + s2.String() + ",,,,,,me,t\n" +
		"row:3,3,S3,DUPLICATE_SPIN,C,PARTIAL,," + s1.String() + "," + c.String() + ",,,,,me,t\n"
	f2 := revertHdr +
		"row:3,3,S3,DUPLICATE_SPIN,C,APPLIED,," + c.String() + "," + c.String() + ",,{},{},,me,t\n" +
		"row:1,1,S1,KEEP,A,APPLIED,," + s1.String() + ",,,{},{},revert: applied,me,t\n" +
		"dh:x,,,DEACTIVATE_HEAD,B,APPLIED,,,,,{},{},,me,t\n"
	got, err := LoadRevertItems(strings.NewReader(f1), strings.NewReader(f2))
	require.NoError(t, err)
	require.Len(t, got, 3, "SKIPPED/PARTIAL and earlier revert rows are ignored")
	// reverse application order: last file/row first
	assert.Equal(t, []string{"dh:x", "row:3", "row:1"}, []string{got[0].Key, got[1].Key, got[2].Key})
	assert.Equal(t, c, got[1].CloneSpinID)
	assert.Equal(t, 3, got[1].Ln)
	assert.Equal(t, `{"a":1}`, got[2].AfterJSON)

	_, err = LoadRevertItems(strings.NewReader("item_key,ln\nx,1\n"))
	require.Error(t, err)

	bad, err := LoadRevertItems(strings.NewReader(revertHdr + "row:1,1,S1,KEEP,A,APPLIED,,not-a-uuid,,,{},{},,me,t\n"))
	require.NoError(t, err)
	out := NewApplier(&recWriter{}).revertOne(context.Background(), bad[0], "me")
	assert.Equal(t, ReasonBadPlanItem, out.Result.Reason)
}

func TestRunRevert_DispatchFiltersLimitAndMessage(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	items := []*RevertItem{
		{Key: "row:2", Ln: 2, SysID: "S2", Action: "KEEP", Phase: PhaseA, SpinID: b, BeforeJSON: "B", AfterJSON: "A"},
		{Key: "dh:1", SysID: "S1", Action: string(KindDeactivateHead), Phase: PhaseB, HeadID: a},
		{Key: "row:1", Ln: 1, SysID: "S1", Action: "KEEP", Phase: PhaseA, SpinID: a},
	}
	w := &recWriter{}
	s := &memSink{}
	sum, err := NewApplier(w).RunRevert(t.Context(), items, RevertOptions{Actor: "rev", OnlySysID: []string{"S1", "S2"}, Limit: 2}, s)
	require.NoError(t, err)
	assert.Equal(t, "limit", sum.Stopped)
	require.Len(t, w.rev, 2)
	assert.Equal(t, RevertInput{Key: "row:2", Action: "KEEP", Phase: PhaseA, SpinID: b, BeforeJSON: "B", AfterJSON: "A", Actor: "rev"}, w.rev[0])
	assert.Equal(t, a, w.rev[1].HeadID)
	require.Len(t, s.out, 2)
	assert.True(t, strings.HasPrefix(s.out[0].Result.Message, RevertMessagePrefix))
	assert.Equal(t, applyKindHead, s.out[1].Item.Kind)

	w = &recWriter{}
	s = &memSink{}
	_, err = NewApplier(w).RunRevert(t.Context(), items, RevertOptions{Actor: "rev", OnlySysID: []string{"S2"}}, s)
	require.NoError(t, err)
	assert.Len(t, w.rev, 1)

	_, err = NewApplier(w).RunRevert(t.Context(), items, RevertOptions{Actor: " "}, s)
	require.Error(t, err)
	_, err = NewApplier(w).RunRevert(t.Context(), items, RevertOptions{Actor: "x"}, nil)
	require.Error(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	sum, err = NewApplier(&recWriter{}).RunRevert(ctx, items, RevertOptions{Actor: "x"}, &memSink{})
	require.Error(t, err)
	assert.Equal(t, "cancelled", sum.Stopped)
}

func TestManualPlanRecordsAndSink(t *testing.T) {
	rows := &PlanCSV{Index: map[string]int{"ln": 0, "sysid": 1, "action": 2, "phase": 3, "item_key": 4, "file_poy": 5, "file_shade": 6, "reason": 7, "spin_id": 8, "head_id": 9},
		Records: [][]string{
			{"1", "S1", "KEEP", "A", "row:1", "P", "R", "", "", ""},
			{"2", "S2", "NEW_RECIPE", "D", "row:2", "P2", "R2", "", "", ""},
			{"3", "S3", "REPORT_ONLY", "", "row:3", "P3", "R3", "poy_not_found", "", "h"},
		}}
	heads := &PlanCSV{Index: map[string]int{"item_key": 0, "kind": 1, "old_sysid": 2, "phase": 3, "reason": 4, "old_spin_id": 5, "old_head_id": 6},
		Records: [][]string{
			{"dh:1", "DEACTIVATE_HEAD", "S1", "B", "", "", "h1"},
			{"rs:1", "REPORT_SPIN", "S1", "", "unallocated", "sp1", "h1"},
			{"rh:1", "REPORT_HEAD", "S4", "", "dead_head", "", "h4"},
		}}
	recs := ManualPlanRecords(rows, heads)
	require.Len(t, recs, 4)
	for _, r := range recs {
		assert.Len(t, r, len(ManualItemsHeader))
	}
	assert.Equal(t, "NEW_RECIPE", recs[0][3])
	assert.Equal(t, "report_only", recs[1][5])
	assert.Equal(t, "REPORT_SPIN", recs[2][3])
	assert.Equal(t, "sp1", recs[2][6])
	assert.Equal(t, "h4", recs[3][8])

	var res, man bytes.Buffer
	sink := NewCSVResultSink(&res, nil).WithManual(&man).WithManualPlan(recs)
	it := &ApplyItem{Key: "row:9", Ln: 9, SysID: "S9", Action: string(ActionGenerateSpin), Phase: PhaseC}
	require.NoError(t, sink.Record(ApplyOutcome{Item: it, Result: ItemResult{Result: ResultFailed, Reason: ReasonValidateGate, ManualReason: ReasonValidateGate, HeadID: "hh", Message: "m"}}))
	require.NoError(t, sink.Close())
	all, err := csv.NewReader(&man).ReadAll()
	require.NoError(t, err)
	require.Len(t, all, 1+4+1)
	assert.Equal(t, ManualItemsHeader, all[0])
	assert.Equal(t, "row:9", all[5][0])
	assert.Equal(t, ReasonValidateGate, all[5][5])

	// Plan-only (no outcome): header + plan items still written on Close.
	man.Reset()
	sink = NewCSVResultSink(&res, nil).WithManual(&man).WithManualPlan(recs)
	require.NoError(t, sink.Close())
	all, err = csv.NewReader(&man).ReadAll()
	require.NoError(t, err)
	assert.Len(t, all, 5)
}

func TestFinishOutcome_ValidateGateIsManual(t *testing.T) {
	out := finishOutcome(ApplyOutcome{}, ItemResult{Result: ResultFailed, Reason: ReasonValidateGate, Message: "x"}, nil)
	assert.Equal(t, ReasonValidateGate, out.Result.ManualReason)
	out = finishOutcome(ApplyOutcome{}, ItemResult{Result: ResultFailed, Reason: ReasonDBError}, nil)
	assert.Empty(t, out.Result.ManualReason)
}
