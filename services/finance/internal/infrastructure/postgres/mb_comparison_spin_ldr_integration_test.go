package postgres_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fill "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomparisonfill"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func f64p(v float64) *float64 { return &v }

// TestMBComparisonSpinLDR_Integration: plan -> dry-run -> commit (SET, CLEAR_ADJ, NULL-run_ldr dipakai) -> stale guard -> revert.
func TestMBComparisonSpinLDR_Integration(t *testing.T) {
	f := newMBASFixture(t, "mbcmp_spinldr")
	pg := postgres.NewMBComparisonSpinLDR(postgres.NewDBFromSQL(f.raw))
	h := f.head(true, "Y1")
	setID := f.spin(h, "Y1", 1.87, true, nil)
	adjID := f.spin(h, "Y1", 2.28, true, nil)
	nullID := f.spin(h, "Y1", 1, true, nil)
	f.exec(`UPDATE mst_mb_spin SET mbs_ldr_is_actual = TRUE WHERE mbs_id = $1`, setID)
	f.exec(`UPDATE mst_mb_spin SET mbs_ldr_is_actual = TRUE, mbs_ldr_calculated_pct = 2.28, mbs_ldr_adjustment_pct = 0.5 WHERE mbs_id = $1`, adjID)
	f.exec(`UPDATE mst_mb_spin SET mbs_run_ldr_pct = NULL, mbs_ldr_is_actual = FALSE WHERE mbs_id = $1`, nullID)

	rows := []fill.SpinRow{
		{ID: setID.String(), Verdict: fill.VerdictSetLDR, RunLDR: f64p(1.87), IsActual: true, Distinct: 1, Min: f64p(2.28), Max: f64p(2.28)},
		{ID: adjID.String(), Verdict: fill.VerdictAdjBasi, RunLDR: f64p(2.28), Adj: f64p(0.5), IsActual: true, Distinct: 1, Min: f64p(2.28), Max: f64p(2.28)},
		{ID: nullID.String(), Verdict: fill.VerdictDipakai, Distinct: 1, Min: f64p(3.72), Max: f64p(3.72)},
	}
	live, err := pg.Live(f.ctx, fill.SpinPlanIDs(rows))
	require.NoError(t, err)
	plan := fill.PlanSpinLDR(rows, live)
	assert.Equal(t, fill.SpinApplySet, plan[0].Decision)
	assert.Equal(t, fill.SpinApplyClearAdj, plan[1].Decision, plan[1].Reason)
	assert.Equal(t, fill.SpinApplySet, plan[2].Decision, plan[2].Reason)

	// dry-run writes nothing
	res, err := fill.ApplySpinLDR(f.ctx, pg, "mbcmp-test", plan, 0, 10, false)
	require.NoError(t, err)
	require.Len(t, res, 3)
	assert.Equal(t, fill.ResultWouldApply, res[0].Result)
	assert.InDelta(t, 1.87, f.readSpin(setID).ldr, 1e-9)

	// stale guard: a user edits a spin between plan and apply
	f.exec(`UPDATE mst_mb_spin SET mbs_run_ldr_pct = 9 WHERE mbs_id = $1`, setID)
	res, err = fill.ApplySpinLDR(f.ctx, pg, "mbcmp-test", plan, 0, 10, true)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultFailed, res[0].Result)
	assert.Equal(t, "changed_since_plan", res[0].Detail)
	assert.Equal(t, fill.ResultApplied, res[1].Result, res[1].Detail)
	assert.Equal(t, fill.ResultApplied, res[2].Result, res[2].Detail)
	f.exec(`UPDATE mst_mb_spin SET mbs_run_ldr_pct = 1.87 WHERE mbs_id = $1`, setID)

	// canary --limit 1 applies only the first row
	one, err := fill.ApplySpinLDR(f.ctx, pg, "mbcmp-test", plan[:1], 1, 10, true)
	require.NoError(t, err)
	require.Equal(t, fill.ResultApplied, one[0].Result, one[0].Detail)
	assert.InDelta(t, 2.28, f.readSpin(setID).ldr, 1e-9)
	assert.InDelta(t, 3.72, f.readSpin(nullID).ldr, 1e-9)
	assert.True(t, f.readSpin(nullID).actual, "NULL run_ldr spin is locked ACTUAL")
	var adj *float64
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT mbs_ldr_adjustment_pct FROM mst_mb_spin WHERE mbs_id = $1`, adjID).Scan(&adj))
	assert.Nil(t, adj, "adjustment cleared")

	// revert (all APPLIED results, oldest first) restores before-images incl. adjustment
	all := append(append([]fill.SpinResult{}, res...), one...)
	rev, err := fill.RevertSpinLDR(f.ctx, pg, "mbcmp-test", all)
	require.NoError(t, err)
	for _, r := range rev {
		assert.Contains(t, []string{fill.ResultReverted, fill.ResultAlready}, r.Result, r.Detail)
	}
	assert.InDelta(t, 1.87, f.readSpin(setID).ldr, 1e-9)
	assert.False(t, f.readSpin(nullID).actual)
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT mbs_ldr_adjustment_pct FROM mst_mb_spin WHERE mbs_id = $1`, adjID).Scan(&adj))
	require.NotNil(t, adj)
	assert.InDelta(t, 0.5, *adj, 1e-9)
}

