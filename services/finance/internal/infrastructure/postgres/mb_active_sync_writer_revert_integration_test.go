package postgres_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	appmbspin "github.com/mutugading/goapps-backend/services/finance/internal/application/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// Revert (design part 2 §8) on a fresh migrated DB.

func revertIn(action string, ph mbactivesync.Phase, spin, head uuid.UUID, res mbactivesync.ItemResult) mbactivesync.RevertInput {
	in := mbactivesync.RevertInput{Key: "k", Action: action, Phase: ph, BeforeJSON: res.BeforeJSON, AfterJSON: res.AfterJSON, Actor: "reverter"}
	in.SpinID, in.HeadID = spin, head
	return in
}

type mbasFull struct {
	spin     mbasSpinRow
	headAct  bool
	headSh   string
	headFG   int64
	headFGOk bool
}

func (f *mbasFixture) full(spin, head uuid.UUID) mbasFull {
	a, s, fg := f.readHead(head)
	return mbasFull{spin: f.readSpin(spin), headAct: a, headSh: s, headFG: fg.Int64, headFGOk: fg.Valid}
}

func TestMBActiveSyncWriter_Revert_Row(t *testing.T) {
	f := newMBASFixture(t, "mbas_writer_revert")
	f64 := func(v float64) *float64 { return &v }

	t.Run("after equals before incl head FG and is_active, then idempotent", func(t *testing.T) {
		poy := f.poy("P-RV1", "Y145T")
		h := f.head(false, "Y-145-T")
		sp := f.spin(h, "Y-145-T", 6, false, nil)
		before := f.full(sp, h)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: sp, Actor: "sync",
			SubActions:      subs(mbactivesync.SubSetFG, mbactivesync.SubSetShade, mbactivesync.SubSetLDR, mbactivesync.SubActSpin, mbactivesync.SubActHead, mbactivesync.SubSetHeadShade),
			TargetShadeCode: "Y145T", TargetLDR: f64(8.5), TargetFG: &poy})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		applied := f.full(sp, h)
		require.NotEqual(t, before, applied)
		require.True(t, applied.headAct)
		require.Equal(t, poy, applied.headFG)

		rr, err := f.w.Revert(f.ctx, revertIn("REACTIVATE", mbactivesync.PhaseD, sp, h, res))
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, rr.Result, rr.Message)
		require.Equal(t, before, f.full(sp, h), "spin AND head restored (is_active, FG, shade)")

		rr, err = f.w.Revert(f.ctx, revertIn("REACTIVATE", mbactivesync.PhaseD, sp, h, res))
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultSkipped, rr.Result)
		require.Equal(t, mbactivesync.ReasonAlreadyReverted, rr.Reason)
	})

	t.Run("changed_since_apply leaves everything untouched", func(t *testing.T) {
		poy := f.poy("P-RV2", "C1")
		h := f.head(true, "C1")
		sp := f.spin(h, "C1", 5, true, nil)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: sp, Actor: "sync",
			SubActions: subs(mbactivesync.SubSetFG, mbactivesync.SubSetLDR), TargetShadeCode: "C1", TargetLDR: f64(7), TargetFG: &poy})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		f.exec(`UPDATE mst_mb_spin SET mbs_run_ldr_pct = 9.9 WHERE mbs_id = $1`, sp) // a later change
		snap := f.full(sp, h)
		rr, err := f.w.Revert(f.ctx, revertIn("KEEP", mbactivesync.PhaseA, sp, h, res))
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultSkipped, rr.Result)
		require.Equal(t, mbactivesync.ReasonChangedSinceApply, rr.Reason)
		require.Equal(t, snap, f.full(sp, h), "nothing written, not even the FG group")
	})

	t.Run("only the groups the item changed are restored", func(t *testing.T) {
		h := f.head(true, "G1")
		sp := f.spin(h, "G1", 5, true, nil)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: sp, Actor: "sync",
			SubActions: subs(mbactivesync.SubSetLDR), TargetShadeCode: "G1", TargetLDR: f64(7)})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		f.exec(`UPDATE mst_mb_head SET mbh_is_active = FALSE WHERE mbh_id = $1`, h) // unrelated group moved later
		rr, err := f.w.Revert(f.ctx, revertIn("UPDATE_LDR", mbactivesync.PhaseA, sp, h, res))
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, rr.Result, rr.Message)
		require.InDelta(t, 5.0, f.readSpin(sp).ldr, 1e-9)
		a, _, _ := f.readHead(h)
		require.False(t, a, "head group untouched by the item stays as it is")
	})

	t.Run("supersede is not reverted by the tool", func(t *testing.T) {
		rr, err := f.w.Revert(f.ctx, mbactivesync.RevertInput{Key: "sup:x", Action: "SUPERSEDE", Phase: mbactivesync.PhaseB, Actor: "r"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultSkipped, rr.Result)
		require.Equal(t, mbactivesync.ReasonNotRevertable, rr.Reason)
		require.NotEmpty(t, rr.ManualReason)
	})

	t.Run("bad images fail", func(t *testing.T) {
		rr, err := f.w.Revert(f.ctx, mbactivesync.RevertInput{Key: "k", Action: "KEEP", Phase: mbactivesync.PhaseA, SpinID: uuid.New(), BeforeJSON: "x", AfterJSON: "y", Actor: "r"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultFailed, rr.Result)
	})
}

func TestMBActiveSyncWriter_Revert_DeactivateAndSupersedeHead(t *testing.T) {
	f := newMBASFixture(t, "mbas_writer_revert_head")

	t.Run("DeactivateHead revert restores head and exactly its spins", func(t *testing.T) {
		poy := f.poy("P-RDH", "D1")
		h := f.head(true, "D1")
		f.exec(`UPDATE mst_mb_head SET mbh_fg_product_id = $2 WHERE mbh_id = $1`, h, poy)
		s1 := f.spin(h, "D1", 5, true, nil)
		s2 := f.spin(h, "D1-X", 5, true, nil)
		dead := f.spin(h, "D1", 5, false, nil)
		res, err := f.w.DeactivateHead(f.ctx, mbactivesync.DeactivateHeadInput{HeadID: h, Actor: "sync"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		require.False(t, f.spinSup(s1).active)

		rr, err := f.w.Revert(f.ctx, revertIn("DEACTIVATE_HEAD", mbactivesync.PhaseB, uuid.Nil, h, res))
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, rr.Result, rr.Message)
		ha, _, hfg := f.readHead(h)
		require.True(t, ha)
		require.Equal(t, poy, hfg.Int64)
		require.True(t, f.spinSup(s1).active)
		require.True(t, f.spinSup(s2).active)
		require.False(t, f.spinSup(dead).active, "a spin the item did not deactivate stays inactive")

		rr, err = f.w.Revert(f.ctx, revertIn("DEACTIVATE_HEAD", mbactivesync.PhaseB, uuid.Nil, h, res))
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ReasonAlreadyReverted, rr.Reason)
	})

	t.Run("DeactivateHead revert skips a superseded spin", func(t *testing.T) {
		h := f.head(true, "D2")
		s1 := f.spin(h, "D2", 5, true, nil)
		res, err := f.w.DeactivateHead(f.ctx, mbactivesync.DeactivateHeadInput{HeadID: h, Actor: "sync"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result)
		other := f.spin(h, "D2b", 5, false, nil)
		f.exec(`UPDATE mst_mb_spin SET mbs_superseded_by_spin_id = $2 WHERE mbs_id = $1`, s1, other)
		rr, err := f.w.Revert(f.ctx, revertIn("DEACTIVATE_HEAD", mbactivesync.PhaseB, uuid.Nil, h, res))
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ReasonChangedSinceApply, rr.Reason)
		ha, _, _ := f.readHead(h)
		require.False(t, ha, "all-or-nothing")
	})

	t.Run("DeactivateSpin revert", func(t *testing.T) {
		h := f.head(true, "D3")
		sp := f.spin(h, "D3", 5, true, nil)
		res, err := f.w.DeactivateSpin(f.ctx, mbactivesync.DeactivateSpinInput{SpinID: sp, Actor: "sync"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		rr, err := f.w.Revert(f.ctx, revertIn("DEACTIVATE_SPIN", mbactivesync.PhaseB, sp, h, res))
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, rr.Result, rr.Message)
		require.True(t, f.spinSup(sp).active)
	})

	t.Run("SupersedeHead revert restores the old head", func(t *testing.T) {
		newHead := f.head(true, "H1")
		oldHead := f.head(true, "H1")
		f.spin(oldHead, "H1", 5, false, nil)
		res, err := f.w.SupersedeHeadOnly(f.ctx, mbactivesync.SupersedeHeadInput{OldHeadID: oldHead, NewHeadID: newHead, Actor: "sync"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		require.False(t, f.headSup(oldHead).active)

		rr, err := f.w.Revert(f.ctx, revertIn("SUPERSEDE_HEAD", mbactivesync.PhaseB, uuid.Nil, oldHead, res))
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, rr.Result, rr.Message)
		st := f.headSup(oldHead)
		require.True(t, st.active)
		require.False(t, st.supersededBy.Valid)
		var at, by any
		require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT mbh_superseded_at, mbh_superseded_by FROM mst_mb_head WHERE mbh_id = $1`, oldHead).Scan(&at, &by))
		require.Nil(t, at)
		require.Nil(t, by)

		rr, err = f.w.Revert(f.ctx, revertIn("SUPERSEDE_HEAD", mbactivesync.PhaseB, uuid.Nil, oldHead, res))
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ReasonAlreadyReverted, rr.Reason)

		// supersede again, then a later change blocks the revert
		res, err = f.w.SupersedeHeadOnly(f.ctx, mbactivesync.SupersedeHeadInput{OldHeadID: oldHead, NewHeadID: newHead, Actor: "sync"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result)
		f.exec(`UPDATE mst_mb_head SET mbh_superseded_by_head_id = $2 WHERE mbh_id = $1`, oldHead, oldHead)
		rr, err = f.w.Revert(f.ctx, revertIn("SUPERSEDE_HEAD", mbactivesync.PhaseB, uuid.Nil, oldHead, res))
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ReasonChangedSinceApply, rr.Reason)
	})
}

func TestMBActiveSyncWriter_Revert_CreatedSpin(t *testing.T) {
	f := newMBASFixture(t, "mbas_writer_revert_gen")

	t.Run("clone spin is deactivated, not deleted", func(t *testing.T) {
		h := f.head(true, "K1")
		clone := f.spin(h, "K1", 5, true, nil)
		rr, err := f.w.Revert(f.ctx, mbactivesync.RevertInput{Key: "row:1", Action: "DUPLICATE_SPIN", Phase: mbactivesync.PhaseC,
			SpinID: clone, CloneSpinID: clone, HeadID: h, BeforeJSON: "{}", AfterJSON: "{}", Actor: "r"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, rr.Result, rr.Message)
		require.False(t, f.spinSup(clone).active)
		require.Equal(t, 1, f.count(`SELECT COUNT(*) FROM mst_mb_spin WHERE mbs_id = $1 AND deleted_at IS NULL`, clone))

		rr, err = f.w.Revert(f.ctx, mbactivesync.RevertInput{Key: "row:1", Action: "DUPLICATE_SPIN", Phase: mbactivesync.PhaseC,
			SpinID: clone, CloneSpinID: clone, HeadID: h, Actor: "r"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ReasonAlreadyReverted, rr.Reason)
	})

	t.Run("referenced clone is listed for manual follow-up", func(t *testing.T) {
		h := f.head(true, "K2")
		clone := f.spin(h, "K2", 5, true, nil)
		f.refRow("K2", "K2", &clone)
		rr, err := f.w.Revert(f.ctx, mbactivesync.RevertInput{Key: "row:2", Action: "DUPLICATE_SPIN", Phase: mbactivesync.PhaseC,
			SpinID: clone, CloneSpinID: clone, HeadID: h, Actor: "r"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ReasonHasRefs, rr.Reason)
		require.Equal(t, mbactivesync.ReasonHasRefs, rr.ManualReason)
		require.True(t, f.spinSup(clone).active)
	})

	t.Run("generated root spin (nested ensure/sync image) is deactivated", func(t *testing.T) {
		h := f.head(true, "K3")
		root := f.spin(h, "K3", 5, true, nil)
		before := `{"ensure":{"head_id":"` + h.String() + `","root_spin_id":null},"sync":{}}`
		rr, err := f.w.Revert(f.ctx, mbactivesync.RevertInput{Key: "row:3", Action: "GENERATE_SPIN", Phase: mbactivesync.PhaseC,
			SpinID: root, HeadID: h, BeforeJSON: before, AfterJSON: `{"ensure":{},"sync":{}}`, Actor: "r"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, rr.Result, rr.Message)
		require.False(t, f.spinSup(root).active)
	})
}

func TestMBActiveSyncWriter_Revert_CreatedSpinRestoresHead(t *testing.T) {
	f := newMBASFixture(t, "mbas_writer_revert_head")
	f64 := func(v float64) *float64 { return &v }

	t.Run("real DuplicateToTarget on an inactive head: revert restores head active/FG/shade", func(t *testing.T) {
		db := postgres.NewDBFromSQL(f.raw)
		w := postgres.NewMBActiveSyncWriter(db).WithDuplicator(itDuplicator{h: appmbspin.NewDuplicateHandler(postgres.NewMBSpinRepository(db), nil)})
		poy := f.poy("P-RVH1", "RVH1B")
		h := f.head(false, "RVH1A")
		src := f.spin(h, "RVH1A", 6, true, nil)
		beforeHead := f.full(src, h)
		res, err := w.DuplicateToTarget(f.ctx, mbactivesync.DuplicateTargetInput{
			SourceSpinID: src,
			Target: mbactivesync.SyncSpinInput{Actor: "sync", SubActions: subs(mbactivesync.SubSetFG, mbactivesync.SubSetShade, mbactivesync.SubActHead, mbactivesync.SubSetHeadShade),
				TargetShadeCode: "RVH1B", TargetFG: &poy, TargetLDR: f64(7)},
		})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		clone := uuid.MustParse(res.CloneSpinID)
		require.True(t, f.full(clone, h).headAct, "tx 2 reactivated the head")

		rr, err := w.Revert(f.ctx, mbactivesync.RevertInput{Key: "row:1", Action: "DUPLICATE_SPIN", Phase: mbactivesync.PhaseC,
			SpinID: clone, CloneSpinID: clone, HeadID: h, BeforeJSON: res.BeforeJSON, AfterJSON: res.AfterJSON, Actor: "r"})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, rr.Result, rr.Message)
		require.False(t, f.spinSup(clone).active)
		got := f.full(src, h)
		require.Equal(t, beforeHead.headAct, got.headAct, "head is_active restored")
		require.Equal(t, beforeHead.headFG, got.headFG)
		require.Equal(t, beforeHead.headSh, got.headSh)
	})

	t.Run("created root spin with nested sync image: head restored; later head change blocks", func(t *testing.T) {
		poy := f.poy("P-RVH2", "RVH2")
		h := f.head(false, "")
		root := f.spin(h, "RVH2", 5, true, nil)
		hb := f.full(root, h)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: root, Actor: "sync",
			SubActions: subs(mbactivesync.SubSetFG, mbactivesync.SubActHead, mbactivesync.SubSetHeadShade), TargetShadeCode: "RVH2", TargetFG: &poy})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		gen := `{"head_id":"` + h.String() + `","root_spin_id":null}`
		before := `{"generate":` + gen + `,"sync":` + res.BeforeJSON + `}`
		after := `{"generate":{"head_id":"` + h.String() + `","root_spin_id":"` + root.String() + `"},"sync":` + res.AfterJSON + `}`
		in := mbactivesync.RevertInput{Key: "row:2", Action: "GENERATE_SPIN", Phase: mbactivesync.PhaseC, SpinID: root, HeadID: h,
			BeforeJSON: before, AfterJSON: after, Actor: "r"}

		f.exec(`UPDATE mst_mb_head SET mbh_shade_code = 'LATER' WHERE mbh_id = $1`, h)
		rr, err := f.w.Revert(f.ctx, in)
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ReasonChangedSinceApply, rr.Reason)
		require.True(t, f.spinSup(root).active, "nothing written")
		f.exec(`UPDATE mst_mb_head SET mbh_shade_code = 'RVH2' WHERE mbh_id = $1`, h)

		rr, err = f.w.Revert(f.ctx, in)
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, rr.Result, rr.Message)
		require.False(t, f.spinSup(root).active)
		got := f.full(root, h)
		require.Equal(t, hb.headAct, got.headAct)
		require.Equal(t, hb.headFG, got.headFG)
		require.Equal(t, hb.headFGOk, got.headFGOk)
		require.Equal(t, hb.headSh, got.headSh)
	})
}

func TestMBActiveSyncWriter_Revert_LDRKeepsNullIsFixed(t *testing.T) {
	f := newMBASFixture(t, "mbas_writer_revert_fixed")
	h := f.head(true, "NF1")
	sp := f.spin(h, "NF1", 6, true, nil)
	before := f.readSpin(sp)
	require.False(t, before.fixed.Valid, "fixture spin has NULL mbs_ldr_is_fixed")
	res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: sp, Actor: "sync",
		SubActions: subs(mbactivesync.SubSetLDR), TargetShadeCode: "NF1", TargetLDR: func() *float64 { v := 9.0; return &v }()})
	require.NoError(t, err)
	require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
	rr, err := f.w.Revert(f.ctx, revertIn("UPDATE_LDR", mbactivesync.PhaseA, sp, h, res))
	require.NoError(t, err)
	require.Equal(t, mbactivesync.ResultApplied, rr.Result, rr.Message)
	require.Equal(t, before, f.readSpin(sp), "ldr_is_fixed NULL, ldr_is_actual and LDR restored")
}
