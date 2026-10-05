// Integration test for the Oracle call log repository and the push store
// (plan-06 P5-T3). LOCAL throwaway PostgreSQL only, via
// internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func TestErpOracleCall_StartFinishLifecycle(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	repo := postgres.NewErpOracleCallRepository(f.db)
	id := insertBatch(ctx, t, f.raw, "202609", 1, "VALIDATED")

	has, err := repo.HasAttempt(ctx, id, erpintegration.CallKeyW1InsertBatch)
	require.NoError(t, err)
	assert.False(t, has)

	callID := uuid.NewString()
	require.NoError(t, repo.Start(ctx, erpintegration.OracleCallStart{
		CallID: callID, BatchID: id, StatementKey: erpintegration.CallKeyW1InsertBatch,
		Params: json.RawMessage(`{"period":"202609","row_count":3}`), Actor: "approver",
	}))

	has, err = repo.HasAttempt(ctx, id, erpintegration.CallKeyW1InsertBatch)
	require.NoError(t, err)
	assert.True(t, has, "a STARTED row counts as an attempt (no retry)")

	var status, actor string
	var attempt int
	require.NoError(t, f.raw.QueryRowContext(ctx,
		`SELECT ceocl_status, ceocl_actor, ceocl_attempt FROM cst_erp_oracle_call_log WHERE ceocl_call_id=$1::uuid`, callID).
		Scan(&status, &actor, &attempt))
	assert.Equal(t, "STARTED", status)
	assert.Equal(t, "approver", actor)
	assert.Equal(t, 1, attempt)

	rows := int64(3)
	require.NoError(t, repo.Finish(ctx, callID, erpintegration.OracleCallFinish{Status: erpintegration.OracleCallSuccess, Rows: &rows}))
	var gotRows int64
	require.NoError(t, f.raw.QueryRowContext(ctx,
		`SELECT ceocl_status, ceocl_rows FROM cst_erp_oracle_call_log WHERE ceocl_call_id=$1::uuid`, callID).
		Scan(&status, &gotRows))
	assert.Equal(t, "SUCCESS", status)
	assert.Equal(t, int64(3), gotRows)

	err = repo.Finish(ctx, callID, erpintegration.OracleCallFinish{Status: erpintegration.OracleCallFailed})
	require.ErrorIs(t, err, postgres.ErrOracleCallNotFound, "a terminal row is never rewritten")

	err = repo.Finish(ctx, callID, erpintegration.OracleCallFinish{Status: erpintegration.OracleCallStarted})
	require.Error(t, err, "STARTED is not a terminal status")
}

func TestErpOracleCall_FailedWithOraCode(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	repo := postgres.NewErpOracleCallRepository(f.db)
	id := insertBatch(ctx, t, f.raw, "202609", 1, "VALIDATED")
	callID := uuid.NewString()
	require.NoError(t, repo.Start(ctx, erpintegration.OracleCallStart{
		CallID: callID, BatchID: id, StatementKey: erpintegration.CallKeyW1InsertBatch, Actor: "approver",
	}))
	require.NoError(t, repo.Finish(ctx, callID, erpintegration.OracleCallFinish{
		Status: erpintegration.OracleCallFailed, OraCode: "ORA-20901", Error: "period frozen",
	}))
	var status, ora string
	require.NoError(t, f.raw.QueryRowContext(ctx,
		`SELECT ceocl_status, ceocl_ora_code FROM cst_erp_oracle_call_log WHERE ceocl_call_id=$1::uuid`, callID).
		Scan(&status, &ora))
	assert.Equal(t, "FAILED", status)
	assert.Equal(t, "ORA-20901", ora)
}

func TestErpOracleCall_RejectsUnknownStatementKey(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	repo := postgres.NewErpOracleCallRepository(f.db)
	id := insertBatch(ctx, t, f.raw, "202609", 1, "VALIDATED")
	err := repo.Start(ctx, erpintegration.OracleCallStart{
		CallID: uuid.NewString(), BatchID: id, StatementKey: "RAW_UPDATE", Actor: "approver",
	})
	require.Error(t, err, "CHECK constraint enforces the statement-key allowlist")
	has, err := repo.HasAttempt(ctx, id, "RAW_UPDATE")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestErpBatchPushStore_ListStdRowsAndPeriodLock(t *testing.T) {
	ctx := context.Background()
	f := newErpBatchFixture(ctx, t)
	id := insertBatch(ctx, t, f.raw, "202609", 1, "VALIDATED")
	err := f.runner.RunLocked(ctx, id, func(ctx context.Context, bs erpintegration.BatchStore) error {
		st, ok := bs.(erpintegration.PushStore)
		require.True(t, ok, "erpBatchStore implements PushStore")
		rows, err := st.ListStdRows(ctx)
		require.NoError(t, err)
		assert.Empty(t, rows)
		locked, err := st.IsPeriodLocked(ctx, "202609", "ACTUAL")
		require.NoError(t, err)
		assert.False(t, locked)
		return nil
	})
	require.NoError(t, err)
}
