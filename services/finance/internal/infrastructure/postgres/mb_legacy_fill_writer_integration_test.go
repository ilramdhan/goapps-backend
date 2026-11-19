package postgres_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func lfPlanRow(t *testing.T, e mblegacyfill.Entity, id uuid.UUID, field, goapps, legacy string) mblegacyfill.PlanRow {
	t.Helper()
	spec, ok := mblegacyfill.Lookup(e, field)
	require.True(t, ok, field)
	return mblegacyfill.PlanRow{
		Row:      mblegacyfill.Row{Entity: e, ID: id.String(), Field: field, GoappsValue: goapps, LegacyValue: legacy, Usulan: "OVERWRITE", Spec: spec},
		Class:    spec.Class,
		Decision: mblegacyfill.DecisionApply,
	}
}

func (f *mbasFixture) text(q string, args ...any) string {
	f.t.Helper()
	var s string
	require.NoError(f.t, f.raw.QueryRowContext(f.ctx, q, args...).Scan(&s))
	return s
}

func lfCaseLegacyFillWriter_ClassL(t *testing.T) {
	f := newLFFixture(t, "mblf_writer_l")
	w := postgres.NewMBLegacyFillWriter(postgres.NewDBFromSQL(f.raw))
	h := f.head(true, "")
	sp := f.spin(h, "", 5, true, nil)
	f.exec(`UPDATE mst_mb_head SET mbh_lesture = 'OLD', mbh_denier = 2.6 WHERE mbh_id = $1`, h)
	f.exec(`UPDATE mst_mb_spin SET mbs_ldr_prsn = 1.5 WHERE mbs_id = $1`, sp)

	rows := []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntitySpin, sp, "ldr_prsn", "1.50", "2.25"),
		lfPlanRow(t, mblegacyfill.EntityHead, h, "lesture", "OLD", "NEW"),
		lfPlanRow(t, mblegacyfill.EntityHead, h, "notes", "", "n1"),
		lfPlanRow(t, mblegacyfill.EntityHead, h, "d_f", "X", "Y"),   // CHANGED (current is empty)
		lfPlanRow(t, mblegacyfill.EntityHead, h, "status", "", ""),  // ALREADY (empty == empty legacy)
		lfPlanRow(t, mblegacyfill.EntityHead, h, "den", "2.6", "3"), // class D -> failed
	}
	res, err := w.Apply(f.ctx, "lf-tester", rows)
	require.NoError(t, err)
	require.Len(t, res, 6)
	by := map[string]mblegacyfill.RowResult{}
	for _, r := range res {
		by[string(r.Row.Entity)+"/"+r.Row.Field] = r
	}
	require.Equal(t, mblegacyfill.ResultApplied, by["head/lesture"].Result, by["head/lesture"].Reason)
	require.Contains(t, by["head/lesture"].BeforeJSON, "OLD")
	require.Contains(t, by["head/lesture"].AfterJSON, "NEW")
	require.Equal(t, mblegacyfill.ResultApplied, by["head/notes"].Result, by["head/notes"].Reason)
	require.Equal(t, "CHANGED", by["head/d_f"].Decision)
	require.Equal(t, mblegacyfill.ResultSkipped, by["head/d_f"].Result)
	require.Equal(t, mblegacyfill.ResultApplied, by["head/den"].Result, by["head/den"].Reason)
	require.Equal(t, mblegacyfill.ResultApplied, by["spin/ldr_prsn"].Result, by["spin/ldr_prsn"].Reason)

	require.Equal(t, "NEW", f.text(`SELECT mbh_lesture FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "n1", f.text(`SELECT notes FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "lf-tester", f.text(`SELECT updated_by FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "3.00", f.text(`SELECT CAST(mbh_denier AS text) FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "2.2500", f.text(`SELECT CAST(mbs_ldr_prsn AS text) FROM mst_mb_spin WHERE mbs_id = $1`, sp))

	// second run = ALREADY
	res, err = w.Apply(f.ctx, "lf-tester", rows[:3])
	require.NoError(t, err)
	for _, r := range res[:2] {
		require.Equal(t, "ALREADY", r.Decision, r.Row.Field)
		require.Equal(t, mblegacyfill.ResultSkipped, r.Result)
	}
}

func lfCaseLegacyFillWriter_ClassS(t *testing.T) {
	f := newLFFixture(t, "mblf_writer_s")
	w := postgres.NewMBLegacyFillWriter(postgres.NewDBFromSQL(f.raw))
	f.exec(`INSERT INTO cost_erp_shade (ces_shade_code, ces_shade_name, ces_shade_source) VALUES ('AX-7', 'Axe Seven', 'ORACLE')`)
	h1, h2 := f.head(true, ""), f.head(true, "")
	s1, s2 := f.spin(h1, "", 5, true, nil), f.spin(h2, "", 5, true, nil)

	res, err := w.Apply(f.ctx, "lf", []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntityHead, h1, "shade_code", "", "ax7"), // existing master (AX-7)
		lfPlanRow(t, mblegacyfill.EntitySpin, s1, "shade_code", "", "AX 7"),
		lfPlanRow(t, mblegacyfill.EntityHead, h2, "shade_code", "", "NEW-1"), // new -> MANUAL
		lfPlanRow(t, mblegacyfill.EntityHead, h2, "shade_name", "", "New One"),
		lfPlanRow(t, mblegacyfill.EntitySpin, s2, "shade_code", "", "NEW1"), // same nk, no second insert
	})
	require.NoError(t, err)
	for _, r := range res {
		require.Equal(t, mblegacyfill.ResultApplied, r.Result, r.Row.Field+": "+r.Reason)
	}
	require.Equal(t, "AX-7", f.text(`SELECT mbh_shade_code FROM mst_mb_head WHERE mbh_id = $1`, h1))
	require.Equal(t, "Axe Seven", f.text(`SELECT mbh_shade_name FROM mst_mb_head WHERE mbh_id = $1`, h1))
	require.Equal(t, "AX-7", f.text(`SELECT mbs_shade_code FROM mst_mb_spin WHERE mbs_id = $1`, s1))
	require.Equal(t, "NEW-1", f.text(`SELECT mbh_shade_code FROM mst_mb_head WHERE mbh_id = $1`, h2))
	require.Equal(t, "New One", f.text(`SELECT mbh_shade_name FROM mst_mb_head WHERE mbh_id = $1`, h2))
	require.Equal(t, "NEW-1", f.text(`SELECT mbs_shade_code FROM mst_mb_spin WHERE mbs_id = $1`, s2))
	require.Equal(t, "1", f.text(`SELECT CAST(COUNT(*) AS text) FROM cost_erp_shade WHERE UPPER(REPLACE(ces_shade_code,'-','')) = 'NEW1'`))
	require.Equal(t, "MANUAL", f.text(`SELECT ces_shade_source FROM cost_erp_shade WHERE ces_shade_code = 'NEW-1'`))
	require.Equal(t, "New One", f.text(`SELECT ces_shade_name FROM cost_erp_shade WHERE ces_shade_code = 'NEW-1'`))
}

