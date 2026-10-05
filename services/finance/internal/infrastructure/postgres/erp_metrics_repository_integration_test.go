// Integration test for the ERP observability aggregates (plan-06 P5-T8).
// LOCAL throwaway PostgreSQL only, via internal/testutil/pgcontainer.
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/metrics"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func TestErpMetrics_EmptyDatabase(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	st, err := postgres.NewErpMetricsRepository(f.db).ErpStats(ctx)
	require.NoError(t, err)
	assert.Empty(t, st.Batches)
	assert.Empty(t, st.Calls)
	assert.Zero(t, st.OldestStartedAge)
	assert.Empty(t, st.StaleOpenPreviews)
	assert.Zero(t, st.OldestOpenPreviewAge)
}

func TestErpMetrics_AggregatesAndSampler(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	repo := postgres.NewErpMetricsRepository(f.db)
	calls := postgres.NewErpOracleCallRepository(f.db)

	b1 := insertBatch(ctx, t, f.raw, "202608", 1, "VALIDATED")
	_ = insertBatch(ctx, t, f.raw, "202607", 1, "VALIDATED")
	b3 := insertBatch(ctx, t, f.raw, "202609", 1, "PUSHED")

	// One SUCCESS W1, one STARTED (backdated 10 min) and one UNKNOWN W2.
	ok := uuid.NewString()
	require.NoError(t, calls.Start(ctx, erpintegration.OracleCallStart{CallID: ok, BatchID: b3, StatementKey: "W1_INSERT_BATCH", Actor: "it"}))
	require.NoError(t, calls.Finish(ctx, ok, erpintegration.OracleCallFinish{Status: erpintegration.OracleCallSuccess}))
	started := uuid.NewString()
	require.NoError(t, calls.Start(ctx, erpintegration.OracleCallStart{CallID: started, BatchID: b1, StatementKey: "W1_INSERT_BATCH", Actor: "it"}))
	_, err := f.raw.ExecContext(ctx, `UPDATE cst_erp_oracle_call_log SET ceocl_started_at = NOW() - INTERVAL '10 minutes' WHERE ceocl_call_id = $1::uuid`, started)
	require.NoError(t, err)
	unk := uuid.NewString()
	require.NoError(t, calls.Start(ctx, erpintegration.OracleCallStart{CallID: unk, BatchID: b3, StatementKey: "W2_VALUATE_ADJ", Actor: "it"}))
	require.NoError(t, calls.Finish(ctx, unk, erpintegration.OracleCallFinish{Status: erpintegration.OracleCallUnknown}))

	// One OPEN preview past its TTL (created 2h ago, expired 1h ago), one fresh OPEN.
	_, err = f.raw.ExecContext(ctx, `
		INSERT INTO cst_erp_valuation_preview (cevp_batch_id, cevp_operation, cevp_status, cevp_created_by, cevp_created_at, cevp_expires_at)
		VALUES ($1, 'VALUATE', 'OPEN', 'it', NOW() - INTERVAL '2 hours', NOW() - INTERVAL '1 hour'),
		       ($1, 'APPROVE', 'OPEN', 'it', NOW(), NOW() + INTERVAL '30 minutes')`, b3)
	require.NoError(t, err)

	st, err := repo.ErpStats(ctx)
	require.NoError(t, err)

	batches := map[string]int64{}
	for _, b := range st.Batches {
		batches[b.Status+"/"+b.Mode] = b.Count
	}
	assert.Equal(t, map[string]int64{"VALIDATED/LIVE": 2, "PUSHED/LIVE": 1}, batches)

	cl := map[string]int64{}
	for _, c := range st.Calls {
		cl[c.Key+"/"+c.Status] = c.Count
	}
	assert.Equal(t, map[string]int64{
		"W1_INSERT_BATCH/SUCCESS": 1, "W1_INSERT_BATCH/STARTED": 1, "W2_VALUATE_ADJ/UNKNOWN": 1,
	}, cl)

	assert.GreaterOrEqual(t, st.OldestStartedAge, 9*time.Minute)
	assert.Less(t, st.OldestStartedAge, time.Hour)
	assert.Equal(t, map[string]int64{"VALUATE": 1}, st.StaleOpenPreviews)
	assert.GreaterOrEqual(t, st.OldestOpenPreviewAge, 119*time.Minute)

	// The sampler accepts the real source end to end.
	require.NoError(t, metrics.NewErpStatsSampler(repo, time.Minute).SampleOnce(ctx))
}
