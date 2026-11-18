package postgres_test

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// Phase B writer items (design part 2 §5.6, §5.6a, §5.7) on a fresh migrated DB.

func (f *mbasFixture) spCodeParam() uuid.UUID {
	f.t.Helper()
	var id uuid.UUID
	require.NoError(f.t, f.raw.QueryRowContext(f.ctx,
		`SELECT id FROM mst_parameter WHERE param_code = 'MB_SP_CODE' AND deleted_at IS NULL LIMIT 1`).Scan(&id))
	return id
}

// refRow adds an MB_SP_CODE row on a fresh product: pointer when spin != nil, else text only.
func (f *mbasFixture) refRow(code, text string, spin *uuid.UUID) int64 {
	f.t.Helper()
	p := f.poy("REF-"+code, "")
	f.exec(`INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_value_mb_spin_id, cpp_filled_by, cpp_created_by)
		VALUES ($1, $2, $3, $4, 'auto_mb_source', 'itest')`, p, f.spCodeParam(), text, spin)
	return p
}

func (f *mbasFixture) setOrion(spin uuid.UUID, orion string) {
	f.exec(`UPDATE mst_mb_spin SET mbs_orion_item_code = $2 WHERE mbs_id = $1`, spin, orion)
}

type mbasSupState struct {
	active       bool
	supersededBy sql.NullString
}

func (f *mbasFixture) spinSup(id uuid.UUID) mbasSupState {
	f.t.Helper()
	var s mbasSupState
	require.NoError(f.t, f.raw.QueryRowContext(f.ctx,
		`SELECT mbs_is_active, CAST(mbs_superseded_by_spin_id AS text) FROM mst_mb_spin WHERE mbs_id = $1`, id).Scan(&s.active, &s.supersededBy))
	return s
}

func (f *mbasFixture) headSup(id uuid.UUID) mbasSupState {
	f.t.Helper()
	var s mbasSupState
	require.NoError(f.t, f.raw.QueryRowContext(f.ctx,
		`SELECT mbh_is_active, CAST(mbh_superseded_by_head_id AS text) FROM mst_mb_head WHERE mbh_id = $1`, id).Scan(&s.active, &s.supersededBy))
	return s
}

func (f *mbasFixture) pointerOf(product int64) string {
	f.t.Helper()
	var v sql.NullString
	require.NoError(f.t, f.raw.QueryRowContext(f.ctx, `
		SELECT CAST(c.cpp_value_mb_spin_id AS text) FROM cost_product_parameter c
		JOIN mst_parameter p ON p.id = c.cpp_param_id AND p.param_code = 'MB_SP_CODE'
		WHERE c.cpp_product_sys_id = $1`, product).Scan(&v))
	return v.String
}

func (f *mbasFixture) count(q string, args ...any) int {
	f.t.Helper()
	var n int
	require.NoError(f.t, f.raw.QueryRowContext(f.ctx, q, args...).Scan(&n))
	return n
}

// newTarget creates a live new head + spin with shade and FG (a row already applied).
func (f *mbasFixture) newTarget(tag, shade string) (head, spin uuid.UUID) {
	fg := f.poy("FG-"+tag, shade)
	head = f.head(true, shade)
	spin = f.spin(head, shade, 6, true, &fg)
	return head, spin
}

func requireResult(t *testing.T, res mbactivesync.ItemResult, err error, result, reason string) {
	t.Helper()
	require.NoError(t, err)
	require.Equal(t, result, res.Result, res.Message)
	require.Equal(t, reason, res.Reason, res.Message)
}