// TestMBComparisonSpinLDR_IdempotentAndSuperseded: re-apply after commit = ALREADY_APPLIED; superseded spin = FAILED.
func TestMBComparisonSpinLDR_IdempotentAndSuperseded(t *testing.T) {
	f := newMBASFixture(t, "mbcmp_spinldr2")
	pg := postgres.NewMBComparisonSpinLDR(postgres.NewDBFromSQL(f.raw))
	h := f.head(true, "Y1")
	a := f.spin(h, "Y1", 1.87, true, nil)
	b := f.spin(h, "Y1", 1.87, true, nil)
	other := f.spin(h, "Y1", 1.87, true, nil)
	f.exec(`UPDATE mst_mb_spin SET mbs_ldr_is_actual = TRUE WHERE mbs_id = $1`, a)
	f.exec(`UPDATE mst_mb_spin SET mbs_ldr_is_actual = TRUE, mbs_superseded_by_spin_id = $2 WHERE mbs_id = $1`, b, other)
	mk := func(id string) fill.SpinRow {
		return fill.SpinRow{ID: id, Verdict: fill.VerdictSetLDR, RunLDR: f64p(1.87), IsActual: true, Distinct: 1, Min: f64p(2.28), Max: f64p(2.28), Target: 2.28, Decision: fill.SpinApplySet}
	}
	res, err := fill.ApplySpinLDR(f.ctx, pg, "mbcmp-test", []fill.SpinRow{mk(a.String()), mk(b.String())}, 0, 10, true)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultApplied, res[0].Result, res[0].Detail)
	assert.Equal(t, fill.ResultFailed, res[1].Result)
	assert.Equal(t, "spin_superseded", res[1].Detail)

	again, err := fill.ApplySpinLDR(f.ctx, pg, "mbcmp-test", []fill.SpinRow{mk(a.String())}, 0, 10, true)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultAlreadyApplied, again[0].Result, again[0].Detail)
}

// TestMBComparisonSpinAdjClear_Integration: scope (active+inactive, locked ACTUAL with adj), run_ldr kept, dry-run, guard, revert.
func TestMBComparisonSpinAdjClear_Integration(t *testing.T) {
	f := newMBASFixture(t, "mbcmp_adjclear")
	pg := postgres.NewMBComparisonSpinAdjClear(postgres.NewDBFromSQL(f.raw))
	h := f.head(true, "Y1")
	adjID := f.spin(h, "Y1", 2.28, true, nil)
	cleanID := f.spin(h, "Y1", 1.5, true, nil)
	f.exec(`UPDATE mst_mb_spin SET mbs_ldr_is_actual = TRUE, mbs_ldr_type = 'ACTUAL', mbs_ldr_calculated_pct = 2, mbs_ldr_adjustment_pct = 0.28 WHERE mbs_id = $1`, adjID)
	f.exec(`UPDATE mst_mb_spin SET mbs_ldr_is_actual = TRUE, mbs_ldr_type = 'ACTUAL', mbs_ldr_calculated_pct = 1.5, mbs_ldr_adjustment_pct = NULL WHERE mbs_id = $1`, cleanID)

	cand, err := pg.Candidates(f.ctx)
	require.NoError(t, err)
	var rows []fill.AdjRow
	for _, c := range cand {
		if c.ID == adjID.String() {
			rows = append(rows, c)
		}
		assert.NotEqual(t, cleanID.String(), c.ID, "clean spin is out of scope")
	}
	require.Len(t, rows, 1)
	plan := fill.PlanAdjClear(rows)
	require.Equal(t, fill.DecisionApply, plan[0].Decision)

	res, err := fill.ApplyAdjClear(f.ctx, pg, "mbcmp-test", plan, 0, 10, false)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultWouldApply, res[0].Result, res[0].Detail)
	assert.InDelta(t, 2.28, f.readSpin(adjID).ldr, 1e-9)

	f.exec(`UPDATE mst_mb_spin SET mbs_ldr_adjustment_pct = 0.1 WHERE mbs_id = $1`, adjID)
	res, err = fill.ApplyAdjClear(f.ctx, pg, "mbcmp-test", plan, 0, 10, true)
	require.NoError(t, err)
	assert.Equal(t, "changed_since_plan", res[0].Detail)
	f.exec(`UPDATE mst_mb_spin SET mbs_ldr_adjustment_pct = 0.28 WHERE mbs_id = $1`, adjID)

	res, err = fill.ApplyAdjClear(f.ctx, pg, "mbcmp-test", plan, 1, 10, true)
	require.NoError(t, err)
	require.Equal(t, fill.ResultApplied, res[0].Result, res[0].Detail)
	var adj, calc *float64
	read := func() {
		require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT mbs_ldr_adjustment_pct, mbs_ldr_calculated_pct FROM mst_mb_spin WHERE mbs_id = $1`, adjID).Scan(&adj, &calc))
	}
	read()
	assert.Nil(t, adj)
	require.NotNil(t, calc)
	assert.InDelta(t, 2.28, *calc, 1e-9)
	assert.InDelta(t, 2.28, f.readSpin(adjID).ldr, 1e-9, "run_ldr unchanged")

	rev, err := fill.RevertAdjClear(f.ctx, pg, "mbcmp-test", res)
	require.NoError(t, err)
	assert.Equal(t, fill.ResultReverted, rev[0].Result, rev[0].Detail)
	read()
	require.NotNil(t, adj)
	assert.InDelta(t, 0.28, *adj, 1e-9)
	assert.InDelta(t, 2, *calc, 1e-9)
}
