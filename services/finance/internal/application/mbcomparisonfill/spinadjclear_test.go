package mbcomparisonfill

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanAdjClear(t *testing.T) {
	tests := []struct {
		name string
		r    AdjRow
		want Decision
		why  string
		tgt  float64
	}{
		{"adj set", AdjRow{RunLDR: f64(1.87), Calc: f64(1.5), Adj: f64(0.37), LDRType: "ACTUAL", IsActual: true}, DecisionApply, "", 1.87},
		{"unlocked", AdjRow{RunLDR: f64(2), Calc: f64(2), LDRType: "CALCULATED"}, DecisionApply, "", 2},
		{"calc differs", AdjRow{RunLDR: f64(2), Calc: f64(3), LDRType: "ACTUAL", IsActual: true}, DecisionApply, "", 2},
		{"clean", AdjRow{RunLDR: f64(2), Calc: f64(2), LDRType: "ACTUAL", IsActual: true}, DecisionSkipSame, "already_clean", 0},
		{"null run", AdjRow{Calc: f64(2), Adj: f64(1)}, DecisionReport, "run_ldr_null", 0},
		{"zero run", AdjRow{RunLDR: f64(0)}, DecisionReport, "run_ldr_not_positive", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PlanAdjClear([]AdjRow{tc.r})[0]
			assert.Equal(t, tc.want, got.Decision)
			assert.Equal(t, tc.why, got.Reason)
			assert.InDelta(t, tc.tgt, got.Target, 1e-9)
		})
	}
}

func TestAdjCSVRoundTripAndTamper(t *testing.T) {
	rows := PlanAdjClear([]AdjRow{{ID: "a", Code: "C1", RunLDR: f64(1.87), Adj: f64(0.5), LDRType: "ACTUAL", IsActual: true}})
	pf := filepath.Join(t.TempDir(), "p.csv")
	f, err := os.Create(pf) //nolint:gosec // test temp path
	require.NoError(t, err)
	require.NoError(t, WriteAdjPlanCSV(f, rows))
	require.NoError(t, f.Close())
	back, err := ReadAdjCSV(pf)
	require.NoError(t, err)
	assert.Equal(t, rows, back)

	assert.True(t, adjConsistent(back[0]))
	back[0].Target = 9
	assert.False(t, adjConsistent(back[0]), "target != run_ldr must be rejected")
}

type fakeAdj struct{ batches [][]string }

func (f *fakeAdj) Candidates(context.Context) ([]AdjRow, error) { return nil, nil }
func (f *fakeAdj) ApplyBatch(_ context.Context, _ string, rows []AdjRow, _ bool) ([]AdjResult, error) {
	var ids []string
	var out []AdjResult
	for _, r := range rows {
		ids = append(ids, r.ID)
		out = append(out, AdjResult{AdjRow: r, Result: ResultApplied})
	}
	f.batches = append(f.batches, ids)
	return out, nil
}
func (f *fakeAdj) Revert(_ context.Context, _ string, r AdjResult) (AdjResult, error) { return r, nil }

func TestApplyAdjClear_LimitBatchTamperAndSkips(t *testing.T) {
	mk := func(id string, d Decision, target float64) AdjRow {
		return AdjRow{ID: id, RunLDR: f64(2), Target: target, Decision: d}
	}
	rows := []AdjRow{mk("a", DecisionApply, 2), mk("skip", DecisionSkipSame, 2), mk("bad", DecisionApply, 9),
		mk("b", DecisionApply, 2), mk("rep", DecisionReport, 0), mk("c", DecisionApply, 2)}
	f := &fakeAdj{}
	res, err := ApplyAdjClear(context.Background(), f, "mbcmp-local", rows, 0, 2, true)
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"a", "b"}, {"c"}}, f.batches, "batch slicing, non-APPLY rows skipped")
	var tampered []string
	for _, r := range res {
		if r.Result == ResultFailed {
			assert.Equal(t, "plan_tampered", r.Detail)
			tampered = append(tampered, r.ID)
		}
	}
	assert.Equal(t, []string{"bad"}, tampered)

	f = &fakeAdj{}
	_, err = ApplyAdjClear(context.Background(), f, "mbcmp-local", rows, 2, 100, true)
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"a", "b"}}, f.batches, "limit counts spins (tampered rows excluded)")

	_, err = ApplyAdjClear(context.Background(), f, "bob", rows, 0, 0, true)
	assert.Error(t, err, "actor validated")
}
