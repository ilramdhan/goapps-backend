// Integration test for the ERP batch / demand / coverage repositories and the
// G11 tx runner (plan-04 P3-T2). LOCAL throwaway PostgreSQL only, via
// internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

type erpBatchFixture struct {
	raw      *sql.DB
	db       *postgres.DB
	batches  *postgres.ErpIntBatchRepository
	demand   *postgres.ErpDemandRepository
	coverage *postgres.ErpCoverageRepository
	runner   *postgres.ErpBatchTxRunner
}

func newErpBatchFixture(ctx context.Context, t *testing.T) erpBatchFixture {
	t.Helper()
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "erp_batch_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))
	db := postgres.NewDBFromSQL(raw)
	return erpBatchFixture{raw: raw, db: db,
		batches:  postgres.NewErpIntBatchRepository(db),
		demand:   postgres.NewErpDemandRepository(db),
		coverage: postgres.NewErpCoverageRepository(db),
		runner:   postgres.NewErpBatchTxRunner(db)}
}

var erpT0 = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

func erpCreate(ctx context.Context, t *testing.T, f erpBatchFixture, period string, mode erpintegration.BatchMode) (*erpintegration.Batch, error) {
	t.Helper()
	b, err := erpintegration.NewBatch(period, mode, "integ", erpT0)
	require.NoError(t, err)
	return f.batches.Create(ctx, b)
}

func erpDemandLines(batchID int64, n int) []erpintegration.DemandLine {
	items := []string{"YRN001", "CMB002", "YRN003"}
	txns := []string{erpintegration.TxnInvAdj, erpintegration.TxnMbInvAdj, erpintegration.TxnInvAdj}
	out := make([]erpintegration.DemandLine, 0, n)
	for i := 0; i < n; i++ {
		l := erpintegration.DemandLine{
			BatchID: batchID, Period: "202608", TxnCode: txns[i], Kind: erpintegration.ItemKindForCode(items[i]),
			ItemCode: items[i], GradeCode: "A", ShadeCode: "S1", ItemCount: 2, RateVariants: 1,
			QtyKg:       decimal.RequireFromString("12.34567"),
			MinRate:     decimal.NewNullDecimal(decimal.RequireFromString("1.00001")),
			LoadedAt:    erpT0,
			ItemName:    "",
			GoappsBatch: "",
		}
		if i == 0 {
			l.ItemName, l.GoappsBatch, l.GoappsSource = "Yarn one", "GB1", "SRC"
			l.AdjVal = decimal.NewNullDecimal(decimal.RequireFromString("-3.5"))
		}
		out = append(out, l)
	}
	return out
}

func erpCoverageLines(batchID int64) []erpintegration.CoverageLine {
	p, c, v := int64(10), int64(20), int32(3)
	return []erpintegration.CoverageLine{
		{BatchID: batchID, Kind: erpintegration.ItemKindYarn, ItemCode: "YRN001", ShadeCode: "S1",
			GradeCodes: []string{"A", "B"}, ProductSysID: &p, CostID: &c, CostVersion: &v,
			Status: erpintegration.CoverageOK, QtyKg: decimal.RequireFromString("1.5"), Candidates: []byte(`[{"id":1}]`)},
		{BatchID: batchID, Kind: erpintegration.ItemKindMB, ItemCode: "CMB002", ShadeCode: "",
			Status: erpintegration.CoverageNoMapping, Reason: "no link", QtyKg: decimal.Zero},
	}
}

