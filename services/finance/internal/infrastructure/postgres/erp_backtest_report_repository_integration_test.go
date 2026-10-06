// Integration test for ErpBacktestReportRepository (migration 000559;
// plan-06 P5-T10b). LOCAL PostgreSQL only, via internal/testutil/pgcontainer.
//
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

func TestErpBacktestReportRepositoryIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "erp_backtest_report_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	var batchID int64
	require.NoError(t, raw.QueryRowContext(ctx, `INSERT INTO cst_erp_int_batch (ceib_period, ceib_seq, ceib_mode, created_by)
		VALUES ('202608', 1, 'SHADOW', 'it') RETURNING ceib_batch_id`).Scan(&batchID))
	repo := postgres.NewErpBacktestReportRepository(postgres.NewDBFromSQL(raw))

	d := func(s string) decimal.NullDecimal {
		return decimal.NullDecimal{Decimal: decimal.RequireFromString(s), Valid: true}
	}
	rep := erpintegration.BacktestReport{
		Counts: map[erpintegration.BacktestClass]int{erpintegration.BacktestDiff: 1, erpintegration.BacktestOnlyLegacy: 1},
		Failed: true,
		Lines: []erpintegration.BacktestLine{
			{Key: erpintegration.ErpKey{ItemCode: "A", GradeCode: "AX", ShadeCode: "NL"}, Class: erpintegration.BacktestDiff, Basis: "SP1",
				GoApps: d("1.5"), Legacy: d("1.25"), Delta: d("0.25"), DeltaPct: d("20"), Fail: true},
			{Key: erpintegration.ErpKey{ItemCode: "B"}, Class: erpintegration.BacktestOnlyLegacy, Legacy: d("2")},
		},
	}

	t.Run("replace_get_roundtrip", func(t *testing.T) {
		require.NoError(t, repo.Replace(ctx, batchID, "202608", rep))
		got, err := repo.Get(ctx, batchID)
		require.NoError(t, err)
		assert.True(t, got.Failed)
		assert.Equal(t, rep.Counts, got.Counts)
		require.Len(t, got.Lines, 2)
		assert.Equal(t, rep.Lines[0].Key, got.Lines[0].Key)
		assert.True(t, got.Lines[0].Delta.Decimal.Equal(decimal.RequireFromString("0.25")))
		assert.False(t, got.Lines[1].GoApps.Valid)
		assert.Empty(t, got.Lines[1].Basis)
	})

	t.Run("replace_overwrites", func(t *testing.T) {
		require.NoError(t, repo.Replace(ctx, batchID, "202608", erpintegration.BacktestReport{
			Lines: rep.Lines[1:], Counts: map[erpintegration.BacktestClass]int{erpintegration.BacktestOnlyLegacy: 1}}))
		got, err := repo.Get(ctx, batchID)
		require.NoError(t, err)
		assert.Len(t, got.Lines, 1)
		assert.False(t, got.Failed)
	})
}