func TestMBActiveSyncWriter_SupersedeExisting(t *testing.T) {
	f := newMBASFixture(t, "mbas_writer_sup")
	const actor = "sync-tester"
	unsup := postgres.NewMBUnsupersedeRepository(postgres.NewDBFromSQL(f.raw))

	t.Run("repoints param with audit, old head deactivated, un-supersede reverses", func(t *testing.T) {
		newHead, newSpin := f.newTarget("S1", "S1")
		oldHead := f.head(true, "S-1")
		oldSpin := f.spin(oldHead, "S-1", 5, true, nil) // legacy: FG NULL
		f.setOrion(oldSpin, "ORI-OLD-S1")
		f.setOrion(newSpin, "ORI-NEW-S1")
		prod := f.refRow("S1", "ORI-OLD-S1", &oldSpin)

		in := mbactivesync.SupersedeInput{OldHeadID: oldHead, OldSpinID: oldSpin, NewSpinID: newSpin, Actor: actor}
		res, err := f.w.SupersedeExisting(f.ctx, in)
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		require.NotEmpty(t, res.BeforeJSON)
		require.NotEmpty(t, res.AfterJSON)
		var beforeImg map[string]any
		require.NoError(t, json.Unmarshal([]byte(res.BeforeJSON), &beforeImg))
		require.Equal(t, true, beforeImg["new_spin_is_active"], "new spin prior state recorded for rollback")
		require.Nil(t, beforeImg["new_spin_superseded_by_spin_id"])
		require.Equal(t, true, beforeImg["new_head_is_active"])
		require.Contains(t, res.Message, "ROLLBACK NOTE")
		require.Contains(t, res.Message, newSpin.String())

		require.Equal(t, newSpin.String(), f.pointerOf(prod), "MB_SP_CODE repointed to the new spin")
		require.Positive(t, f.count(`SELECT COUNT(*) FROM mst_mb_spin_supersede_item
			WHERE mssi_old_spin_id = $1 AND mssi_new_spin_id = $2 AND mssi_param_code = 'MB_SP_CODE'`, oldSpin, newSpin))
		os := f.spinSup(oldSpin)
		require.False(t, os.active)
		require.Equal(t, newSpin.String(), os.supersededBy.String)
		oh := f.headSup(oldHead)
		require.False(t, oh.active)
		require.Equal(t, newHead.String(), oh.supersededBy.String)
		require.Equal(t, 1, f.count(`SELECT COUNT(*) FROM mst_mb_workflow_log
			WHERE mbwl_mbh_id = $1 AND mbwl_to_state = 'SUPERSEDE' AND mbwl_meta->>'role' = 'OLD'`, oldHead))

		res, err = f.w.SupersedeExisting(f.ctx, in)
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyInTarget)

		// Existing Un-supersede reverses it (batch + OLD workflow log written by the writer).
		_, err = unsup.Unsupersede(f.ctx, oldHead, oldSpin, actor)
		require.NoError(t, err)
		os = f.spinSup(oldSpin)
		require.True(t, os.active)
		require.False(t, os.supersededBy.Valid)
		require.Equal(t, oldSpin.String(), f.pointerOf(prod), "param restored to the old spin")
		oh = f.headSup(oldHead)
		require.True(t, oh.active)
		require.False(t, oh.supersededBy.Valid)
		// KNOWN GAP (not the intended outcome): the new spin here is a pre-existing live
		// spin, but Un-supersede's swapSpinsTx deactivates it and links it to the old
		// spin. The operator must reactivate it afterwards from before_json
		// (new_spin_is_active / new_spin_superseded_by_spin_id); see ROLLBACK NOTE in res.Message.
		ns := f.spinSup(newSpin)
		require.False(t, ns.active, "known gap: Un-supersede deactivates the pre-existing new spin")
		require.Equal(t, oldSpin.String(), ns.supersededBy.String, "known gap: new spin linked to old spin")
	})

	t.Run("old head with 2 live spins is deactivated only by the second item", func(t *testing.T) {
		_, newA := f.newTarget("S2A", "S2A")
		newHeadB := f.head(true, "S2B")
		fgB := f.poy("FG-S2B", "S2B")
		newB := f.spin(newHeadB, "S2B", 6, true, &fgB)
		oldHead := f.head(true, "S2A")
		oldA := f.spin(oldHead, "S2A", 5, true, nil)
		oldB := f.spin(oldHead, "S2B", 5, true, nil)

		res, err := f.w.SupersedeExisting(f.ctx, mbactivesync.SupersedeInput{OldHeadID: oldHead, OldSpinID: oldA, NewSpinID: newA, Actor: actor})
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		require.True(t, f.headSup(oldHead).active, "a live spin remains")
		res, err = f.w.SupersedeExisting(f.ctx, mbactivesync.SupersedeInput{OldHeadID: oldHead, OldSpinID: oldB, NewSpinID: newB, Actor: actor})
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		oh := f.headSup(oldHead)
		require.False(t, oh.active)
		require.Equal(t, newHeadB.String(), oh.supersededBy.String)
	})

	t.Run("new spin without FG is not ready and nothing changes", func(t *testing.T) {
		nh := f.head(true, "S3")
		ns := f.spin(nh, "S3", 6, true, nil)
		oldHead := f.head(true, "S3")
		oldSpin := f.spin(oldHead, "S3", 5, true, nil)
		res, err := f.w.SupersedeExisting(f.ctx, mbactivesync.SupersedeInput{OldSpinID: oldSpin, NewSpinID: ns, Actor: actor})
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonNewSpinNotReady)
		require.True(t, f.spinSup(oldSpin).active)

		res, err = f.w.SupersedeExisting(f.ctx, mbactivesync.SupersedeInput{OldSpinID: oldSpin, NewSpinID: uuid.Nil, Actor: actor})
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonNewSpinNotReady)
	})

	t.Run("shade differing beyond format fails", func(t *testing.T) {
		_, ns := f.newTarget("S4", "S4")
		oldHead := f.head(true, "ZZ4")
		oldSpin := f.spin(oldHead, "ZZ4", 5, true, nil)
		res, err := f.w.SupersedeExisting(f.ctx, mbactivesync.SupersedeInput{OldSpinID: oldSpin, NewSpinID: ns, Actor: actor})
		requireResult(t, res, err, mbactivesync.ResultFailed, mbactivesync.ReasonShadeMismatch)
		require.True(t, f.spinSup(oldSpin).active)
	})

	t.Run("inactive old spin is a stale plan", func(t *testing.T) {
		_, ns := f.newTarget("S5", "S5")
		oldHead := f.head(true, "S5")
		oldSpin := f.spin(oldHead, "S5", 5, false, nil)
		res, err := f.w.SupersedeExisting(f.ctx, mbactivesync.SupersedeInput{OldSpinID: oldSpin, NewSpinID: ns, Actor: actor})
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonStalePlan)
	})
}

