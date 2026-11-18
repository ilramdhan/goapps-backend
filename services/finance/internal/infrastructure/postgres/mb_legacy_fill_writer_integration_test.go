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

func TestLegacyFillWriter_ClassL(t *testing.T) {
	f := newMBASFixture(t, "mblf_writer_l")
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
	require.Equal(t, mblegacyfill.ResultFailed, by["head/den"].Result)
	require.Equal(t, mblegacyfill.ReasonClassDNotImplemented, by["head/den"].Reason)
	require.Equal(t, mblegacyfill.ResultApplied, by["spin/ldr_prsn"].Result, by["spin/ldr_prsn"].Reason)

	require.Equal(t, "NEW", f.text(`SELECT mbh_lesture FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "n1", f.text(`SELECT notes FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "lf-tester", f.text(`SELECT updated_by FROM mst_mb_head WHERE mbh_id = $1`, h))
	require.Equal(t, "2.60", f.text(`SELECT CAST(mbh_denier AS text) FROM mst_mb_head WHERE mbh_id = $1`, h), "class D untouched")
	require.Equal(t, "2.2500", f.text(`SELECT CAST(mbs_ldr_prsn AS text) FROM mst_mb_spin WHERE mbs_id = $1`, sp))

	// second run = ALREADY
	res, err = w.Apply(f.ctx, "lf-tester", rows[:3])
	require.NoError(t, err)
	for _, r := range res[:2] {
		require.Equal(t, "ALREADY", r.Decision, r.Row.Field)
		require.Equal(t, mblegacyfill.ResultSkipped, r.Result)
	}
}

func TestLegacyFillWriter_ClassS(t *testing.T) {
	f := newMBASFixture(t, "mblf_writer_s")
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
