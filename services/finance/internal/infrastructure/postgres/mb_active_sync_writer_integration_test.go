package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

type mbasFixture struct {
	t      *testing.T
	ctx    context.Context
	raw    *sql.DB
	w      *postgres.MBActiveSyncWriter
	poyTyp int64
}

func newMBASFixture(t *testing.T, dbName string) *mbasFixture {
	t.Helper()
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, dbName, "")
	m, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, m.Up(ctx))
	f := &mbasFixture{t: t, ctx: ctx, raw: raw, w: postgres.NewMBActiveSyncWriter(postgres.NewDBFromSQL(raw))}
	require.NoError(t, raw.QueryRowContext(ctx, `SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'POY'`).Scan(&f.poyTyp))
	return f
}

func (f *mbasFixture) exec(q string, args ...any) {
	f.t.Helper()
	_, err := f.raw.ExecContext(f.ctx, q, args...)
	require.NoError(f.t, err)
}

func (f *mbasFixture) poy(code, shade string) int64 {
	f.t.Helper()
	var id int64
	require.NoError(f.t, f.raw.QueryRowContext(f.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_erp_item_code, cpm_shade_code, cpm_is_active, cpm_created_by, cpm_updated_by)
		VALUES (CAST($1 AS text), $2, CAST($1 AS text), CAST($1 AS text), $3, TRUE, 'itest', 'itest') RETURNING cpm_product_sys_id`, code, f.poyTyp, shade).Scan(&id))
	return id
}

func (f *mbasFixture) head(active bool, shade string) uuid.UUID {
	f.t.Helper()
	id := uuid.New()
	f.exec(`INSERT INTO mst_mb_head (mbh_id, mbh_oracle_sys_id, mbh_mb_costing, mbh_is_active, mbh_check_status, mbh_current_version, mbh_entry_status, mbh_shade_code, created_by)
		VALUES ($1, CAST($2 AS text), CAST($2 AS text), $3, 'Valid', 1, 'VALIDATED', NULLIF($4, ''), 'itest')`, id, "SYS-"+id.String()[:8], active, shade)
	return id
}

func (f *mbasFixture) spin(head uuid.UUID, shade string, ldr float64, active bool, fg *int64) uuid.UUID {
	f.t.Helper()
	id := uuid.New()
	f.exec(`INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_denier, mbs_filament, mbs_shade_code, mbs_run_ldr_pct,
			mbs_is_active, mbs_fg_product_id, created_at, created_by)
		VALUES ($1, $2, CAST($3 AS text), CAST($3 AS text), 380, 144, NULLIF($4, ''), $5, $6, $7, NOW(), 'itest')`, id, head, "SP-"+id.String()[:8], shade, ldr, active, fg)
	return id
}

type mbasSpinRow struct {
	shade, ldrType string
	ldr            float64
	active, actual bool
	fg             sql.NullInt64
	fgSrc          sql.NullString
	fixed          sql.NullBool // NULL must survive a revert (migration 000486)
}

func (f *mbasFixture) readSpin(id uuid.UUID) mbasSpinRow {
	f.t.Helper()
	var r mbasSpinRow
	require.NoError(f.t, f.raw.QueryRowContext(f.ctx, `
		SELECT COALESCE(mbs_shade_code, ''), COALESCE(mbs_run_ldr_pct, 0), COALESCE(mbs_ldr_type, ''), COALESCE(mbs_ldr_is_actual, FALSE),
		       mbs_is_active, mbs_fg_product_id, mbs_fg_link_source, mbs_ldr_is_fixed
		  FROM mst_mb_spin WHERE mbs_id = $1`, id).Scan(&r.shade, &r.ldr, &r.ldrType, &r.actual, &r.active, &r.fg, &r.fgSrc, &r.fixed))
	return r
}

func (f *mbasFixture) readHead(id uuid.UUID) (active bool, shade string, fg sql.NullInt64) {
	f.t.Helper()
	require.NoError(f.t, f.raw.QueryRowContext(f.ctx,
		`SELECT mbh_is_active, COALESCE(mbh_shade_code, ''), mbh_fg_product_id FROM mst_mb_head WHERE mbh_id = $1`, id).Scan(&active, &shade, &fg))
	return
}

func subs(s ...string) []string { return s }

func TestMBActiveSyncWriter_SyncExistingSpin(t *testing.T) {
	f := newMBASFixture(t, "mbas_writer")
	f64 := func(v float64) *float64 { return &v }
	const actor = "sync-tester"

	t.Run("keep sets FG only and is idempotent", func(t *testing.T) {
		poy := f.poy("P-KEEP", "K1")
		h := f.head(true, "K1")
		sp := f.spin(h, "K1", 7.5, true, nil)
		in := mbactivesync.SyncSpinInput{SpinID: sp, Actor: actor, SubActions: subs(mbactivesync.SubSetFG), TargetShadeCode: "K1", TargetFG: &poy}
		res, err := f.w.SyncExistingSpin(f.ctx, in)
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		r := f.readSpin(sp)
		require.Equal(t, poy, r.fg.Int64)
		require.Equal(t, "BACKFILL", r.fgSrc.String)
		_, _, hfg := f.readHead(h)
		require.Equal(t, poy, hfg.Int64, "single root head FG is written by the backfill helper")
		require.NotEmpty(t, res.BeforeJSON)
		require.NotEmpty(t, res.AfterJSON)

		res, err = f.w.SyncExistingSpin(f.ctx, in)
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultSkipped, res.Result)
		require.Equal(t, mbactivesync.ReasonAlreadyInTarget, res.Reason)
	})

	t.Run("update LDR sets ACTUAL and fixed", func(t *testing.T) {
		poy := f.poy("P-LDR", "L1")
		h := f.head(true, "L1")
		sp := f.spin(h, "L1", 5, true, nil)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: sp, Actor: actor,
			SubActions: subs(mbactivesync.SubSetFG, mbactivesync.SubSetLDR), TargetShadeCode: "L1", TargetLDR: f64(8.25), TargetFG: &poy})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		r := f.readSpin(sp)
		require.InDelta(t, 8.25, r.ldr, 1e-9)
		require.Equal(t, "ACTUAL", r.ldrType)
		require.True(t, r.actual)
		require.Equal(t, poy, r.fg.Int64)
	})

	t.Run("update shade + FG + ROOT head shade format follows, no cascade to siblings", func(t *testing.T) {
		poy := f.poy("P-SHADE", "Y145T")
		h := f.head(true, "Y-145-T")
		root := f.spin(h, "Y-145-T", 6, true, nil)
		sib := f.spin(h, "Y-145-T", 6, true, nil)
		_, err := f.raw.ExecContext(f.ctx, `UPDATE mst_mb_spin SET mbs_parent_spin_id = $2 WHERE mbs_id = $1`, sib, root)
		require.NoError(t, err)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: root, Actor: actor,
			SubActions: subs(mbactivesync.SubSetFG, mbactivesync.SubSetShade, mbactivesync.SubSetHeadShade), TargetShadeCode: "Y145T", TargetFG: &poy})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		require.Equal(t, "Y145T", f.readSpin(root).shade)
		require.Equal(t, poy, f.readSpin(root).fg.Int64)
		_, hs, _ := f.readHead(h)
		require.Equal(t, "Y145T", hs, "head shade normalised so re-validate does not flip the root")
		require.Equal(t, "Y-145-T", f.readSpin(sib).shade, "no cascade to other spins")
	})

	t.Run("head shade differing beyond format is skipped", func(t *testing.T) {
		poy := f.poy("P-DIFF", "D1")
		h := f.head(true, "ZZZ")
		root := f.spin(h, "D1", 6, true, nil)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: root, Actor: actor,
			SubActions: subs(mbactivesync.SubSetFG, mbactivesync.SubSetHeadShade), TargetShadeCode: "D1", TargetFG: &poy})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultSkipped, res.Result)
		require.Equal(t, mbactivesync.ReasonHeadShadeDiffers, res.Reason)
		require.False(t, f.readSpin(root).fg.Valid)
	})

	t.Run("reactivate spin and head without cascade", func(t *testing.T) {
		poy := f.poy("P-REACT", "R1")
		h := f.head(false, "R1")
		root := f.spin(h, "R1", 6, false, nil)
		other := f.spin(h, "OLD", 6, true, nil)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: root, Actor: actor,
			SubActions: subs(mbactivesync.SubSetFG, mbactivesync.SubActSpin, mbactivesync.SubActHead), TargetShadeCode: "R1", TargetFG: &poy})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultApplied, res.Result, res.Message)
		require.True(t, f.readSpin(root).active)
		ha, hs, _ := f.readHead(h)
		require.True(t, ha)
		require.Equal(t, "R1", hs)
		require.Equal(t, "OLD", f.readSpin(other).shade, "no cascade")
	})

	t.Run("unique (shade, FG) conflict rolls back the whole item", func(t *testing.T) {
		poy := f.poy("P-UNQ", "U1")
		h1, h2 := f.head(true, "U1"), f.head(true, "U1")
		f.spin(h1, "U1", 6, true, &poy) // holder
		sp := f.spin(h2, "U-1", 5, true, nil)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: sp, Actor: actor,
			SubActions: subs(mbactivesync.SubSetFG, mbactivesync.SubSetShade, mbactivesync.SubSetLDR), TargetShadeCode: "U1", TargetLDR: f64(9), TargetFG: &poy})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultSkipped, res.Result, res.Message)
		require.Equal(t, mbactivesync.ReasonFGConflict, res.Reason)
		r := f.readSpin(sp)
		require.Equal(t, "U-1", r.shade, "shade rolled back")
		require.InDelta(t, 5.0, r.ldr, 1e-9, "LDR rolled back")
		require.False(t, r.fg.Valid)
	})

	t.Run("different existing FG fails and rolls back", func(t *testing.T) {
		poyOld := f.poy("P-OLD", "F1")
		poyNew := f.poy("P-NEW", "F1b")
		h := f.head(true, "F1")
		sp := f.spin(h, "F1", 5, true, &poyOld)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: sp, Actor: actor,
			SubActions: subs(mbactivesync.SubSetFG, mbactivesync.SubSetLDR), TargetShadeCode: "F1", TargetLDR: f64(9), TargetFG: &poyNew})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultFailed, res.Result, res.Message)
		require.Equal(t, mbactivesync.ReasonFGMismatch, res.Reason)
		require.InDelta(t, 5.0, f.readSpin(sp).ldr, 1e-9, "LDR rolled back")
		require.Equal(t, poyOld, f.readSpin(sp).fg.Int64, "FG untouched")
	})

	t.Run("root spin whose head FG differs fails fg_follows_recipe", func(t *testing.T) {
		poyHead := f.poy("P-HFG", "H1")
		poyNew := f.poy("P-HFG2", "H1b")
		h := f.head(true, "H1")
		f.exec(`UPDATE mst_mb_head SET mbh_fg_product_id = $2 WHERE mbh_id = $1`, h, poyHead)
		sp := f.spin(h, "H1", 5, true, nil)
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: sp, Actor: actor,
			SubActions: subs(mbactivesync.SubSetFG), TargetShadeCode: "H1", TargetFG: &poyNew})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultFailed, res.Result)
		require.Equal(t, mbactivesync.ReasonFGFollowsRecipe, res.Reason)
	})

	t.Run("stale spin fails", func(t *testing.T) {
		res, err := f.w.SyncExistingSpin(f.ctx, mbactivesync.SyncSpinInput{SpinID: uuid.New(), Actor: actor, SubActions: subs(mbactivesync.SubSetLDR), TargetLDR: f64(1)})
		require.NoError(t, err)
		require.Equal(t, mbactivesync.ResultFailed, res.Result)
		require.Equal(t, mbactivesync.ReasonStalePlan, res.Reason)
	})
}