func TestMBActiveSyncWriter_SupersedeHeadOnly(t *testing.T) {
	f := newMBASFixture(t, "mbas_writer_suph")
	const actor = "sync-tester"

	t.Run("old head without live spin is superseded and deactivated", func(t *testing.T) {
		newHead := f.head(true, "H1")
		oldHead := f.head(true, "H1")
		dead := f.spin(oldHead, "H1", 5, false, nil)
		in := mbactivesync.SupersedeHeadInput{OldHeadID: oldHead, NewHeadID: newHead, Actor: actor}
		res, err := f.w.SupersedeHeadOnly(f.ctx, in)
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		oh := f.headSup(oldHead)
		require.False(t, oh.active)
		require.Equal(t, newHead.String(), oh.supersededBy.String)
		require.Equal(t, 1, f.count(`SELECT COUNT(*) FROM mst_mb_workflow_log
			WHERE mbwl_mbh_id = $1 AND mbwl_to_state = 'SUPERSEDE' AND mbwl_meta->>'role' = 'OLD'
			  AND mbwl_meta->>'kind' = 'SUPERSEDE_HEAD' AND mbwl_meta->>'new_spin_id' IS NULL`, oldHead))
		require.Zero(t, f.count(`SELECT COUNT(*) FROM mst_mb_spin_supersede_item WHERE mssi_old_spin_id = $1`, dead))
		var before map[string]any
		require.NoError(t, json.Unmarshal([]byte(res.BeforeJSON), &before))
		require.Equal(t, true, before["mbh_is_active"])

		res, err = f.w.SupersedeHeadOnly(f.ctx, in)
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyInTarget)
	})

	t.Run("inactive new head is not ready", func(t *testing.T) {
		newHead := f.head(false, "H2")
		oldHead := f.head(true, "H2")
		res, err := f.w.SupersedeHeadOnly(f.ctx, mbactivesync.SupersedeHeadInput{OldHeadID: oldHead, NewHeadID: newHead, Actor: actor})
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonNewHeadNotReady)
		require.True(t, f.headSup(oldHead).active)
	})

	t.Run("old head that gained a live spin is a stale plan", func(t *testing.T) {
		newHead := f.head(true, "H3")
		oldHead := f.head(true, "H3")
		f.spin(oldHead, "H3", 5, true, nil)
		res, err := f.w.SupersedeHeadOnly(f.ctx, mbactivesync.SupersedeHeadInput{OldHeadID: oldHead, NewHeadID: newHead, Actor: actor})
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonStalePlan)
		require.True(t, f.headSup(oldHead).active)
	})
}

