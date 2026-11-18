package postgres_test

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// planRows runs --plan against the test DB and returns the row plans by ln.
func (f *mbasFixture) planRows(rows []mbactivesync.FileRow) (*mbactivesync.Plan, map[int]*mbactivesync.RowPlan) {
	f.t.Helper()
	pl, err := mbactivesync.NewPlanner(postgres.NewMBActiveSyncSnapshotReader(postgres.NewDBFromSQL(f.raw))).Build(f.ctx, rows)
	require.NoError(f.t, err)
	by := map[int]*mbactivesync.RowPlan{}
	for _, p := range pl.Rows {
		by[p.Row.Ln] = p
	}
	return pl, by
}

// applyItems round-trips the plan through its CSVs exactly like --plan / --apply.
func (f *mbasFixture) applyItems(pl *mbactivesync.Plan) []*mbactivesync.ApplyItem {
	f.t.Helper()
	var rb, hb bytes.Buffer
	require.NoError(f.t, mbactivesync.WriteRowsCSV(&rb, pl.Rows))
	require.NoError(f.t, mbactivesync.WriteHeadsCSV(&hb, pl.Heads))
	rows, err := mbactivesync.ReadPlanCSV(&rb, mbactivesync.RowsRequiredColumns)
	require.NoError(f.t, err)
	heads, err := mbactivesync.ReadPlanCSV(&hb, mbactivesync.HeadsRequiredColumns)
	require.NoError(f.t, err)
	items, err := mbactivesync.LoadApplyItems(rows, heads)
	require.NoError(f.t, err)
	return items
}

// P2h-4c: a GENERATE_SPIN ROOT is born with the HEAD spec (auto-gen); phase C must move it
// to the ROW spec (den/fil/cross-section) so the re-plan of that row is KEEP, not a new
// DUPLICATE_SPIN (the oscillation seen on the full local apply). The head spec is untouched.
func TestMBActiveSync_PhaseCRootTakesRowSpec(t *testing.T) {
	f := newMBASFixture(t, "mbas_root_spec")
	fg := f.poy("POYRS-1", "SHRS1")
	head := f.draftHead(mbhead.StatusDraft, "SHRS1", "", &fg, false)
	f.exec(`UPDATE mst_mb_head SET mbh_denier = 400, mbh_filament = 96 WHERE mbh_id = $1`, head)
	var sysID string
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT mbh_oracle_sys_id FROM mst_mb_head WHERE mbh_id = $1`, head).Scan(&sysID))

	file, err := mbactivesync.ParseBytes([]byte("1,POYRS-1,POY 380/144/TBL,SHRS1,Shade RS1,MB1,DYE,\"7,5\"," + sysID + "\n"))
	require.NoError(t, err)

	pl, by := f.planRows(file)
	p := by[1]
	require.Equal(t, mbactivesync.ActionGenerateSpin, p.Action)
	require.Equal(t, mbactivesync.RoleRoot, p.Role)
	require.Equal(t, mbactivesync.DecisionApply, p.Decision, "%v", p.Reasons)
	require.NotNil(t, p.TargetDen)
	require.InDelta(t, 380, *p.TargetDen, 1e-9)
	require.Equal(t, 144, *p.TargetFil)
	require.Equal(t, "TBL", p.TargetCS)

	sink := &memResults{}
	_, err = mbactivesync.NewApplier(f.w).WithHeadWorkflow(newITHeadWorkflow(f)).
		Run(f.ctx, f.applyItems(pl), mbactivesync.ApplyOptions{Phase: mbactivesync.PhaseC, Actor: "itest", Batch: 10}, sink)
	require.NoError(t, err)
	require.Len(t, sink.out, 1)
	o := sink.out[0].Result
	require.Equal(t, mbactivesync.ResultApplied, o.Result, "%s %s", o.Reason, o.Message)
	root := uuid.MustParse(o.SpinID)

	var den float64
	var fil int
	var cs, shade string
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `
		SELECT mbs_denier, mbs_filament, COALESCE(mbs_cross_section, ''), COALESCE(mbs_shade_code, '')
		  FROM mst_mb_spin WHERE mbs_id = $1`, root).Scan(&den, &fil, &cs, &shade))
	require.InDelta(t, 380, den, 1e-9, "root takes the row denier, not the head one")
	require.Equal(t, 144, fil)
	require.Equal(t, "TBL", cs)
	require.Equal(t, "SHRS1", shade)
	var hden float64
	var hfil int
	require.NoError(t, f.raw.QueryRowContext(f.ctx, `SELECT mbh_denier, mbh_filament FROM mst_mb_head WHERE mbh_id = $1`, head).Scan(&hden, &hfil))
	require.InDelta(t, 400, hden, 1e-9, "no head cascade")
	require.Equal(t, 96, hfil)

	_, by = f.planRows(file)
	require.Equal(t, mbactivesync.ActionKeep, by[1].Action, "re-plan converges: %v %v", by[1].Reasons, by[1].SubActions)
	require.NotNil(t, by[1].Spin)
	require.Equal(t, root, by[1].Spin.ID)

	// Rerun of phase C on the same plan is idempotent.
	sink = &memResults{}
	_, err = mbactivesync.NewApplier(f.w).WithHeadWorkflow(newITHeadWorkflow(f)).
		Run(f.ctx, f.applyItems(pl), mbactivesync.ApplyOptions{Phase: mbactivesync.PhaseC, Actor: "itest", Batch: 10}, sink)
	require.NoError(t, err)
	require.Equal(t, mbactivesync.ResultSkipped, sink.out[0].Result.Result, sink.out[0].Result.Message)
	require.Len(t, f.headSpins(head), 1)

}
