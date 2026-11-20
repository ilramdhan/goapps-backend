package postgres_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
)

// Residual fix 2026-11-20: explicit head-shade decisions from the planner and the
// backfill head-FG guard counting inactive roots (fg_follows_recipe ordering defect).
func TestMBActiveSyncWriter_HeadShadeDecisions(t *testing.T) {
	f := newMBASFixture(t, "mbas_head_shade")
	const actor = "sync-tester"

	t.Run("KEEP_HEAD_SHADE: root spin shade written, head untouched", func(t *testing.T) {
		poy := f.poy("P-KHS", "KB1")
		h := f.head(true, "KA1")
		root := f.spin(h, "KA1", 6, true, nil)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: root, Actor: actor,
			SubActions: subs(mbactivesync.SubSetFG, mbactivesync.SubSetShade, mbactivesync.SubKeepHeadShade), TargetShadeCode: "KB1", TargetFG: &poy})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		require.Equal(t, "KB1", f.readSpin(root).shade)
		_, hs, _ := f.readHead(h)
		require.Equal(t, "KA1", hs, "head shade kept")
	})

	t.Run("FORCE_HEAD_SHADE: head shade set without cascade, revert restores", func(t *testing.T) {
		poy := f.poy("P-FHS", "FB1")
		h := f.head(true, "FA1")
		root := f.spin(h, "FA1", 6, true, nil)
		sib := f.spin(h, "FA1", 6, true, nil)
		f.exec(`UPDATE mst_mb_spin SET mbs_parent_spin_id = $2 WHERE mbs_id = $1`, sib, root)
		before := f.full(root, h)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: root, Actor: actor,
			SubActions:      subs(mbactivesync.SubSetFG, mbactivesync.SubSetShade, mbactivesync.SubSetHeadShade, mbactivesync.SubForceHeadShade),
			TargetShadeCode: "FB1", TargetFG: &poy})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		_, hs, _ := f.readHead(h)
		require.Equal(t, "FB1", hs)
		require.Equal(t, "FB1", f.readSpin(root).shade)
		require.Equal(t, "FA1", f.readSpin(sib).shade, "no cascade")

		rr, err := f.w.Revert(f.ctx, mbactivesync.RevertInput{Key: "row:1", Action: "UPDATE_SHADE", Phase: mbactivesync.PhaseA,
			SpinID: root, HeadID: h, BeforeJSON: res.BeforeJSON, AfterJSON: res.AfterJSON, Actor: "r"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, rr.Result, rr.Message)
		got := f.full(root, h)
		require.Equal(t, before.headSh, got.headSh, "head shade restored")
		require.False(t, f.readSpin(root).fg.Valid, "FG restored")
	})

	t.Run("without explicit sub the guard still skips", func(t *testing.T) {
		poy := f.poy("P-GHS", "GB1")
		h := f.head(true, "GA1")
		root := f.spin(h, "GA1", 6, true, nil)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: root, Actor: actor,
			SubActions: subs(mbactivesync.SubSetFG, mbactivesync.SubSetShade), TargetShadeCode: "GB1", TargetFG: &poy})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ReasonHeadShadeDiffers, res.Reason)
	})

	t.Run("backfill does not set head FG while another (inactive) root exists", func(t *testing.T) {
		poy1, poy2 := f.poy("P-MR1", "MR1"), f.poy("P-MR2", "MR1")
		h := f.head(true, "MR1")
		r1 := f.spin(h, "MR1", 6, true, nil)
		r2 := f.spin(h, "MR1", 6, false, nil)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: r1, Actor: actor,
			SubActions: subs(mbactivesync.SubSetFG), TargetShadeCode: "MR1", TargetFG: &poy1})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		_, _, hfg := f.readHead(h)
		require.False(t, hfg.Valid, "head FG stays empty: head has two roots")

		res, err = f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: r2, Actor: actor,
			SubActions: subs(mbactivesync.SubSetFG, mbactivesync.SubActSpin), TargetShadeCode: "MR1", TargetFG: &poy2})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, "second root no longer fails fg_follows_recipe: %s", res.Message)
	})

	t.Run("FORCE moves mbh_code that held the old shade; revert restores it", func(t *testing.T) {
		poy := f.poy("P-FCD", "FD2")
		h := f.head(true, "FD-1")
		f.exec(`UPDATE mst_mb_head SET mbh_code = 'FD1' WHERE mbh_id = $1`, h)
		root := f.spin(h, "FD-1", 6, true, nil)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: root, Actor: actor,
			SubActions:      subs(mbactivesync.SubSetFG, mbactivesync.SubSetShade, mbactivesync.SubSetHeadShade, mbactivesync.SubForceHeadShade),
			TargetShadeCode: "FD2", TargetFG: &poy, PlanHeadShadeCode: "FD-1"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		var code string
		require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT mbh_code FROM mst_mb_head WHERE mbh_id = $1`, h).Scan(&code))
		require.Equal(t, "FD2", code)

		rr, err := f.w.Revert(f.ctx, mbactivesync.RevertInput{Key: "row:9", Action: "UPDATE_SHADE", Phase: mbactivesync.PhaseA,
			SpinID: root, HeadID: h, BeforeJSON: res.BeforeJSON, AfterJSON: res.AfterJSON, Actor: "r"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, rr.Result, rr.Message)
		require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT mbh_code FROM mst_mb_head WHERE mbh_id = $1`, h).Scan(&code))
		require.Equal(t, "FD1", code)
	})

	t.Run("head shade changed since plan → stale_plan, nothing written", func(t *testing.T) {
		poy := f.poy("P-STL", "SB1")
		h := f.head(true, "SA2")
		root := f.spin(h, "SA2", 6, true, nil)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: root, Actor: actor,
			SubActions:      subs(mbactivesync.SubSetFG, mbactivesync.SubSetShade, mbactivesync.SubSetHeadShade, mbactivesync.SubForceHeadShade),
			TargetShadeCode: "SB1", TargetFG: &poy, PlanHeadShadeCode: "SA1"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ReasonStalePlan, res.Reason)
		_, hs, _ := f.readHead(h)
		require.Equal(t, "SA2", hs)
	})
}
