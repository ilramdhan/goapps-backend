package mbcomparisonfill

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	uA = "11111111-1111-1111-1111-111111111111"
	uB = "22222222-2222-2222-2222-222222222222"
	uC = "33333333-3333-3333-3333-333333333333"
)

func TestPlanRelink(t *testing.T) {
	okProd := RelinkProduct{Found: true, Active: true, HasRow: true, Text: "X", Ptr: uA}
	spins := map[string]RelinkSpin{uA: {}, uB: {}, uC: {Parent: uA, Den: 250, Fil: 144, LDR: 1.45, Actual: true, Active: true}}
	ptr := RelinkRow{Product: 1, Kind: RelinkPointer, TargetSpin: uB}
	child := RelinkRow{Product: 1, Kind: RelinkChild, ParentSpin: uA, ChildKey: "k", Den: 250, Fil: 144, LDR: 1.45}
	tests := []struct {
		name string
		row  RelinkRow
		mut  func(*RelinkProduct)
		want Decision
		why  string
	}{
		{"pointer", ptr, nil, RelinkPointer, ""},
		{"same", RelinkRow{Product: 1, Kind: RelinkPointer, TargetSpin: uA}, nil, DecisionSkipSame, "already_target"},
		{"not found", ptr, func(p *RelinkProduct) { *p = RelinkProduct{} }, DecisionReport, "product_not_found"},
		{"inactive", ptr, func(p *RelinkProduct) { p.Active = false }, DecisionReport, "product_inactive"},
		{"locked", ptr, func(p *RelinkProduct) { p.Locked = true }, DecisionReport, "product_locked"},
		{"no row", ptr, func(p *RelinkProduct) { p.HasRow = false }, DecisionReport, "no_mb_sp_code_row"},
		{"target missing", RelinkRow{Product: 1, Kind: RelinkPointer, TargetSpin: "99999999-9999-9999-9999-999999999999"}, nil, DecisionReport, "target_spin_missing_or_deleted"},
		{"export stale", RelinkRow{Product: 1, Kind: RelinkPointer, TargetSpin: uB, checkExport: true, exportPtr: uC}, nil, DecisionReport, "pointer_changed_since_export"},
		{"export same (case)", RelinkRow{Product: 1, Kind: RelinkPointer, TargetSpin: uB, checkExport: true, exportPtr: "11111111-1111-1111-1111-111111111111"}, nil, RelinkPointer, ""},
		{"child", child, nil, RelinkChild, ""},
		{"child already", child, func(p *RelinkProduct) { p.Ptr = uC }, DecisionSkipSame, "already_on_matching_child"},
		{"parent missing", RelinkRow{Product: 1, Kind: RelinkChild, ParentSpin: uB + "x"}, nil, DecisionReport, "target_spin_missing_or_deleted"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := okProd
			if tc.mut != nil {
				tc.mut(&p)
			}
			got := PlanRelink([]RelinkRow{tc.row}, map[int64]RelinkProduct{1: p}, spins)[0]
			assert.Equal(t, tc.want, got.Decision)
			assert.Equal(t, tc.why, got.Reason)
			assert.Equal(t, p.Ptr, got.SnapPtr)
			assert.Equal(t, p.Text, got.SnapText)
		})
	}
}

func TestPlanRelink_DuplicateProductReported(t *testing.T) {
	rows := []RelinkRow{{Product: 1, Kind: RelinkPointer, TargetSpin: uB}, {Product: 1, Kind: RelinkPointer, TargetSpin: uC}}
	got := PlanRelink(rows, map[int64]RelinkProduct{1: {Found: true, Active: true, HasRow: true}}, map[string]RelinkSpin{uB: {}, uC: {}})
	assert.Equal(t, "product_in_several_input_rows", got[0].Reason)
	assert.Equal(t, DecisionReport, got[1].Decision)
}

func TestParseDenFilLDR(t *testing.T) {
	d, f, l, err := ParseDenFilLDR("250.00/144/1.4500")
	require.NoError(t, err)
	assert.Equal(t, 250.0, d)
	assert.Equal(t, 144, f)
	assert.Equal(t, 1.45, l)
	_, _, _, err = ParseDenFilLDR("250/x/1")
	assert.Error(t, err)
	_, _, _, err = ParseDenFilLDR("250/1")
	assert.Error(t, err)
}

