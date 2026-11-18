package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	appmbspin "github.com/mutugading/goapps-backend/services/finance/internal/application/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// itDuplicator adapts the real DuplicateHandler like cmd/sync-mb-active does.
type itDuplicator struct{ h *appmbspin.DuplicateHandler }

func (d itDuplicator) Duplicate(ctx context.Context, req mbactivesync.DuplicateSpinRequest) (uuid.UUID, error) {
	res, err := d.h.Handle(ctx, appmbspin.DuplicateCommand{SourceSpinID: req.SourceSpinID, MgtName: req.MgtName,
		Denier: req.Denier, Filament: req.Filament, FgProductID: req.FgProductID, ActorUserID: req.Actor})
	if err != nil {
		return uuid.Nil, err
	}
	return res.Output.NewSpinID, nil
}

type memResults struct{ out []mbactivesync.ApplyOutcome }

func (m *memResults) Record(o mbactivesync.ApplyOutcome) error {
	m.out = append(m.out, o)
	return nil
}

func (m *memResults) byResult(r string) []mbactivesync.ApplyOutcome {
	var out []mbactivesync.ApplyOutcome
	for _, o := range m.out {
		if o.Result.Result == r {
			out = append(out, o)
		}
	}
	return out
}

func (f *mbasFixture) children(source uuid.UUID) []uuid.UUID {
	f.t.Helper()
	rows, err := f.raw.QueryContext(f.ctx, `SELECT mbs_id FROM mst_mb_spin WHERE mbs_parent_spin_id = $1 AND deleted_at IS NULL ORDER BY created_at, mbs_id`, source)
	require.NoError(f.t, err)
	defer func() { require.NoError(f.t, rows.Close()) }()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		require.NoError(f.t, rows.Scan(&id))
		out = append(out, id)
	}
	require.NoError(f.t, rows.Err())
	return out
}

func (f *mbasFixture) setPOYActive(id int64, active bool) {
	f.exec(`UPDATE cost_product_master SET cpm_is_active = $2 WHERE cpm_product_sys_id = $1`, id, active)
}

func (f *mbasFixture) spinName(id uuid.UUID) string {
	f.t.Helper()
	var n string
	require.NoError(f.t, f.raw.QueryRowContext(f.ctx, `SELECT mbs_mgt_name FROM mst_mb_spin WHERE mbs_id = $1`, id).Scan(&n))
	return n
}

func dupItem(ln int, sysid string, source uuid.UUID, shade string, fg int64, ldr float64, subsList ...string) *mbactivesync.ApplyItem {
	return &mbactivesync.ApplyItem{
		Kind: "row", Key: "row:" + uuid.NewString()[:4], Ln: ln, SysID: sysid, Action: string(mbactivesync.ActionDuplicateSpin),
		Phase: mbactivesync.PhaseC, Source: source.String(),
		Spin: mbactivesync.SyncSpinInput{SubActions: subsList, TargetShadeCode: shade, TargetFG: &fg, TargetLDR: &ldr},
	}
}

