package postgres_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func lfToItems(res []mblegacyfill.RowResult) []mblegacyfill.ResultRow {
	out := make([]mblegacyfill.ResultRow, len(res))
	for i, r := range res {
		out[i] = mblegacyfill.ResultRow{File: 0, Line: i + 2, Entity: r.Row.Entity, ID: r.Row.ID, Field: r.Row.Field,
			Result: r.Result, BeforeJSON: r.BeforeJSON, AfterJSON: r.AfterJSON, SideEffects: r.SideEffects}
	}
	return out
}

func lfCaseLegacyFillRevert_RestoresAndIdempotent(t *testing.T) {
	f := newLFFixture(t, "mblf_revert")
	db := postgres.NewDBFromSQL(f.raw)
	w, rv := postgres.NewMBLegacyFillWriter(db), postgres.NewMBLegacyFillReverter(db)
	h := f.head(true, "")
	f.exec(`UPDATE mst_mb_head SET mbh_is_locked = TRUE WHERE mbh_id = $1`, h)
	pid := f.costProduct(h)
	s1, s2 := f.spin(h, "", 5, true, nil), f.spin(h, "", 5, true, nil)
	f.exec(`UPDATE mst_mb_head SET mbh_lesture = 'OLD' WHERE mbh_id = $1`, h)
	oldCosting := f.text(`SELECT mbh_mb_costing FROM mst_mb_head WHERE mbh_id = $1`, h)
	f.exec(`UPDATE mst_mb_spin SET mbs_mb_costing = $2 WHERE mbs_mbh_id = $1`, h, oldCosting)
	f.exec(`UPDATE cost_product_master SET cpm_product_name = $2 WHERE cpm_product_sys_id = $1`, pid, oldCosting)
	oldS1 := f.text(`SELECT CAST(mbs_denier AS text) FROM mst_mb_spin WHERE mbs_id = $1`, s1)
	oldS2 := f.text(`SELECT COALESCE(CAST(mbs_denier AS text), '') FROM mst_mb_spin WHERE mbs_id = $1`, s2)
	oldS2Costing := f.text(`SELECT COALESCE(mbs_mb_costing, '') FROM mst_mb_spin WHERE mbs_id = $1`, s2)
	oldName := f.text(`SELECT cpm_product_name FROM cost_product_master WHERE cpm_product_sys_id = $1`, pid)

	res, err := w.Apply(f.ctx, "lf", []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntityHead, h, "lesture", "OLD", "NEW"),
		lfPlanRow(t, mblegacyfill.EntityHead, h, "mb_costing", oldCosting, "NEW-COSTING"),
		lfPlanRow(t, mblegacyfill.EntityHead, h, "den", "", "150"),
		lfPlanRow(t, mblegacyfill.EntitySpin, s1, "den", oldS1, "999"),
		lfPlanRow(t, mblegacyfill.EntitySpin, s1, "run_ldr", "5", "7.5"),
	})
	require.NoError(t, err)
	for _, r := range res {
		require.Equal(t, mblegacyfill.ResultApplied, r.Result, r.Row.Field+": "+r.Reason)
	}
	require.Equal(t, "NEW-COSTING", f.text(`SELECT mbs_mb_costing FROM mst_mb_spin WHERE mbs_id = $1`, s2))
	items := lfToItems(res)

	out, err := rv.Revert(f.ctx, "lf-rev", items)
	require.NoError(t, err)
	for _, r := range out {
		require.Equal(t, mblegacyfill.RevertReverted, r.Result, r.Item.Field+": "+r.Reason)
	}
	require.Equal(t, "OLD", f.text(`SELECT mbh_lesture FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, oldCosting, f.text(`SELECT mbh_mb_costing FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "", f.text(`SELECT COALESCE(CAST(mbh_denier AS text), '') FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, oldS1, f.text(`SELECT CAST(mbs_denier AS text) FROM mst_mb_spin WHERE mbs_id = $1`, s1))
	require.Equal(t, oldS2, f.text(`SELECT COALESCE(CAST(mbs_denier AS text), '') FROM mst_mb_spin WHERE mbs_id = $1`, s2))
	require.Equal(t, oldS2Costing, f.text(`SELECT COALESCE(mbs_mb_costing, '') FROM mst_mb_spin WHERE mbs_id = $1`, s2))
	require.Equal(t, oldName, f.text(`SELECT cpm_product_name FROM cost_product_master WHERE cpm_product_sys_id = $1`, pid))
	require.Equal(t, "false", f.text(`SELECT CAST(mbs_ldr_is_actual AS text) FROM mst_mb_spin WHERE mbs_id = $1`, s1))
	require.NotEqual(t, "ACTUAL", f.text(`SELECT mbs_ldr_type FROM mst_mb_spin WHERE mbs_id = $1`, s1))
	require.Equal(t, "VALIDATED", f.text(`SELECT mbh_entry_status FROM mst_mb_head WHERE mbh_id = $1`, h))

	again, err := rv.Revert(f.ctx, "lf-rev", items)
	require.NoError(t, err)
	for _, r := range again {
		require.Equal(t, mblegacyfill.RevertAlreadyReverted, r.Result, r.Item.Field+": "+r.Reason)
	}
}

func lfCaseLegacyFillRevert_ChangedSinceApply(t *testing.T) {
	f := newLFFixture(t, "mblf_revert_chg")
	db := postgres.NewDBFromSQL(f.raw)
	w, rv := postgres.NewMBLegacyFillWriter(db), postgres.NewMBLegacyFillReverter(db)
	h := f.head(true, "")
	sp := f.spin(h, "", 5, true, nil)
	f.exec(`UPDATE mst_mb_head SET mbh_lesture = 'OLD' WHERE mbh_id = $1`, h)
	res, err := w.Apply(f.ctx, "lf", []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntityHead, h, "lesture", "OLD", "NEW"),
		lfPlanRow(t, mblegacyfill.EntityHead, h, "notes", "", "n1"),
		lfPlanRow(t, mblegacyfill.EntitySpin, sp, "ldr_prsn", "", "2.5"),
	})
	require.NoError(t, err)
	f.exec(`UPDATE mst_mb_head SET mbh_lesture = 'HUMAN' WHERE mbh_id = $1`, h)

	out, err := rv.Revert(f.ctx, "lf-rev", lfToItems(res))
	require.NoError(t, err)
	by := map[string]mblegacyfill.RevertResult{}
	for _, r := range out {
		by[r.Item.Field] = r
	}
	require.Equal(t, mblegacyfill.RevertChangedSince, by["lesture"].Result)
	require.Equal(t, mblegacyfill.RevertReverted, by["notes"].Result)
	require.Equal(t, mblegacyfill.RevertReverted, by["ldr_prsn"].Result)
	require.Equal(t, "HUMAN", f.text(`SELECT mbh_lesture FROM mst_mb_head WHERE mbh_id = $1`, h), "untouched")
	require.Equal(t, "", f.text(`SELECT COALESCE(notes, '') FROM mst_mb_head WHERE mbh_id = $1`, h))
}

func lfCaseLegacyFillRevert_NonAppliedAndShadeCount(t *testing.T) {
	f := newLFFixture(t, "mblf_revert_shade")
	db := postgres.NewDBFromSQL(f.raw)
	w, rv := postgres.NewMBLegacyFillWriter(db), postgres.NewMBLegacyFillReverter(db)
	h := f.head(true, "")
	res, err := w.Apply(f.ctx, "lf", []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntityHead, h, "shade_code", "", "ZZ-9"),
		lfPlanRow(t, mblegacyfill.EntityHead, h, "d_f", "X", "Y"), // CHANGED -> skipped
	})
	require.NoError(t, err)
	items := lfToItems(res)
	require.Equal(t, 1, mblegacyfill.CountShadeMastersCreated(items))
	out, err := rv.Revert(f.ctx, "lf-rev", items)
	require.NoError(t, err)
	require.Equal(t, mblegacyfill.RevertReverted, out[0].Result, out[0].Reason)
	require.Equal(t, mblegacyfill.RevertSkipped, out[1].Result)
	require.Equal(t, "", f.text(`SELECT COALESCE(mbh_shade_code, '') FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "1", f.text(`SELECT CAST(COUNT(*) AS text) FROM cost_erp_shade WHERE ces_shade_code = 'ZZ-9'`), "master kept")
	_ = uuid.Nil
}

func lfCaseLegacyFillRevert_CascadeTargetChangedIsPartial(t *testing.T) {
	f := newLFFixture(t, "mblf_revert_casc")
	db := postgres.NewDBFromSQL(f.raw)
	w, rv := postgres.NewMBLegacyFillWriter(db), postgres.NewMBLegacyFillReverter(db)
	h := f.head(true, "")
	s2 := f.spin(h, "", 5, true, nil)
	oldCosting := f.text(`SELECT mbh_mb_costing FROM mst_mb_head WHERE mbh_id = $1`, h)
	f.exec(`UPDATE mst_mb_spin SET mbs_mb_costing = $2 WHERE mbs_id = $1`, s2, oldCosting)
	res, err := w.Apply(f.ctx, "lf", []mblegacyfill.PlanRow{
		lfPlanRow(t, mblegacyfill.EntityHead, h, "mb_costing", oldCosting, "NEW-COSTING"),
	})
	require.NoError(t, err)
	require.Equal(t, mblegacyfill.ResultApplied, res[0].Result, res[0].Reason)
	require.Equal(t, "NEW-COSTING", f.text(`SELECT mbs_mb_costing FROM mst_mb_spin WHERE mbs_id = $1`, s2))
	f.exec(`UPDATE mst_mb_spin SET mbs_mb_costing = 'HUMAN' WHERE mbs_id = $1`, s2)

	out, err := rv.Revert(f.ctx, "lf-rev", lfToItems(res))
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, mblegacyfill.RevertChangedSince, out[0].Result)
	require.Equal(t, mblegacyfill.ReasonCascadePartial, out[0].Reason)
	require.Equal(t, oldCosting, f.text(`SELECT mbh_mb_costing FROM mst_mb_head WHERE mbh_id = $1`, h), "head restored")
	require.Equal(t, "HUMAN", f.text(`SELECT mbs_mb_costing FROM mst_mb_spin WHERE mbs_id = $1`, s2), "later edit kept")
}