func (f *mbasFixture) costProduct(h uuid.UUID) int64 {
	f.t.Helper()
	var pid int64
	require.NoError(f.t, f.raw.QueryRowContext(f.ctx, `INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_source, cpm_created_by, cpm_updated_by)
		VALUES ($1, (SELECT min(cpt_type_id) FROM cost_product_type), 'old name', 'MB_RECIPE', 'it', 'it') RETURNING cpm_product_sys_id`, "LF-"+h.String()[:8]).Scan(&pid))
	f.exec(`UPDATE mst_mb_head SET mbh_cost_product_id = $2 WHERE mbh_id = $1`, h, pid)
	return pid
}

func lfCaseLegacyFillWriter_ClassD_CascadeSpinWins(t *testing.T) {
	f := newLFFixture(t, "mblf_writer_d")
	w := postgres.NewMBLegacyFillWriter(postgres.NewDBFromSQL(f.raw))
	h := f.head(true, "")
	f.exec(`UPDATE mst_mb_head SET mbh_is_locked = TRUE WHERE mbh_id = $1`, h)
	pid := f.costProduct(h)
	s1, s2 := f.spin(h, "", 5, true, nil), f.spin(h, "", 5, true, nil)
	f.exec(`UPDATE mst_mb_head SET mbh_mgt_name = 'HM', mbh_denier = 380 WHERE mbh_id = $1`, h)
	oldCosting := f.text(`SELECT mbh_mb_costing FROM mst_mb_head WHERE mbh_id = $1`, h)
	f.exec(`UPDATE mst_mb_spin SET mbs_mb_costing = $2 WHERE mbs_mbh_id = $1`, h, oldCosting)
	f.exec(`UPDATE cost_product_master SET cpm_product_name = $2 WHERE cpm_product_sys_id = $1`, pid, oldCosting)

	res, err := w.Apply(f.ctx, "lf-d", []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntitySpin, s1, "den", "380", "999"), // spin wins over cascade
		lfPlanRow(t, mblegacyfill.EntityHead, h, "mb_costing", oldCosting, "NEW-COSTING"),
		lfPlanRow(t, mblegacyfill.EntityHead, h, "den", "380", "150"),
		lfPlanRow(t, mblegacyfill.EntitySpin, s1, "run_ldr", "5", "7.5"),
	})
	require.NoError(t, err)
	for _, r := range res {
		require.Equal(t, mblegacyfill.ResultApplied, r.Result, r.Row.Field+": "+r.Reason)
	}
	require.Equal(t, "150.00", f.text(`SELECT CAST(mbh_denier AS text) FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "VALIDATED", f.text(`SELECT mbh_entry_status FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "true", f.text(`SELECT CAST(mbh_is_locked AS text) FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "1", f.text(`SELECT CAST(mbh_current_version AS text) FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "NEW-COSTING", f.text(`SELECT cpm_product_name FROM cost_product_master WHERE cpm_product_sys_id = $1`, pid))
	require.Equal(t, "999.00", f.text(`SELECT CAST(mbs_denier AS text) FROM mst_mb_spin WHERE mbs_id = $1`, s1), "spin legacy wins")
	require.Equal(t, "150.00", f.text(`SELECT CAST(mbs_denier AS text) FROM mst_mb_spin WHERE mbs_id = $1`, s2), "cascaded")
	require.Equal(t, "NEW-COSTING", f.text(`SELECT mbs_mb_costing FROM mst_mb_spin WHERE mbs_id = $1`, s2))
	require.Equal(t, "ACTUAL", f.text(`SELECT mbs_ldr_type FROM mst_mb_spin WHERE mbs_id = $1`, s1))
	require.Equal(t, "true", f.text(`SELECT CAST(mbs_ldr_is_actual AS text) FROM mst_mb_spin WHERE mbs_id = $1`, s1))
	require.Contains(t, res[3].AfterJSON, "ACTUAL")
	require.Contains(t, res[1].SideEffects, s2.String())
	require.Contains(t, res[1].SideEffects, "cost_product_id")
}

func lfCaseLegacyFillWriter_ClassD_UniquenessReject(t *testing.T) {
	f := newLFFixture(t, "mblf_writer_d_uniq")
	w := postgres.NewMBLegacyFillWriter(postgres.NewDBFromSQL(f.raw))
	h1, h2 := f.head(true, ""), f.head(true, "")
	f.exec(`UPDATE mst_mb_head SET mbh_vs_number = 'VS-1' WHERE mbh_id = $1`, h1)

	res, err := w.Apply(f.ctx, "lf-d", []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntityHead, h2, "vs", "", "VS-1"),
		lfPlanRow(t, mblegacyfill.EntityHead, h2, "notes", "", "still applied"),
	})
	require.NoError(t, err)
	require.Equal(t, "REJECT", res[0].Decision)
	require.Equal(t, mblegacyfill.ResultSkipped, res[0].Result)
	require.Contains(t, res[0].Reason, "uniqueness")
	require.Equal(t, mblegacyfill.ResultApplied, res[1].Result, res[1].Reason)
	require.Equal(t, "", f.text(`SELECT COALESCE(mbh_vs_number, '') FROM mst_mb_head WHERE mbh_id = $1`, h2))
	require.Equal(t, "still applied", f.text(`SELECT notes FROM mst_mb_head WHERE mbh_id = $1`, h2))
}

