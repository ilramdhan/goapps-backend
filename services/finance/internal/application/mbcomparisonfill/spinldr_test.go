package mbcomparisonfill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func f64(v float64) *float64 { return &v }

func TestPlanSpinLDR(t *testing.T) {
	base := func(verdict string) SpinRow {
		return SpinRow{ID: "a", Verdict: verdict, RunLDR: f64(1.87), Calc: f64(2), IsActual: true, Distinct: 1, Min: f64(2.28), Max: f64(2.28)}
	}
	liveOf := func(r SpinRow) SpinLive { return SpinLive{Found: true, RunLDR: r.RunLDR, Adj: r.Adj, IsActual: r.IsActual} }
	tests := []struct {
		name string
		mut  func(*SpinRow, *SpinLive)
		verd string
		want Decision
		why  string
	}{
		{"set", func(*SpinRow, *SpinLive) {}, VerdictSetLDR, SpinApplySet, ""},
		{"same verdict", func(*SpinRow, *SpinLive) {}, VerdictSame, DecisionSkipSame, "csv_verdict_same"},
		{"conflict", func(*SpinRow, *SpinLive) {}, VerdictConflict, SpinSkipConflict, "excel_values_differ"},
		{"distinct>1", func(r *SpinRow, _ *SpinLive) { r.Distinct = 2 }, VerdictSetLDR, SpinSkipConflict, "excel_values_differ"},
		{"dipakai null run_ldr", func(r *SpinRow, l *SpinLive) { r.RunLDR, r.IsActual = nil, false; *l = liveOf(*r) }, VerdictDipakai, SpinApplySet, ""},
		{"adj basi", func(r *SpinRow, l *SpinLive) { r.RunLDR, r.Adj = f64(2.28), f64(0.5); *l = liveOf(*r) }, VerdictAdjBasi, SpinApplyClearAdj, ""},
		{"adj basi run differs", func(*SpinRow, *SpinLive) {}, VerdictAdjBasi, DecisionReport, "adj_basi_but_run_ldr_differs"},
		{"stale", func(_ *SpinRow, l *SpinLive) { l.RunLDR = f64(3) }, VerdictSetLDR, DecisionReport, "stale_vs_csv_snapshot"},
		{"not found", func(_ *SpinRow, l *SpinLive) { *l = SpinLive{} }, VerdictSetLDR, DecisionReport, "spin_not_found"},
		{"target zero", func(r *SpinRow, _ *SpinLive) { r.Min, r.Max = f64(0), f64(0) }, VerdictSetLDR, DecisionReport, "target_not_positive"},
		{"already target", func(r *SpinRow, l *SpinLive) { r.RunLDR = f64(2.28); *l = liveOf(*r) }, VerdictDipakai, DecisionSkipSame, "already_target"},
		{"unknown verdict", func(*SpinRow, *SpinLive) {}, "WAT", DecisionReport, "unknown_verdict:WAT"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := base(tc.verd)
			l := liveOf(r)
			tc.mut(&r, &l)
			got := PlanSpinLDR([]SpinRow{r}, map[string]SpinLive{"a": l})[0]
			assert.Equal(t, tc.want, got.Decision)
			assert.Equal(t, tc.why, got.Reason)
		})
	}
}

func TestPlanSpinLDR_TargetIsRoundedSp68(t *testing.T) {
	r := SpinRow{ID: "a", Verdict: VerdictSetLDR, Distinct: 1, Min: f64(2.283456), Max: f64(2.283456)}
	got := PlanSpinLDR([]SpinRow{r}, map[string]SpinLive{"a": {Found: true}})[0]
	assert.Equal(t, SpinApplySet, got.Decision)
	assert.InDelta(t, 2.2835, got.Target, 1e-9)
}

func TestReadSpinCSV_Q1AndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	q1 := filepath.Join(dir, "q1.csv")
	require.NoError(t, os.WriteFile(q1, []byte("mbs_id,spin_code,run_ldr,calc,adj,is_actual,excel_products,distinct_sp68,sp68_min,sp68_max,all_linked_products,verdict\n"+
		"x1,CMB1,1.87,2.28,,true,2,1,2.28,2.28,2,BEDA_SET_RUN_LDR\n"), 0o600))
	rows, err := ReadSpinCSV(q1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, VerdictSetLDR, rows[0].Verdict)
	assert.Nil(t, rows[0].Adj)
	assert.True(t, rows[0].IsActual)

	rows = PlanSpinLDR(rows, map[string]SpinLive{"x1": {Found: true, RunLDR: f64(1.87), IsActual: true}})
	pf := filepath.Join(dir, "plan.csv")
	f, err := os.Create(pf) //nolint:gosec // test temp path
	require.NoError(t, err)
	require.NoError(t, WriteSpinPlanCSV(f, rows))
	require.NoError(t, f.Close())
	back, err := ReadSpinCSV(pf)
	require.NoError(t, err)
	assert.Equal(t, SpinApplySet, back[0].Decision)
	assert.InDelta(t, 2.28, back[0].Target, 1e-9)
}
