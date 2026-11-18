package mbactivesync

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func f64(v float64) *float64 { return &v }

type fakeProducts struct{ ps []ReportProduct }

func (f fakeProducts) LoadReportProducts(context.Context) ([]ReportProduct, error) { return f.ps, nil }

func TestReport_SupersedeRepointAndRelinkAndAutofill(t *testing.T) {
	oldH, newH := uuid.New(), uuid.New()
	oldS, newS := uuid.New(), uuid.New()
	snap := &Snapshot{
		Heads: []HeadSnap{{ID: oldH, IsActive: true}, {ID: newH, IsActive: true}},
		Spins: []SpinSnap{
			{ID: oldS, HeadID: oldH, IsActive: true, ShadeCode: "R1", OrionItemCode: "OLD-1", RunLDRPct: f64(1.0)},
			{ID: newS, HeadID: newH, IsActive: true, ShadeCode: "R1", OrionItemCode: "NEW-1", RunLDRPct: f64(1.5), FGProductID: i64(7)},
		},
	}
	plan := &ReportPlan{
		Rows:  []ReportPlanRow{{Ln: 1, Action: ActionKeep, HeadID: newH, SpinID: newS.String(), TargetFG: i64(7), TargetShadeCode: "R1", TargetLDR: f64(1.5)}},
		Heads: []ReportPlanHead{{Kind: KindSupersede, OldHeadID: oldH, OldSpinID: oldS.String(), NewSpin: "spin:" + newS.String()}},
	}
	oldID := oldS
	prods := []ReportProduct{
		{SysID: 1, Code: "P1", Shade: "R1", HasSPCode: true, SPText: "OLD-1", SPSpinID: &oldID, Dozing: f64(1.0), FGCandidates: []int64{7}, Locked: true},
		{SysID: 2, Code: "P2", Shade: "R1", FGCandidates: []int64{7}},                                                     // no MB_SP_CODE -> autofill? already resolvable before -> NO_CHANGE
		{SysID: 3, Code: "P3", Shade: "ZZ", HasSPCode: true, SPText: "OLD-1", Dozing: f64(1.0), FGCandidates: []int64{7}}, // text ref to old spin
	}
	res, err := NewReporter(fakeReader{snap}, fakeProducts{prods}).Run(context.Background(), plan)
	require.NoError(t, err)
	assert.Equal(t, 3, res.Considered)
	assert.Equal(t, 1, res.Stats.Repoints)
	assert.Equal(t, 2, res.ByVia[ViaSupersedeRepoint]) // P1 pointer + P3 text
	assert.Equal(t, 1, res.ByVia[ViaNoChange])
	require.Len(t, res.Rows, 2)
	r := res.Rows[0]
	assert.Equal(t, "P1", r.ProductCode)
	assert.Equal(t, "NEW-1", r.SimSPCode)
	assert.Equal(t, newS.String(), r.SimSpinID)
	require.NotNil(t, r.DozingDelta)
	assert.InDelta(t, 0.5, *r.DozingDelta, 1e-9)
	assert.Equal(t, []string{"P1"}, res.LockedAffected)
	assert.Equal(t, 2, res.DozingChanged)
}

func TestReport_AutofillAndRelinkViaCreatedSpin(t *testing.T) {
	h := uuid.New()
	other := uuid.New()
	snap := &Snapshot{
		Heads: []HeadSnap{{ID: h, IsActive: true}},
		Spins: []SpinSnap{
			{ID: other, HeadID: h, IsActive: true, ShadeCode: "R1", OrionItemCode: "X-1", RunLDRPct: f64(2)},
		},
	}
	plan := &ReportPlan{Rows: []ReportPlanRow{
		{Ln: 5, Action: ActionDuplicateSpin, HeadID: h, TargetFG: i64(9), TargetShadeCode: "R1", TargetLDR: f64(3)},
	}}
	prods := []ReportProduct{
		{SysID: 10, Code: "E", Shade: "R1", FGCandidates: []int64{9}},                                                 // empty -> AUTOFILL_EMPTY
		{SysID: 11, Code: "C", Shade: "R1", HasSPCode: true, SPText: "X-1", FGCandidates: []int64{9}, Dozing: f64(2)}, // other spin -> RELINK_NEEDED
		{SysID: 12, Code: "N", Shade: "R1", FGCandidates: []int64{99}},                                                // no FG match
	}
	res, err := BuildReport(plan, snap, prods)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Stats.NewSpins)
	assert.Equal(t, 1, res.ByVia[ViaAutofillEmpty])
	assert.Equal(t, 1, res.ByVia[ViaRelinkNeeded])
	assert.Equal(t, 1, res.ByVia[ViaNoChange])
	require.Len(t, res.Rows, 2)
	assert.Equal(t, "ln:5", res.Rows[0].SimSpinID)
	assert.Equal(t, "NEW(ln:5)", res.Rows[0].SimSPCode)
	require.NotNil(t, res.Rows[1].SimDozing)
	assert.InDelta(t, 3.0, *res.Rows[1].SimDozing, 1e-9)
}