func lfCaseLegacyFillWriter_ClassS_Guards(t *testing.T) {
	f := newLFFixture(t, "mblf_writer_s_guard")
	w := postgres.NewMBLegacyFillWriter(postgres.NewDBFromSQL(f.raw))
	f.exec(`INSERT INTO cost_erp_shade (ces_shade_code, ces_shade_name, ces_shade_source) VALUES ('AX-7', 'Axe Seven', 'ORACLE')`)
	h1, h2 := f.head(true, ""), f.head(true, "")
	s1, s2 := f.spin(h1, "", 5, true, nil), f.spin(h2, "", 5, true, nil)
	f.exec(`UPDATE mst_mb_head SET mbh_shade_code = 'OTHER' WHERE mbh_id = $1`, h1)
	f.exec(`UPDATE mst_mb_spin SET mbs_shade_code = 'AX-7' WHERE mbs_id = $1`, s2)

	res, err := w.Apply(f.ctx, "lf", []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntityHead, h1, "shade_code", "", "AX7"),  // CHANGED (current OTHER)
		lfPlanRow(t, mblegacyfill.EntitySpin, s2, "shade_code", "", "AX-7"), // ALREADY
		lfPlanRow(t, mblegacyfill.EntitySpin, s1, "shade_code", "", "AX 7"), // applied, same master
	})
	require.NoError(t, err)
	require.Equal(t, "CHANGED", res[0].Decision)
	require.Equal(t, mblegacyfill.ResultSkipped, res[0].Result)
	require.Equal(t, "ALREADY", res[1].Decision)
	require.Equal(t, mblegacyfill.ResultSkipped, res[1].Result)
	require.Equal(t, mblegacyfill.ResultApplied, res[2].Result, res[2].Reason)
	require.Equal(t, "OTHER", f.text(`SELECT mbh_shade_code FROM mst_mb_head WHERE mbh_id = $1`, h1))
	require.Equal(t, "AX-7", f.text(`SELECT mbs_shade_code FROM mst_mb_spin WHERE mbs_id = $1`, s1))
	require.Equal(t, "1", f.text(`SELECT CAST(COUNT(*) AS text) FROM cost_erp_shade WHERE UPPER(REPLACE(ces_shade_code,'-','')) = 'AX7'`))
}