func TestMBActiveSyncWriter_Deactivate(t *testing.T) {
	f := newMBASFixture(t, "mbas_writer_deact")
	const actor = "sync-tester"

	t.Run("spin with pointer refs is guarded", func(t *testing.T) {
		h := f.head(true, "D1")
		sp := f.spin(h, "D1", 5, true, nil)
		f.refRow("D1", "X", &sp)
		res, err := f.w.DeactivateSpin(f.ctx, mbactivesync.DeactivateSpinInput{SpinID: sp, Actor: actor})
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonHasRefs)
		require.True(t, f.spinSup(sp).active)
	})

	t.Run("spin with text-only refs on its ORION code is guarded", func(t *testing.T) {
		h := f.head(true, "D2")
		sp := f.spin(h, "D2", 5, true, nil)
		f.setOrion(sp, "ORI-D2")
		f.refRow("D2", " ori-d2 ", nil)
		res, err := f.w.DeactivateSpin(f.ctx, mbactivesync.DeactivateSpinInput{SpinID: sp, Actor: actor})
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonHasRefs)
	})

	t.Run("unreferenced spin is deactivated, idempotent, no shade cascade", func(t *testing.T) {
		h := f.head(true, "D3")
		sp := f.spin(h, "D3", 5, true, nil)
		sib := f.spin(h, "OTHER", 5, true, nil)
		in := mbactivesync.DeactivateSpinInput{SpinID: sp, Actor: actor}
		res, err := f.w.DeactivateSpin(f.ctx, in)
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		require.False(t, f.spinSup(sp).active)
		require.Equal(t, "D3", f.readSpin(sp).shade)
		require.True(t, f.spinSup(sib).active)
		ha, _, _ := f.readHead(h)
		require.True(t, ha, "head untouched")
		res, err = f.w.DeactivateSpin(f.ctx, in)
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyInTarget)
	})

	t.Run("head and all live spins deactivated in one tx, before_json lists them", func(t *testing.T) {
		poy := f.poy("P-DH", "D4")
		h := f.head(true, "D4")
		f.exec(`UPDATE mst_mb_head SET mbh_fg_product_id = $2, mbh_check_status = 'Locked' WHERE mbh_id = $1`, h, poy)
		s1 := f.spin(h, "D4", 5, true, nil)
		s2 := f.spin(h, "D4-X", 5, true, nil)
		dead := f.spin(h, "D4", 5, false, nil)
		in := mbactivesync.DeactivateHeadInput{HeadID: h, Actor: actor}
		res, err := f.w.DeactivateHead(f.ctx, in)
		requireResult(t, res, err, mbactivesync.ResultApplied, "")

		ha, hs, hfg := f.readHead(h)
		require.False(t, ha)
		require.Equal(t, "D4", hs, "head shade untouched")
		require.Equal(t, poy, hfg.Int64, "head FG untouched")
		require.False(t, f.spinSup(s1).active)
		require.False(t, f.spinSup(s2).active)
		require.Equal(t, "D4-X", f.readSpin(s2).shade, "no shade cascade")

		var before struct {
			Head struct {
				IsActive bool   `json:"mbh_is_active"`
				FG       *int64 `json:"mbh_fg_product_id"`
			} `json:"head"`
			Spins []struct {
				ID       uuid.UUID `json:"id"`
				IsActive bool      `json:"is_active"`
			} `json:"spins"`
		}
		require.NoError(t, json.Unmarshal([]byte(res.BeforeJSON), &before))
		require.True(t, before.Head.IsActive)
		require.Equal(t, poy, *before.Head.FG)
		ids := []uuid.UUID{}
		for _, s := range before.Spins {
			require.True(t, s.IsActive)
			ids = append(ids, s.ID)
		}
		require.ElementsMatch(t, []uuid.UUID{s1, s2}, ids, "only the spins this item deactivated")
		require.NotContains(t, ids, dead)

		res, err = f.w.DeactivateHead(f.ctx, in)
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyInTarget)
	})

	t.Run("head with a referenced live spin is guarded and unchanged", func(t *testing.T) {
		h := f.head(true, "D5")
		s1 := f.spin(h, "D5", 5, true, nil)
		s2 := f.spin(h, "D5", 5, true, nil)
		f.refRow("D5", "X", &s2)
		res, err := f.w.DeactivateHead(f.ctx, mbactivesync.DeactivateHeadInput{HeadID: h, Actor: actor})
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonHasRefs)
		ha, _, _ := f.readHead(h)
		require.True(t, ha)
		require.True(t, f.spinSup(s1).active)
	})
}

