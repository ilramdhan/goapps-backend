// Integration test for the recon store writes and the Oracle call-log
// resolver (plan-06 P5-T6). LOCAL throwaway PostgreSQL only, via
// internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func TestErpReconStore_ApplyRecon(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	id := insertBatch(ctx, t, f.raw, "202608", 1, string(erpintegration.StatusValuated))
	rows := erpStdRows(6, "")
	_, err := postgres.NewErpStdCostRepository(f.db).Replace(ctx, id, "202608", rows)
	require.NoError(t, err)

	two := int64(2)
	recon := []erpintegration.ReconRow{
		{Key: rows[0].Key, Status: erpintegration.ReconMatch, ErpRate: stdDec("1.2345678"), RateVariants: &two,
			QtyKg: stdDec("10.5"), Value: stdDec("12.9629619"), Flex13: "7"},
		{Key: rows[1].Key, Status: erpintegration.ReconNotInAdj},
	}
	apply := func(rs []erpintegration.ReconRow) int64 {
		var n int64
		err := f.runner.RunLocked(ctx, id, func(ctx context.Context, bs erpintegration.BatchStore) error {
			st, ok := bs.(erpintegration.ReconStore)
			require.True(t, ok)
			var err error
			n, err = st.ApplyRecon(ctx, rs)
			return err
		})
		require.NoError(t, err)
		return n
	}
	assert.Equal(t, int64(2), apply(recon))

	var (
		status, flex sql.NullString
		rate, val    sql.NullString
		variants     sql.NullInt64
	)
	require.NoError(t, f.raw.QueryRowContext(ctx, `SELECT cesc_recon_status, cesc_erp_flex13, cesc_erp_rate::text,
		cesc_erp_value::text, cesc_erp_rate_variants FROM cst_erp_std_cost
		WHERE cesc_batch_id=$1 AND cesc_item_code=$2 AND cesc_grade_code=$3`, id, rows[0].Key.ItemCode, rows[0].Key.GradeCode).
		Scan(&status, &flex, &rate, &val, &variants))
	assert.Equal(t, "MATCH", status.String)
	assert.Equal(t, "7", flex.String)
	assert.Equal(t, "1.2345678", rate.String)
	assert.Equal(t, "12.9629619", val.String)
	assert.Equal(t, int64(2), variants.Int64)

	var set int
	require.NoError(t, f.raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cst_erp_std_cost WHERE cesc_batch_id=$1 AND cesc_recon_status IS NOT NULL`, id).Scan(&set))
	assert.Equal(t, 2, set)

	// A re-run replaces the previous recon (stale statuses cleared).
	assert.Equal(t, int64(1), apply(recon[1:]))
	require.NoError(t, f.raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cst_erp_std_cost WHERE cesc_batch_id=$1 AND cesc_recon_status IS NOT NULL`, id).Scan(&set))
	assert.Equal(t, 1, set)

	err = f.runner.RunLocked(ctx, id, func(ctx context.Context, bs erpintegration.BatchStore) error {
		_, err := bs.(erpintegration.ReconStore).ApplyRecon(ctx, []erpintegration.ReconRow{{Key: rows[0].Key, Status: "BOGUS"}})
		return err
	})
	require.Error(t, err)
}

func TestErpOracleCall_ListUnresolvedAndResolve(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	repo := postgres.NewErpOracleCallRepository(f.db)
	id := insertBatch(ctx, t, f.raw, "202608", 1, string(erpintegration.StatusPushed))

	start := func(key string) string {
		callID := uuid.NewString()
		require.NoError(t, repo.Start(ctx, erpintegration.OracleCallStart{CallID: callID, BatchID: id, StatementKey: key, Actor: "approver"}))
		return callID
	}
	unknown := start(erpintegration.CallKeyW2ValuateAdj)
	require.NoError(t, repo.Finish(ctx, unknown, erpintegration.OracleCallFinish{Status: erpintegration.OracleCallUnknown, Error: "timeout"}))
	started := start(erpintegration.CallKeyW1InsertBatch)
	done := start(erpintegration.CallKeyW2LockBatch)
	require.NoError(t, repo.Finish(ctx, done, erpintegration.OracleCallFinish{Status: erpintegration.OracleCallSuccess}))

	recs, err := repo.ListUnresolved(ctx, id)
	require.NoError(t, err)
	require.Len(t, recs, 2)
	assert.Equal(t, unknown, recs[0].CallID)
	assert.Equal(t, erpintegration.OracleCallUnknown, recs[0].Status)
	assert.Equal(t, started, recs[1].CallID)
	assert.Equal(t, erpintegration.OracleCallStarted, recs[1].Status)

	note := json.RawMessage(`{"by":"recon","evidence":"GSB_STATUS=VALUATED"}`)
	require.NoError(t, repo.Resolve(ctx, unknown, erpintegration.OracleCallSuccess, note))
	require.NoError(t, repo.Resolve(ctx, started, erpintegration.OracleCallFailed, nil))
	require.ErrorIs(t, repo.Resolve(ctx, unknown, erpintegration.OracleCallFailed, nil), erpintegration.ErrCallAlreadyResolved)
	require.ErrorIs(t, repo.Resolve(ctx, done, erpintegration.OracleCallFailed, nil), erpintegration.ErrCallAlreadyResolved)
	require.Error(t, repo.Resolve(ctx, started, erpintegration.OracleCallUnknown, nil), "target must be terminal")

	var status, by, errText string
	require.NoError(t, f.raw.QueryRowContext(ctx, `SELECT ceocl_status, ceocl_summary->'resolution'->>'by', ceocl_error
		FROM cst_erp_oracle_call_log WHERE ceocl_call_id=$1::uuid`, unknown).Scan(&status, &by, &errText))
	assert.Equal(t, "SUCCESS", status)
	assert.Equal(t, "recon", by)
	assert.Equal(t, "timeout", errText, "the original error is kept")

	var finished bool
	require.NoError(t, f.raw.QueryRowContext(ctx, `SELECT ceocl_finished_at IS NOT NULL
		FROM cst_erp_oracle_call_log WHERE ceocl_call_id=$1::uuid`, started).Scan(&finished))
	assert.True(t, finished)

	recs, err = repo.ListUnresolved(ctx, id)
	require.NoError(t, err)
	assert.Empty(t, recs)
}