func lfCaseLegacyFillWriter_SavepointIsolation(t *testing.T) {
	f := newLFFixture(t, "mblf_writer_sp")
	w := postgres.NewMBLegacyFillWriter(postgres.NewDBFromSQL(f.raw))
	h := f.head(true, "")
	sp := f.spin(h, "", 5, true, nil)
	f.exec(`UPDATE mst_mb_head SET mbh_ldr_prsn = 1 WHERE mbh_id = $1`, h)

	res, err := w.Apply(f.ctx, "lf", []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntityHead, h, "notes", "", "before-bad"),
		lfPlanRow(t, mblegacyfill.EntityHead, h, "ldr_prsn", "1", "not-a-number"), // cast error
		lfPlanRow(t, mblegacyfill.EntityHead, h, "lesture", "", "after-bad"),
		lfPlanRow(t, mblegacyfill.EntitySpin, sp, "notes", "", "spin-ok"),
	})
	require.NoError(t, err)
	require.Equal(t, mblegacyfill.ResultApplied, res[0].Result, res[0].Reason)
	require.Equal(t, mblegacyfill.ResultFailed, res[1].Result)
	require.NotEmpty(t, res[1].Reason)
	require.Equal(t, mblegacyfill.ResultApplied, res[2].Result, res[2].Reason)
	require.Equal(t, mblegacyfill.ResultApplied, res[3].Result, res[3].Reason)
	require.Equal(t, "before-bad", f.text(`SELECT notes FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "after-bad", f.text(`SELECT mbh_lesture FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "1.00", f.text(`SELECT CAST(mbh_ldr_prsn AS numeric(10,2)) FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "spin-ok", f.text(`SELECT notes FROM mst_mb_spin WHERE mbs_id = $1`, sp))
}

func lfCaseLegacyFillWriter_MissingAndDeletedSpin(t *testing.T) {
	f := newLFFixture(t, "mblf_writer_missing")
	w := postgres.NewMBLegacyFillWriter(postgres.NewDBFromSQL(f.raw))
	h := f.head(true, "")
	good, gone := f.spin(h, "", 5, true, nil), f.spin(h, "", 5, true, nil)
	f.exec(`UPDATE mst_mb_spin SET deleted_at = NOW() WHERE mbs_id = $1`, gone)

	res, err := w.Apply(f.ctx, "lf", []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntitySpin, gone, "notes", "", "x"),
		lfPlanRow(t, mblegacyfill.EntitySpin, uuid.New(), "notes", "", "x"),
		lfPlanRow(t, mblegacyfill.EntitySpin, good, "notes", "", "ok"),
	})
	require.NoError(t, err)
	for _, r := range res[:2] {
		require.Equal(t, mblegacyfill.ResultFailed, r.Result)
		require.Equal(t, mblegacyfill.ReasonNotFound, r.Reason)
	}
	require.Equal(t, mblegacyfill.ResultApplied, res[2].Result, res[2].Reason)
	require.Equal(t, "ok", f.text(`SELECT notes FROM mst_mb_spin WHERE mbs_id = $1`, good))
}

func lfCaseLegacyFillWriter_HeadFirstOrdering(t *testing.T) {
	f := newLFFixture(t, "mblf_writer_order")
	w := postgres.NewMBLegacyFillWriter(postgres.NewDBFromSQL(f.raw))
	h := f.head(true, "")
	s1, s2 := f.spin(h, "", 5, true, nil), f.spin(h, "", 5, true, nil)
	f.exec(`UPDATE mst_mb_head SET mbh_denier = 380 WHERE mbh_id = $1`, h)

	// spin row listed BEFORE the head row: head must still go first so the spin value wins.
	res, err := w.Apply(f.ctx, "lf", []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntitySpin, s1, "den", "380", "999"),
		lfPlanRow(t, mblegacyfill.EntityHead, h, "den", "380", "150"),
	})
	require.NoError(t, err)
	require.Len(t, res, 2)
	require.Equal(t, mblegacyfill.EntitySpin, res[0].Row.Entity, "results keep input order")
	for _, r := range res {
		require.Equal(t, mblegacyfill.ResultApplied, r.Result, r.Reason)
	}
	require.Equal(t, "999.00", f.text(`SELECT CAST(mbs_denier AS text) FROM mst_mb_spin WHERE mbs_id = $1`, s1))
	require.Equal(t, "150.00", f.text(`SELECT CAST(mbs_denier AS text) FROM mst_mb_spin WHERE mbs_id = $1`, s2))
}

func lfCaseLegacyFillWriter_ClassD_CodeSharedAndFgShadeGuard(t *testing.T) {
	f := newLFFixture(t, "mblf_writer_d_code")
	w := postgres.NewMBLegacyFillWriter(postgres.NewDBFromSQL(f.raw))
	h1, h2, h3 := f.head(true, ""), f.head(true, ""), f.head(true, "")
	f.exec(`UPDATE mst_mb_head SET mbh_code = 'C-1' WHERE mbh_id = $1`, h1)
	fg := f.costProduct(h3)
	f.exec(`UPDATE mst_mb_head SET mbh_fg_product_id = $2, mbh_shade_code = 'S9' WHERE mbh_id = $1`, h3, fg)

	res, err := w.Apply(f.ctx, "lf-d", []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntityHead, h2, "code", "", "C-1"), // shared with h1: allowed
		lfPlanRow(t, mblegacyfill.EntityHead, h3, "code", "", "C-2"), // FG head, shade S9: rejected
	})
	require.NoError(t, err)
	require.Equal(t, mblegacyfill.ResultApplied, res[0].Result, res[0].Reason)
	require.Equal(t, "C-1", f.text(`SELECT mbh_code FROM mst_mb_head WHERE mbh_id = $1`, h2))
	require.Equal(t, "REJECT", res[1].Decision)
	require.Contains(t, res[1].Reason, "fg shade")
	require.Equal(t, "", f.text(`SELECT COALESCE(mbh_code, '') FROM mst_mb_head WHERE mbh_id = $1`, h3))
}

func (f *mbasFixture) spinDen(id uuid.UUID) string {
	return f.text(`SELECT COALESCE(CAST(mbs_denier AS text), '') FROM mst_mb_spin WHERE mbs_id = $1`, id)
}

func lfCaseLegacyFillWriter_Targeted_DevOnlyTouchesNoSpin(t *testing.T) {
	f := newLFFixture(t, "mblf_tgt_dev")
	w := postgres.NewMBLegacyFillWriter(postgres.NewDBFromSQL(f.raw))
	h := f.head(true, "")
	s1 := f.spin(h, "", 5, true, nil)
	f.exec(`UPDATE mst_mb_head SET mbh_denier = 150 WHERE mbh_id = $1`, h) // head differs from spin: must NOT be pushed
	res, err := w.Apply(f.ctx, "lf", []mblegacyfill.PlanRow{lfPlanRow(t, mblegacyfill.EntityHead, h, "dev", "", "DEV-1")})
	require.NoError(t, err)
	require.Equal(t, mblegacyfill.ResultApplied, res[0].Result, res[0].Reason)
	require.Equal(t, "380.00", f.spinDen(s1))
	require.Equal(t, "144", f.text(`SELECT CAST(mbs_filament AS text) FROM mst_mb_spin WHERE mbs_id = $1`, s1))
	require.NotContains(t, res[0].SideEffects, s1.String())
}

func lfCaseLegacyFillWriter_Targeted_DenierFollowersOnly_AndRevert(t *testing.T) {
	f := newLFFixture(t, "mblf_tgt_den")
	db := postgres.NewDBFromSQL(f.raw)
	w, rv := postgres.NewMBLegacyFillWriter(db), postgres.NewMBLegacyFillReverter(db)
	h := f.head(true, "")
	f.exec(`UPDATE mst_mb_head SET mbh_denier = 380 WHERE mbh_id = $1`, h)
	a, b := f.spin(h, "", 5, true, nil), f.spin(h, "", 5, true, nil)
	f.exec(`UPDATE mst_mb_spin SET mbs_denier = 500 WHERE mbs_id = $1`, b)
	res, err := w.Apply(f.ctx, "lf", []mblegacyfill.PlanRow{lfPlanRow(t, mblegacyfill.EntityHead, h, "den", "380", "150")})
	require.NoError(t, err)
	require.Equal(t, mblegacyfill.ResultApplied, res[0].Result, res[0].Reason)
	require.Equal(t, "150.00", f.spinDen(a))
	require.Equal(t, "500.00", f.spinDen(b))

	out, err := rv.Revert(f.ctx, "lf-rev", lfToItems(res))
	require.NoError(t, err)
	require.Equal(t, mblegacyfill.RevertReverted, out[0].Result, out[0].Reason)
	require.Equal(t, "380.00", f.text(`SELECT CAST(mbh_denier AS text) FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "380.00", f.spinDen(a))
	require.Equal(t, "500.00", f.spinDen(b))
}

func lfCaseLegacyFillWriter_Targeted_CostingRename(t *testing.T) {
	f := newLFFixture(t, "mblf_tgt_cost")
	w := postgres.NewMBLegacyFillWriter(postgres.NewDBFromSQL(f.raw))
	h := f.head(true, "")
	pid := f.costProduct(h) // product name 'old name' != costing
	old := f.text(`SELECT mbh_mb_costing FROM mst_mb_head WHERE mbh_id = $1`, h)
	a, b := f.spin(h, "", 5, true, nil), f.spin(h, "", 5, true, nil)
	f.exec(`UPDATE mst_mb_spin SET mbs_mb_costing = $2 WHERE mbs_id = $1`, a, old)
	res, err := w.Apply(f.ctx, "lf", []mblegacyfill.PlanRow{lfPlanRow(t, mblegacyfill.EntityHead, h, "mb_costing", old, "NEW-C")})
	require.NoError(t, err)
	require.Equal(t, mblegacyfill.ResultApplied, res[0].Result, res[0].Reason)
	require.Equal(t, "NEW-C", f.text(`SELECT mbs_mb_costing FROM mst_mb_spin WHERE mbs_id = $1`, a))
	require.Equal(t, "SP-"+b.String()[:8], f.text(`SELECT mbs_mb_costing FROM mst_mb_spin WHERE mbs_id = $1`, b))
	require.Equal(t, "old name", f.text(`SELECT cpm_product_name FROM cost_product_master WHERE cpm_product_sys_id = $1`, pid))

	// now a product that does equal the old costing follows
	h2 := f.head(true, "")
	pid2 := f.costProduct(h2)
	old2 := f.text(`SELECT mbh_mb_costing FROM mst_mb_head WHERE mbh_id = $1`, h2)
	f.exec(`UPDATE cost_product_master SET cpm_product_name = $2 WHERE cpm_product_sys_id = $1`, pid2, old2)
	res, err = w.Apply(f.ctx, "lf", []mblegacyfill.PlanRow{lfPlanRow(t, mblegacyfill.EntityHead, h2, "mb_costing", old2, "NEW-D")})
	require.NoError(t, err)
	require.Equal(t, mblegacyfill.ResultApplied, res[0].Result, res[0].Reason)
	require.Equal(t, "NEW-D", f.text(`SELECT cpm_product_name FROM cost_product_master WHERE cpm_product_sys_id = $1`, pid2))
}

func lfCaseLegacyFillWriter_ClassD_SpinOrionUniquenessReject(t *testing.T) {
	f := newLFFixture(t, "mblf_writer_d_spin_orion")
	w := postgres.NewMBLegacyFillWriter(postgres.NewDBFromSQL(f.raw))
	h := f.head(true, "")
	s1, s2 := f.spin(h, "", 5, true, nil), f.spin(h, "", 5, true, nil)
	f.exec(`UPDATE mst_mb_spin SET mbs_orion_item_code = 'ORX-1' WHERE mbs_id = $1`, s1)

	res, err := w.Apply(f.ctx, "lf-d", []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntitySpin, s2, "orion", "", " ORX-1 "),
		lfPlanRow(t, mblegacyfill.EntitySpin, s2, "notes", "", "still applied"),
	})
	require.NoError(t, err)
	require.Equal(t, "REJECT", res[0].Decision)
	require.Contains(t, res[0].Reason, "uniqueness")
	require.Equal(t, mblegacyfill.ResultApplied, res[1].Result, res[1].Reason)
	require.Equal(t, "", f.text(`SELECT COALESCE(mbs_orion_item_code, '') FROM mst_mb_spin WHERE mbs_id = $1`, s2))
	require.Equal(t, "still applied", f.text(`SELECT notes FROM mst_mb_spin WHERE mbs_id = $1`, s2))
}