// Phase D (Q2, Q15): REACTIVATE (SyncExistingSpin + ACT_SPIN/ACT_HEAD) makes the new
// version live without cascade; the SUPERSEDE / SUPERSEDE_HEAD items then succeed.
func TestMBActiveSyncWriter_PhaseDReactivateThenSupersede(t *testing.T) {
	f := newMBASFixture(t, "mbas_writer_phased")
	const actor = "sync-tester"
	subs := []string{mbactivesync.SubSetFG, mbactivesync.SubActSpin, mbactivesync.SubActHead}

	t.Run("reactivate head without cascade then supersede spin", func(t *testing.T) {
		fg := f.poy("FG-D1", "D1")
		newHead := f.head(false, "D1")
		newSpin := f.spin(newHead, "D1", 6, false, nil)
		sibling := f.spin(newHead, "SIB", 6, false, nil)
		oldHead := f.head(true, "D1")
		oldSpin := f.spin(oldHead, "D1", 5, true, nil)
		sup := mbactivesync.SupersedeInput{OldHeadID: oldHead, OldSpinID: oldSpin, NewSpinID: newSpin, Actor: actor}

		res, err := f.w.SupersedeExisting(f.ctx, sup)
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultSkipped, res.Result, "new spin not live yet")
		require.True(t, f.spinSup(oldSpin).active)

		res, err = f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: newSpin, Actor: actor,
			SubActions: subs, TargetShadeCode: "D1", TargetFG: &fg})
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		require.True(t, f.spinSup(newSpin).active)
		require.True(t, f.headSup(newHead).active)
		require.False(t, f.spinSup(sibling).active, "no cascade to sibling spins")

		res, err = f.w.SupersedeExisting(f.ctx, sup)
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		require.Equal(t, newSpin.String(), f.spinSup(oldSpin).supersededBy.String)
		require.False(t, f.headSup(oldHead).active)
	})

	t.Run("reactivate head then supersede head only", func(t *testing.T) {
		newHead := f.head(false, "D2")
		sp := f.spin(newHead, "D2", 6, false, nil)
		oldHead := f.head(true, "D2")
		in := mbactivesync.SupersedeHeadInput{OldHeadID: oldHead, NewHeadID: newHead, Actor: actor}

		res, err := f.w.SupersedeHeadOnly(f.ctx, in)
		requireResult(t, res, err, mbactivesync.ResultSkipped, mbactivesync.ReasonNewHeadNotReady)

		res, err = f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: sp, Actor: actor,
			SubActions: []string{mbactivesync.SubActSpin, mbactivesync.SubActHead}, TargetShadeCode: "D2"})
		requireResult(t, res, err, mbactivesync.ResultApplied, "")

		res, err = f.w.SupersedeHeadOnly(f.ctx, in)
		requireResult(t, res, err, mbactivesync.ResultApplied, "")
		require.Equal(t, newHead.String(), f.headSup(oldHead).supersededBy.String)
	})
}
