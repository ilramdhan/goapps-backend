// Integration test for the ADJ execute store additions and the one-shot
// preview consumption (plan-06 P5-T5). LOCAL throwaway PostgreSQL only, via
// internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func TestErpAdjExecStore_ActiveSupersedeAndLatest(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	prior := insertBatch(ctx, t, f.raw, "202608", 1, string(erpintegration.StatusReconciled))
	cur := insertBatch(ctx, t, f.raw, "202608", 2, string(erpintegration.StatusPushed))

	err := f.runner.RunLocked(ctx, cur, func(ctx context.Context, bs erpintegration.BatchStore) error {
		st, ok := bs.(erpintegration.AdjExecStore)
		require.True(t, ok)
		latest, err := st.IsLatestBatch(ctx, "202608")
		require.NoError(t, err)
		assert.True(t, latest)

		active, err := st.ActiveForUpdate(ctx, "202608")
		require.NoError(t, err)
		assert.Equal(t, prior, active.ID())

		self, err := st.GetForUpdate(ctx)
		require.NoError(t, err)
		require.Error(t, st.SaveOther(ctx, self, erpintegration.StatusPushed), "the locked batch goes through Save")

		_, err = active.Transition(erpintegration.StatusSuperseded, "integ", time.Now())
		require.NoError(t, err)
		require.NoError(t, st.SaveOther(ctx, active, erpintegration.StatusReconciled))
		_, err = st.ActiveForUpdate(ctx, "202608")
		require.ErrorIs(t, err, erpintegration.ErrBatchNotFound)
		return nil
	})
	require.NoError(t, err)

	got, err := f.batches.GetByID(ctx, prior)
	require.NoError(t, err)
	assert.Equal(t, erpintegration.StatusSuperseded, got.Status())

	err = f.runner.RunLocked(ctx, prior, func(ctx context.Context, bs erpintegration.BatchStore) error {
		st, ok := bs.(erpintegration.AdjExecStore)
		require.True(t, ok)
		latest, err := st.IsLatestBatch(ctx, "202608")
		require.NoError(t, err)
		assert.False(t, latest, "a later seq exists")
		return nil
	})
	require.NoError(t, err)
}

func TestErpAdjExecPreviewConsume_OnlyOnce(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	repo := postgres.NewErpValuationPreviewRepository(f.db)
	id := insertBatch(ctx, t, f.raw, "202608", 1, string(erpintegration.StatusPushed))
	p, err := repo.CreateOpen(ctx, newPreview(id), "202608", erpintegration.StatusPushed, previewSnapshotRows())
	require.NoError(t, err)

	at := time.Now().UTC().Truncate(time.Second)
	require.ErrorIs(t, repo.Consume(ctx, p.ID, " ", at), erpintegration.ErrActorRequired)
	require.NoError(t, repo.Consume(ctx, p.ID, "alice", at))
	require.ErrorIs(t, repo.Consume(ctx, p.ID, "alice", at), erpintegration.ErrPreviewConsumed)
	require.ErrorIs(t, repo.Consume(ctx, "not-a-uuid", "alice", at), erpintegration.ErrPreviewConsumed)

	got, err := repo.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, erpintegration.PreviewConsumed, got.Status)
	assert.Equal(t, "alice", got.ConsumedBy)
	require.NotNil(t, got.ConsumedAt)
	assert.True(t, got.ConsumedAt.Equal(at))
}