func writeTmp(t *testing.T, name, body string) string {
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestReadRelinkInputs(t *testing.T) {
	e := writeTmp(t, "e.csv", "env,product_sys_id,item_code,current_spin_id,target_spin_id,action\n"+
		"staging,1,P1,,"+uB+",RELINK\nstaging,2,P2,,,REPORT_TIE\n")
	d := writeTmp(t, "d.csv", "entity,id,sysid,field,goapps_value,legacy_value,env,product_sys_id,target_spin_id,action\n"+
		"head,x,,orion,,C,staging,,,FILL_HEAD_ORION\n"+"product,3,,ptr,"+uA+",x,staging,3,"+uC+",RELINK_TO_NAMED_HEAD_SPIN\n")
	c := writeTmp(t, "c.csv", "env,product_sys_id,item_code,parent_spin_id,target_den_fil_ldr,child_key,action\n"+
		"staging,4,P4,"+uA+",250.00/144/1.4500,K#2,MOVE_TO_CHILD\nstaging,5,P5,"+uA+",250.00/144/1.4500,,KEEP_ON_PARENT\n")
	rows, err := ReadRelinkInputs(e, d, c)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, RelinkPointer, rows[0].Kind)
	assert.True(t, rows[1].checkExport)
	assert.Equal(t, uA, rows[1].exportPtr)
	assert.Equal(t, RelinkChild, rows[2].Kind)
	assert.Equal(t, 144, rows[2].Fil)

	// mixed env rejected
	c2 := writeTmp(t, "c2.csv", "env,product_sys_id,item_code,parent_spin_id,target_den_fil_ldr,child_key,action\nprod,9,P,"+uA+",250/144/1.45,K#2,MOVE_TO_CHILD\n")
	_, err = ReadRelinkInputs(e, "", c2)
	assert.ErrorContains(t, err, "mixed")
	// missing column / bad target
	_, err = ReadRelinkInputs(writeTmp(t, "bad.csv", "env,product_sys_id\nstaging,1\n"), "", "")
	assert.ErrorContains(t, err, "missing")
	_, err = ReadRelinkInputs(writeTmp(t, "bad2.csv", "env,product_sys_id,item_code,current_spin_id,target_spin_id,action\nstaging,1,P,,nope,RELINK\n"), "", "")
	assert.ErrorContains(t, err, "bad target_spin_id")
}

func TestRelinkPlanAndResultCSVRoundTrip(t *testing.T) {
	rows := []RelinkRow{
		{Env: "staging", Source: "E", Product: 1, Item: "P", Kind: RelinkPointer, TargetSpin: uB, SnapText: "T,1", SnapPtr: uA, Decision: RelinkPointer},
		{Env: "staging", Source: "CONFLICT", Product: 2, Kind: RelinkChild, ParentSpin: uA, ChildKey: "K#2", Den: 250, Fil: 144, LDR: 1.45, Decision: RelinkChild},
	}
	p := filepath.Join(t.TempDir(), "plan.csv")
	f, err := os.Create(p) //nolint:gosec // test temp path
	require.NoError(t, err)
	require.NoError(t, WriteRelinkPlanCSV(f, rows))
	require.NoError(t, f.Close())
	back, err := ReadRelinkPlanCSV(p)
	require.NoError(t, err)
	require.Len(t, back, 2)
	assert.Equal(t, "T,1", back[0].SnapText)
	assert.Equal(t, RelinkChild, back[1].Decision)
	assert.Equal(t, 1.45, back[1].LDR)
	assert.Equal(t, 2, back[1].Row)
}

type fakeRelink struct {
	RelinkStore
	ensured []RelinkChildSpec
	applied []RelinkRow
}

func (f *fakeRelink) EnsureChild(_ context.Context, _ string, c RelinkChildSpec, _ bool) (string, bool, string, error) {
	f.ensured = append(f.ensured, c)
	return uC, true, "", nil
}

