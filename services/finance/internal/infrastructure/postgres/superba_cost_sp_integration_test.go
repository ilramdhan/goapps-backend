// Integration test for SuperbaCostSpRepository (shade resolution + upsert).
// LOCAL PostgreSQL only, via internal/testutil/pgcontainer.
//
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sp "github.com/mutugading/goapps-backend/services/finance/internal/domain/superbacostsp"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

func TestSuperbaCostSpRepositoryIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "superba_cost_sp_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	repo := postgres.NewSuperbaCostSpRepository(postgres.NewDBFromSQL(raw))
	colour := func(s string) *string { return &s }

	// Shades are ZZ-prefixed: the migrated DB already holds the 644 seed rows.
	// Duplicate shade (max legacy id wins), case/space normalization, inactive, deleted.
	for _, s := range []sp.Sourced{
		{LegacySysID: 100, ShadeCode: "ZZTESTDUP", ColourName: colour("old"), OldValue: 1},
		{LegacySysID: 300, ShadeCode: " zztestdup ", ColourName: colour("newest"), OldValue: 3},
		{LegacySysID: 200, ShadeCode: "ZZTESTDUP", ColourName: colour("mid"), OldValue: 2},
		{LegacySysID: 400, ShadeCode: "ZZTEST9", OldValue: 9},
	} {
		out, upErr := repo.UpsertByLegacySysID(ctx, s)
		require.NoError(t, upErr)
		assert.Equal(t, sp.OutcomeInserted, out)
	}

	t.Run("duplicate shade resolves to max legacy_sys_id, normalized both sides", func(t *testing.T) {
		got, rErr := repo.ResolveByShades(ctx, []string{"zztestdup ", "ZZTEST9", "ZZNOPE"})
		require.NoError(t, rErr)
		require.Len(t, got, 2)
		assert.Equal(t, int64(300), got["ZZTESTDUP"].LegacySysID)
		assert.InDelta(t, 3, got["ZZTESTDUP"].OldValue, 1e-9)
		assert.Equal(t, "newest", got["ZZTESTDUP"].ColourName)
		assert.NotContains(t, got, "ZZNOPE")
	})

	t.Run("upsert is idempotent and overwrites MANUAL rows", func(t *testing.T) {
		out, uErr := repo.UpsertByLegacySysID(ctx, sp.Sourced{LegacySysID: 400, ShadeCode: "ZZTEST9", OldValue: 9})
		require.NoError(t, uErr)
		assert.Equal(t, sp.OutcomeUnchanged, out)

		e, gErr := repo.GetByLegacySysID(ctx, 400)
		require.NoError(t, gErr)
		require.NoError(t, e.Update(sp.UpdateParams{OldValue: ptrF(5), UpdatedBy: "u"}))
		require.NoError(t, repo.Update(ctx, e))
		assert.Equal(t, sp.SourceManual, e.Source())

		out, uErr = repo.UpsertByLegacySysID(ctx, sp.Sourced{LegacySysID: 400, ShadeCode: "ZZTEST9", OldValue: 9})
		require.NoError(t, uErr)
		assert.Equal(t, sp.OutcomeUpdated, out)
		e, gErr = repo.GetByLegacySysID(ctx, 400)
		require.NoError(t, gErr)
		assert.Equal(t, sp.SourceOracle, e.Source())
		assert.InDelta(t, 9, e.OldValue(), 1e-9)
	})

	t.Run("inactive and soft-deleted rows are ignored; delete then upsert revives", func(t *testing.T) {
		top, gErr := repo.GetByLegacySysID(ctx, 300)
		require.NoError(t, gErr)
		require.NoError(t, repo.SoftDelete(ctx, top.ID(), "u"))
		got, rErr := repo.ResolveByShades(ctx, []string{"ZZTESTDUP"})
		require.NoError(t, rErr)
		assert.Equal(t, int64(200), got["ZZTESTDUP"].LegacySysID, "falls back to next largest")

		_, gErr = repo.GetByLegacySysID(ctx, 300)
		assert.ErrorIs(t, gErr, sp.ErrNotFound)

		out, uErr := repo.UpsertByLegacySysID(ctx, sp.Sourced{LegacySysID: 300, ShadeCode: "ZZTESTDUP", OldValue: 3})
		require.NoError(t, uErr)
		assert.Equal(t, sp.OutcomeUpdated, out)
		got, rErr = repo.ResolveByShades(ctx, []string{"ZZTESTDUP"})
		require.NoError(t, rErr)
		assert.Equal(t, int64(300), got["ZZTESTDUP"].LegacySysID)

		mid, gErr := repo.GetByLegacySysID(ctx, 200)
		require.NoError(t, gErr)
		off := false
		require.NoError(t, mid.Update(sp.UpdateParams{IsActive: &off, UpdatedBy: "u"}))
		require.NoError(t, repo.Update(ctx, mid))
		got, rErr = repo.ResolveByShades(ctx, []string{"ZZTESTDUP"})
		require.NoError(t, rErr)
		assert.Equal(t, int64(300), got["ZZTESTDUP"].LegacySysID)
	})

	t.Run("list flags the effective row", func(t *testing.T) {
		items, total, _, lErr := repo.List(ctx, sp.ListFilter{Search: "ZZTESTDUP", PageSize: 10})
		require.NoError(t, lErr)
		assert.Equal(t, int64(3), total)
		eff := 0
		for _, it := range items {
			if it.Effective() {
				eff++
				assert.Equal(t, int64(300), it.LegacySysID())
			}
		}
		assert.Equal(t, 1, eff)
	})
}

func ptrF(v float64) *float64 { return &v }