func TestMBActiveSyncWriter_DuplicateToTarget(t *testing.T) {
	f := newMBASFixture(t, "mbas_writer_dup")
	db := postgres.NewDBFromSQL(f.raw)
	w := postgres.NewMBActiveSyncWriter(db).WithDuplicator(itDuplicator{h: appmbspin.NewDuplicateHandler(postgres.NewMBSpinRepository(db), nil)})
	ap := mbactivesync.NewApplier(w)
	const actor = "sync-dup"
	run := func(t *testing.T, prev map[int]uuid.UUID, items ...*mbactivesync.ApplyItem) *memResults {
		t.Helper()
		s := &memResults{}
		_, err := ap.Run(f.ctx, items, mbactivesync.ApplyOptions{Phase: mbactivesync.PhaseC, Actor: actor, PrevClones: prev}, s)
		require.NoError(t, err)
		return s
	}
	fullSubs := []string{mbactivesync.SubSetFG, mbactivesync.SubSetLDR, mbactivesync.SubSetShade}

	t.Run("different shade: clone named like source, shade/LDR/FG in tx 2, rerun idempotent", func(t *testing.T) {
		poy := f.poy("P-DUP-B", "DB1")
		h := f.head(true, "DA1")
		src := f.spin(h, "DA1", 6, true, nil)
		it := dupItem(1, "S-DIFF", src, "DB1", poy, 8.5, fullSubs...)
		s := run(t, nil, it)
		require.Len(t, s.out, 2, "PARTIAL flushed before tx 2, then the final result")
		require.Equal(t, mbactivesync.ResultPartial, s.out[0].Result.Result)
		require.Equal(t, mbactivesync.ResultApplied, s.out[1].Result.Result, s.out[1].Result.Message)
		kids := f.children(src)
		require.Len(t, kids, 1)
		require.Equal(t, kids[0].String(), s.out[0].Result.CloneSpinID)
		require.Equal(t, kids[0].String(), s.out[1].Result.CloneSpinID)
		r := f.readSpin(kids[0])
		require.Equal(t, "DB1", r.shade)
		require.InDelta(t, 8.5, r.ldr, 1e-9)
		require.Equal(t, "ACTUAL", r.ldrType)
		require.Equal(t, poy, r.fg.Int64)
		require.Equal(t, "BACKFILL", r.fgSrc.String)
		require.Equal(t, f.spinName(src), f.spinName(kids[0]), "Q10: clone keeps the source name")

		s = run(t, nil, it)
		require.Len(t, s.out, 1)
		require.Equal(t, mbactivesync.ResultSkipped, s.out[0].Result.Result)
		require.Equal(t, mbactivesync.ReasonAlreadyInTarget, s.out[0].Result.Reason)
		require.Len(t, f.children(src), 1)
	})

	t.Run("same shade: FG linked by the duplicate itself", func(t *testing.T) {
		poy := f.poy("P-DUP-S", "SS1")
		h := f.head(true, "SS1")
		src := f.spin(h, "SS1", 6, true, nil)
		s := run(t, nil, dupItem(2, "S-SAME", src, "SS1", poy, 7, mbactivesync.SubSetFG, mbactivesync.SubSetLDR))
		require.Equal(t, mbactivesync.ResultApplied, s.out[len(s.out)-1].Result.Result, s.out[len(s.out)-1].Result.Message)
		kids := f.children(src)
		require.Len(t, kids, 1)
		r := f.readSpin(kids[0])
		require.Equal(t, poy, r.fg.Int64)
		require.Equal(t, "DUPLICATE", r.fgSrc.String)
		require.InDelta(t, 7.0, r.ldr, 1e-9)
	})

	t.Run("tx 2 fails -> rerun -> exactly one clone (without and with prev-results)", func(t *testing.T) {
		for _, withPrev := range []bool{false, true} {
			poy := f.poy("P-DUP-F-"+uuid.NewString()[:6], "FB1")
			f.setPOYActive(poy, false)
			h := f.head(true, "FA1")
			src := f.spin(h, "FA1", 6, true, nil)
			it := dupItem(3, "S-FAIL", src, "FB1", poy, 9, fullSubs...)
			s := run(t, nil, it)
			last := s.out[len(s.out)-1].Result
			require.Equal(t, mbactivesync.ResultFailed, last.Result)
			require.Equal(t, mbactivesync.ReasonPartialClone, last.Reason)
			require.NotEmpty(t, last.CloneSpinID)
			require.Empty(t, last.ManualReason, "head active: no Q1 guard")
			require.Len(t, f.children(src), 1)
			require.True(t, f.readSpin(f.children(src)[0]).active)

			f.setPOYActive(poy, true)
			var prev map[int]uuid.UUID
			if withPrev {
				prev = map[int]uuid.UUID{3: uuid.MustParse(last.CloneSpinID)}
			}
			s = run(t, prev, it)
			require.Len(t, s.out, 1, "no new PARTIAL: the clone is reused")
			require.Equal(t, mbactivesync.ResultApplied, s.out[0].Result.Result, s.out[0].Result.Message)
			require.Equal(t, last.CloneSpinID, s.out[0].Result.CloneSpinID)
			kids := f.children(src)
			require.Len(t, kids, 1, "withPrev=%v", withPrev)
			require.Equal(t, poy, f.readSpin(kids[0]).fg.Int64)
		}
	})

	t.Run("two DUP rows sharing (source, den, fil) claim distinct clones deterministically", func(t *testing.T) {
		poyA := f.poy("P-DUP-2A", "TA1")
		poyB := f.poy("P-DUP-2B", "TB1")
		f.setPOYActive(poyA, false)
		f.setPOYActive(poyB, false)
		h := f.head(true, "T01")
		src := f.spin(h, "T01", 6, true, nil)
		itA := dupItem(10, "S-TWO", src, "TA1", poyA, 5, fullSubs...)
		itB := dupItem(11, "S-TWO", src, "TB1", poyB, 6, fullSubs...)
		s := run(t, nil, itA, itB)
		require.Len(t, s.byResult(mbactivesync.ResultFailed), 2)
		kids := f.children(src)
		require.Len(t, kids, 2)
		cloneA := s.byResult(mbactivesync.ResultFailed)[0].Result.CloneSpinID
		cloneB := s.byResult(mbactivesync.ResultFailed)[1].Result.CloneSpinID
		require.NotEqual(t, cloneA, cloneB)

		f.setPOYActive(poyA, true)
		f.setPOYActive(poyB, true)
		s = run(t, nil, itA, itB)
		require.Len(t, s.out, 2)
		for _, o := range s.out {
			require.Equal(t, mbactivesync.ResultApplied, o.Result.Result, o.Result.Message)
		}
		require.Len(t, f.children(src), 2, "no third clone")
		require.Equal(t, cloneA, s.out[0].Result.CloneSpinID, "oldest unclaimed clone goes to the lowest ln")
		require.Equal(t, cloneB, s.out[1].Result.CloneSpinID)
		require.Equal(t, poyA, f.readSpin(uuid.MustParse(cloneA)).fg.Int64)
		require.Equal(t, poyB, f.readSpin(uuid.MustParse(cloneB)).fg.Int64)

		s = run(t, nil, itB, itA) // reruns are fully idempotent
		for _, o := range s.out {
			require.Equal(t, mbactivesync.ReasonAlreadyInTarget, o.Result.Reason)
		}
		require.Len(t, f.children(src), 2)
	})

	t.Run("partial clone on inactive head is deactivated (Q1) and listed for manual follow-up", func(t *testing.T) {
		poy := f.poy("P-DUP-Q1", "QB1")
		f.setPOYActive(poy, false)
		h := f.head(false, "QA1")
		src := f.spin(h, "QA1", 6, true, nil)
		it := dupItem(20, "S-Q1", src, "QB1", poy, 4, append(append([]string{}, fullSubs...), mbactivesync.SubActHead)...)
		s := run(t, nil, it)
		last := s.out[len(s.out)-1].Result
		require.Equal(t, mbactivesync.ReasonPartialClone, last.Reason)
		require.Equal(t, mbactivesync.ReasonPartialCloneInactiveHead, last.ManualReason)
		clone := uuid.MustParse(last.CloneSpinID)
		require.False(t, f.readSpin(clone).active, "clone deactivated on the dead head")
		ha, _, _ := f.readHead(h)
		require.False(t, ha)

		f.setPOYActive(poy, true)
		s = run(t, map[int]uuid.UUID{20: clone}, it)
		require.Equal(t, mbactivesync.ResultApplied, s.out[0].Result.Result, s.out[0].Result.Message)
		require.True(t, f.readSpin(clone).active, "rerun reactivates the clone")
		ha, _, _ = f.readHead(h)
		require.True(t, ha, "Q1: head activated once it has a valid spin")
		require.Len(t, f.children(src), 1)
	})

	t.Run("source of a GENERATE DUP row not created yet is skipped", func(t *testing.T) {
		it := dupItem(30, "S-GEN", uuid.New(), "X", 1, 1, fullSubs...)
		it.Action, it.Role, it.Source = string(mbactivesync.ActionGenerateSpin), string(mbactivesync.RoleDup), "ln:29"
		s := run(t, nil, it)
		require.Equal(t, mbactivesync.ReasonSourceNotReady, s.out[0].Result.Reason)
	})
}