func (f *fakeRelink) ApplyBatch(_ context.Context, _ string, rows []RelinkRow, _ bool) ([]RelinkResult, error) {
	var out []RelinkResult
	for _, r := range rows {
		f.applied = append(f.applied, r)
		out = append(out, RelinkResult{Row: r.Row, Product: r.Product, Decision: r.Decision, Status: ResultApplied})
	}
	return out, nil
}

func TestApplyRelink_GroupsChildrenAndRejectsTamper(t *testing.T) {
	ch := func(row int, p int64) RelinkRow {
		return RelinkRow{Env: "staging", Row: row, Product: p, Kind: RelinkChild, Decision: RelinkChild, ParentSpin: uA, ChildKey: "k", Den: 250, Fil: 144, LDR: 1.45}
	}
	rows := []RelinkRow{
		ch(1, 1), ch(2, 2),
		{Env: "staging", Row: 3, Product: 3, Kind: RelinkPointer, Decision: RelinkPointer, TargetSpin: "bad"},
		{Env: "staging", Row: 4, Product: 4, Kind: RelinkPointer, Decision: DecisionSkipSame},
	}
	f := &fakeRelink{}
	res, err := ApplyRelink(context.Background(), f, "mbcmp-staging", rows, 0, 1, true)
	require.NoError(t, err)
	assert.Len(t, f.ensured, 1, "one child per (parent,key-spec) group")
	assert.Equal(t, []string{ResultFailed, ResultApplied, ResultApplied}, []string{res[0].Status, res[1].Status, res[2].Status})
	assert.Equal(t, "plan_tampered", res[0].Reason)
	assert.True(t, res[1].ChildCreated)
	assert.False(t, res[2].ChildCreated, "only the first product of a group records the creation")
	assert.Equal(t, uC, res[2].ChildID)

	_, err = ApplyRelink(context.Background(), f, "bob", rows, 0, 1, true)
	assert.Error(t, err)
	_, err = ApplyRelink(context.Background(), f, "mbcmp-prod", rows, 0, 1, true)
	assert.ErrorContains(t, err, "must be")
}

type fakeRevert struct {
	RelinkStore
	deleted []string
}

func (f *fakeRevert) RevertPointer(_ context.Context, _ string, r RelinkResult) (RelinkResult, error) {
	r.Status = ResultReverted
	return r, nil
}

func (f *fakeRevert) DeleteChild(_ context.Context, _, id string) (string, string, error) {
	f.deleted = append(f.deleted, id)
	return ResultReverted, "", nil
}

func TestRevertRelink_ChildCandidatesFromAllRows(t *testing.T) {
	items := []RelinkResult{
		{Env: "staging", Row: 1, Decision: RelinkChild, Status: ResultFailed, Reason: "changed_since_plan", ChildID: uC, ChildCreated: true},
		{Env: "staging", Row: 2, Decision: RelinkChild, Status: ResultApplied, ChildID: uC},
		{Env: "staging", Row: 3, Decision: RelinkChild, Status: ResultAlreadyApplied, ChildID: uB},
	}
	f := &fakeRevert{}
	out, err := RevertRelink(context.Background(), f, "mbcmp-staging", items)
	require.NoError(t, err)
	assert.Equal(t, []string{uC, uB}, f.deleted)
	assert.Len(t, out, 3) // 1 reverted pointer + 2 child rows

	_, err = RevertRelink(context.Background(), f, "mbcmp-prod", items)
	assert.ErrorContains(t, err, "must be")
	mixed := append(items, RelinkResult{Env: "prod"})
	_, err = RevertRelink(context.Background(), f, "mbcmp-staging", mixed)
	assert.ErrorContains(t, err, "mixed")
}

func TestRelinkSpinMatches_RequiresActive(t *testing.T) {
	c := RelinkChildSpec{Parent: uA, Den: 250, Fil: 144, LDR: 1.45}
	s := RelinkSpin{Parent: uA, Den: 250, Fil: 144, LDR: 1.45, Actual: true, Active: true}
	assert.True(t, s.matches(c))
	s.Active = false
	assert.False(t, s.matches(c))
}
