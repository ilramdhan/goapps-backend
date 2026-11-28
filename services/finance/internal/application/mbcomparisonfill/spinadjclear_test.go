package mbcomparisonfill

import (
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