func TestErpBatchRepositoryIntegration(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)

	t.Run("create_and_inflight_uniqueness", func(t *testing.T) {
		a, err := erpCreate(ctx, t, f, "202608", erpintegration.ModeLive)
		require.NoError(t, err)
		assert.Positive(t, a.ID())
		assert.Equal(t, 1, a.Seq())
		assert.Equal(t, erpintegration.StatusDraft, a.Status())
		assert.Equal(t, "integ", a.CreatedBy())

		_, err = erpCreate(ctx, t, f, "202608", erpintegration.ModeLive)
		assert.ErrorIs(t, err, erpintegration.ErrActiveBatchExists, "second LIVE in-flight batch")

		s, err := erpCreate(ctx, t, f, "202608", erpintegration.ModeShadow)
		require.NoError(t, err, "SHADOW is outside the LIVE unique indexes")
		assert.Equal(t, 2, s.Seq())

		got, err := f.batches.FindInFlight(ctx, "202608")
		require.NoError(t, err)
		assert.Equal(t, a.ID(), got.ID())
		_, err = f.batches.FindActive(ctx, "202608")
		assert.ErrorIs(t, err, erpintegration.ErrBatchNotFound)
		list, err := f.batches.ListByPeriod(ctx, "202608")
		require.NoError(t, err)
		require.Len(t, list, 2)
		assert.Equal(t, 2, list[0].Seq(), "newest seq first")
		_, err = f.batches.GetByID(ctx, 987654)
		assert.ErrorIs(t, err, erpintegration.ErrBatchNotFound)
	})

	t.Run("active_uniqueness", func(t *testing.T) {
		const p = "202607"
		_ = insertBatch(ctx, t, f.raw, p, 1, "VALUATED")
		pushedID := insertBatch(ctx, t, f.raw, p, 2, "PUSHED")
		b, err := f.batches.GetByID(ctx, pushedID)
		require.NoError(t, err)
		inv, err := b.Transition(erpintegration.StatusValuated, "integ", erpT0)
		require.NoError(t, err)
		err = f.batches.Save(ctx, b, erpintegration.StatusPushed, inv)
		assert.ErrorIs(t, err, erpintegration.ErrActiveBatchExists)
		reread, err := f.batches.GetByID(ctx, pushedID)
		require.NoError(t, err)
		assert.Equal(t, erpintegration.StatusPushed, reread.Status(), "rolled back")
		act, err := f.batches.FindActive(ctx, p)
		require.NoError(t, err)
		assert.Equal(t, erpintegration.StatusValuated, act.Status())
	})

	t.Run("stale_status_update", func(t *testing.T) {
		b, err := erpCreate(ctx, t, f, "202606", erpintegration.ModeLive)
		require.NoError(t, err)
		first, err := f.batches.GetByID(ctx, b.ID())
		require.NoError(t, err)
		second, err := f.batches.GetByID(ctx, b.ID())
		require.NoError(t, err)

		inv, err := first.Transition(erpintegration.StatusDemandLoaded, "u1", erpT0.Add(time.Minute))
		require.NoError(t, err)
		require.NoError(t, f.batches.Save(ctx, first, erpintegration.StatusDraft, inv))

		inv2, err := second.Transition(erpintegration.StatusFailed, "u2", erpT0.Add(2*time.Minute))
		require.NoError(t, err)
		assert.ErrorIs(t, f.batches.Save(ctx, second, erpintegration.StatusDraft, inv2), erpintegration.ErrStaleBatchStatus)

		got, err := f.batches.GetByID(ctx, b.ID())
		require.NoError(t, err)
		assert.Equal(t, erpintegration.StatusDemandLoaded, got.Status())
		assert.Equal(t, "u1", got.UpdatedBy())
		require.NotNil(t, got.DemandLoadedAt())

		st := got.State()
		st.ID = 987654
		ghost, err := erpintegration.ReconstituteBatch(st)
		require.NoError(t, err)
		assert.ErrorIs(t, f.batches.Save(ctx, ghost, erpintegration.StatusDemandLoaded, erpintegration.Invalidation{}),
			erpintegration.ErrBatchNotFound)
	})

	t.Run("demand_and_coverage_rerun_replacement", func(t *testing.T) {
		b, err := erpCreate(ctx, t, f, "202605", erpintegration.ModeLive)
		require.NoError(t, err)
		id := b.ID()

		for run := 0; run < 2; run++ {
			n, err := f.demand.Replace(ctx, id, erpDemandLines(id, 3))
			require.NoError(t, err)
			assert.Equal(t, int64(3), n)
		}
		cnt, err := f.demand.Count(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, int64(3), cnt, "re-run replaces, no duplicates")

		lines, err := f.demand.List(ctx, id)
		require.NoError(t, err)
		require.Len(t, lines, 3)
		first := lines[0]
		assert.Equal(t, erpintegration.TxnInvAdj, first.TxnCode)
		assert.Equal(t, "YRN001", first.ItemCode)
		assert.Equal(t, "Yarn one", first.ItemName)
		assert.Equal(t, "GB1", first.GoappsBatch)
		assert.True(t, first.QtyKg.Equal(decimal.RequireFromString("12.34567")))
		assert.True(t, first.AdjVal.Valid)
		assert.True(t, first.AdjVal.Decimal.Equal(decimal.RequireFromString("-3.5")))
		assert.False(t, first.MaxRate.Valid)
		assert.True(t, first.LoadedAt.Equal(erpT0))
		assert.Equal(t, erpintegration.ItemKindMB, lines[2].Kind)

		n, err := f.demand.Replace(ctx, id, erpDemandLines(id, 1))
		require.NoError(t, err)
		assert.Equal(t, int64(1), n)
		cnt, err = f.demand.Count(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, int64(1), cnt)

		bad := erpDemandLines(id, 1)
		bad[0].TxnCode = "BAD"
		_, err = f.demand.Replace(ctx, id, bad)
		assert.ErrorIs(t, err, erpintegration.ErrInvalidDemandLine)
		cnt, err = f.demand.Count(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, int64(1), cnt, "invalid input leaves rows untouched")

		for run := 0; run < 2; run++ {
			n, err = f.coverage.Replace(ctx, id, erpCoverageLines(id))
			require.NoError(t, err)
			assert.Equal(t, int64(2), n)
		}
		counts, err := f.coverage.Counts(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, int64(2), counts.Total())
		assert.Equal(t, int64(1), counts[erpintegration.CoverageNoMapping])
		assert.False(t, counts.AllOK())

		all, err := f.coverage.List(ctx, id)
		require.NoError(t, err)
		require.Len(t, all, 2)
		assert.Equal(t, "CMB002", all[0].ItemCode)
		assert.Nil(t, all[0].ProductSysID)
		assert.Equal(t, "no link", all[0].Reason)
		assert.Empty(t, all[0].GradeCodes)
		assert.Equal(t, []string{"A", "B"}, all[1].GradeCodes)
		require.NotNil(t, all[1].CostVersion)
		assert.Equal(t, int32(3), *all[1].CostVersion)
		assert.JSONEq(t, `[{"id":1}]`, string(all[1].Candidates))

		require.NotZero(t, all[0].ID)
		one, err := f.coverage.GetByID(ctx, id, all[0].ID)
		require.NoError(t, err)
		assert.Equal(t, all[0].ItemCode, one.ItemCode)
		assert.Equal(t, all[0].Status, one.Status)
		_, err = f.coverage.GetByID(ctx, id, all[0].ID+1000)
		require.ErrorIs(t, err, erpintegration.ErrCoverageLineNotFound)

		gaps, err := f.coverage.List(ctx, id, erpintegration.CoverageNoMapping, erpintegration.CoverageNoCost)
		require.NoError(t, err)
		require.Len(t, gaps, 1)
		assert.Equal(t, erpintegration.CoverageNoMapping, gaps[0].Status)

		// Reload through Save: DRAFT -> DEMAND_LOADED invalidates demand +
		// coverage in the same tx as the status update.
		inv, err := b.Transition(erpintegration.StatusDemandLoaded, "integ", erpT0)
		require.NoError(t, err)
		require.True(t, inv.Demand && inv.Coverage)
		require.NoError(t, f.batches.Save(ctx, b, erpintegration.StatusDraft, inv))
		cnt, err = f.demand.Count(ctx, id)
		require.NoError(t, err)
		assert.Zero(t, cnt)
		counts, err = f.coverage.Counts(ctx, id)
		require.NoError(t, err)
		assert.Zero(t, counts.Total())
	})

	t.Run("run_locked", func(t *testing.T) {
		b, err := erpCreate(ctx, t, f, "202604", erpintegration.ModeLive)
		require.NoError(t, err)
		id := b.ID()

		// Another session holds G11: RunLocked refuses without calling fn.
		tx, err := f.raw.BeginTx(ctx, nil)
		require.NoError(t, err)
		got, err := postgres.TryLockErpBatch(ctx, tx, id)
		require.NoError(t, err)
		require.True(t, got)
		called := false
		err = f.runner.RunLocked(ctx, id, func(context.Context, erpintegration.BatchStore) error {
			called = true
			return nil
		})
		assert.ErrorIs(t, err, erpintegration.ErrConcurrentRun)
		assert.False(t, called)
		require.NoError(t, tx.Rollback())

		// fn error rolls back the replacement.
		boom := errors.New("boom")
		err = f.runner.RunLocked(ctx, id, func(ctx context.Context, s erpintegration.BatchStore) error {
			_, err := s.ReplaceDemand(ctx, erpDemandLines(id, 2))
			require.NoError(t, err)
			return boom
		})
		assert.ErrorIs(t, err, boom)
		cnt, err := f.demand.Count(ctx, id)
		require.NoError(t, err)
		assert.Zero(t, cnt)

		// Replace then Save: the invalidation must not delete the new rows.
		err = f.runner.RunLocked(ctx, id, func(ctx context.Context, s erpintegration.BatchStore) error {
			cur, err := s.GetForUpdate(ctx)
			if err != nil {
				return err
			}
			if _, err := s.ReplaceDemand(ctx, erpDemandLines(id, 2)); err != nil {
				return err
			}
			if _, err := s.ReplaceCoverage(ctx, erpCoverageLines(id)[:1]); err != nil {
				return err
			}
			inv, err := cur.Transition(erpintegration.StatusDemandLoaded, "runner", erpT0)
			if err != nil {
				return err
			}
			if err := s.Save(ctx, cur, erpintegration.StatusDraft, inv); err != nil {
				return err
			}
			rows, err := s.ListDemand(ctx)
			if err != nil {
				return err
			}
			assert.Len(t, rows, 2)
			return nil
		})
		require.NoError(t, err)
		cnt, err = f.demand.Count(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, int64(2), cnt)
		counts, err := f.coverage.Counts(ctx, id)
		require.NoError(t, err)
		assert.True(t, counts.AllOK())
		after, err := f.batches.GetByID(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, erpintegration.StatusDemandLoaded, after.Status())

		// Saving a different batch through the store is refused.
		other, err := erpCreate(ctx, t, f, "202603", erpintegration.ModeLive)
		require.NoError(t, err)
		err = f.runner.RunLocked(ctx, id, func(ctx context.Context, s erpintegration.BatchStore) error {
			return s.Save(ctx, other, erpintegration.StatusDraft, erpintegration.Invalidation{})
		})
		assert.Error(t, err)
	})
}