func TestReport_DeactivateAndDuplicateKeys(t *testing.T) {
	h := uuid.New()
	a, b := uuid.New(), uuid.New()
	snap := &Snapshot{
		Heads: []HeadSnap{{ID: h, IsActive: true}},
		Spins: []SpinSnap{
			{ID: a, HeadID: h, IsActive: true, ShadeCode: "R1", OrionItemCode: "A", FGProductID: i64(7)},
			{ID: b, HeadID: h, IsActive: true, ShadeCode: "r1 ", OrionItemCode: "B", FGProductID: i64(7)},
		},
	}
	res, err := BuildReport(&ReportPlan{Heads: []ReportPlanHead{{Kind: KindDeactivateSpin, OldSpinID: a.String()}}}, snap, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Stats.DuplicateKeys) // a deactivated, only b holds the key
	res, err = BuildReport(&ReportPlan{}, snap, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Stats.DuplicateKeys)
}

func TestReport_StaleRefsAndNoChange(t *testing.T) {
	snap := &Snapshot{}
	plan := &ReportPlan{Rows: []ReportPlanRow{{Ln: 1, Action: ActionKeep, SpinID: uuid.NewString()}},
		Heads: []ReportPlanHead{{Kind: KindSupersede, OldSpinID: uuid.NewString(), NewSpin: "ln:3"}}}
	res, err := BuildReport(plan, snap, []ReportProduct{{SysID: 1, Code: "P", Shade: "A"}})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Stats.MissingSpins)
	assert.Equal(t, 1, res.ByVia[ViaNoChange])
	assert.Empty(t, res.Rows)
	_, err = BuildReport(nil, snap, nil)
	assert.Error(t, err)
}

func TestReport_LoadPlanAndCSV(t *testing.T) {
	hid, sid := uuid.NewString(), uuid.NewString()
	rowsCSV := "ln,sysid,action,phase,sub_actions,item_key,decision,head_id,spin_id,source_spin_id,target_fg,target_shade_code,target_shade_name,target_den,target_fil,target_cs,target_ldr\n" +
		"1,S1,KEEP,A,," + "row:1,APPLY," + hid + "," + sid + ",,7,R1,,,,,1.5\n" +
		"2,S2,REPORT_ONLY,,,row:2,SKIP,,,,,,,,,,\n" +
		"3,S3,UPDATE_LDR,A,,row:3,REVIEW," + hid + "," + sid + ",,7,R1,,,,,2\n"
	headsCSV := "item_key,kind,old_head_id,old_spin_id,new_head_id,new_spin_ref,phase,decision\n" +
		"sup:x:y,SUPERSEDE," + hid + "," + sid + ",," + "ln:1,B,APPLY\n" +
		"dh:z,DEACTIVATE_HEAD," + hid + ",,,,B,REVIEW\n"
	rc, err := ReadPlanCSV(bytes.NewReader([]byte(rowsCSV)), RowsRequiredColumns)
	require.NoError(t, err)
	hc, err := ReadPlanCSV(bytes.NewReader([]byte(headsCSV)), HeadsRequiredColumns)
	require.NoError(t, err)
	rp, err := LoadReportPlan(rc, hc)
	require.NoError(t, err)
	require.Len(t, rp.Rows, 1)
	assert.Equal(t, 1, rp.Rows[0].Ln)
	assert.Equal(t, sid, rp.Rows[0].SpinID)
	require.NotNil(t, rp.Rows[0].TargetFG)
	assert.EqualValues(t, 7, *rp.Rows[0].TargetFG)
	require.NotNil(t, rp.Rows[0].TargetLDR)
	assert.InDelta(t, 1.5, *rp.Rows[0].TargetLDR, 1e-9)
	require.Len(t, rp.Heads, 1)
	assert.Equal(t, KindSupersede, rp.Heads[0].Kind)
	assert.Equal(t, "ln:1", rp.Heads[0].NewSpin)

	_, err = LoadReportPlan(nil, hc)
	assert.Error(t, err)

	res := &ReportResult{ByVia: map[ReportVia]int{ViaNoChange: 1}, Rows: []ReportRow{{ProductSysID: 1, ProductCode: "P", Via: ViaRelinkNeeded, DozingDelta: f64(0.5)}}, LockedAffected: []string{"P"}}
	var cb, sb bytes.Buffer
	require.NoError(t, WriteReportCSV(&cb, res.Rows))
	require.NoError(t, WriteReportSummary(&sb, res))
	assert.Contains(t, cb.String(), "product_sys_id,product_code")
	assert.Contains(t, cb.String(), "RELINK_NEEDED")
	assert.Contains(t, sb.String(), "locked products affected: 1")
	assert.Len(t, ReportRecord(&res.Rows[0]), len(ReportHeader))
}
