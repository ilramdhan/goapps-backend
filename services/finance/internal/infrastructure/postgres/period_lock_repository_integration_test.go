// Integration test for PeriodLockRepository (plan-02 P1-T2). LOCAL
// PostgreSQL only, via internal/testutil/pgcontainer (a throwaway
// postgres:16-alpine container, or a LOCAL TEST_DATABASE_URL). Migrations
// 000001..latest are applied to a fresh database.
//
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costauditlog"
	plapp "github.com/mutugading/goapps-backend/services/finance/internal/application/periodlock"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

func TestPeriodLockRepositoryIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "period_lock_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	db := postgres.NewDBFromSQL(raw)
	repo := postgres.NewPeriodLockRepository(db)
	emitter := auditapp.NewEmitter(postgres.NewCostAuditLogRepository(db))
	const period = "202608"

	auditCount := func(op string) int {
		var n int
		require.NoError(t, raw.QueryRowContext(ctx,
			`SELECT count(*) FROM cost_audit_log WHERE cal_entity_type='cst_period_lock' AND cal_entity_id=$1 AND cal_operation=$2`,
			202608, op).Scan(&n))
		return n
	}

	// No row: not locked, Get = not found, FOR SHARE = false.
	locked, err := repo.IsLocked(ctx, period, periodlock.CalcTypeActual)
	require.NoError(t, err)
	assert.False(t, locked)
	_, err = repo.Get(ctx, period, periodlock.CalcTypeActual)
	assert.ErrorIs(t, err, periodlock.ErrLockNotFound)
	assert.False(t, forShare(ctx, t, raw, period))

	// Lock via handler -> IsLocked true, audit ERP_PERIOD_LOCK.
	_, err = plapp.NewLockHandler(repo, emitter).Handle(ctx, plapp.LockCommand{Period: period, User: "alice", Reason: "close"})
	require.NoError(t, err)
	locked, err = repo.IsLocked(ctx, period, periodlock.CalcTypeActual)
	require.NoError(t, err)
	assert.True(t, locked)
	locked, err = repo.IsLocked(ctx, period, "FORECAST")
	require.NoError(t, err)
	assert.False(t, locked, "FORECAST is never locked")
	assert.True(t, forShare(ctx, t, raw, period))
	assert.Equal(t, 1, auditCount("ERP_PERIOD_LOCK"))

	got, err := repo.Get(ctx, period, periodlock.CalcTypeActual)
	require.NoError(t, err)
	assert.True(t, got.IsLocked())
	assert.Equal(t, "alice", got.LockedBy())
	assert.Nil(t, got.ErpBatchID())

	// Repo-level double lock is race-safe.
	dup, err := periodlock.NewPeriodLock(period, periodlock.CalcTypeActual, "x", "y", time.Now())
	require.NoError(t, err)
	assert.ErrorIs(t, repo.Lock(ctx, dup), periodlock.ErrAlreadyLocked)

	// Batches: one PUSHED LIVE (flag on unlock), one SUPERSEDED (untouched).
	// uix_ceib_inflight allows only one in-flight batch per period.
	pushed := insertBatch(ctx, t, raw, period, 1, "PUSHED")
	superseded := insertBatch(ctx, t, raw, period, 2, "SUPERSEDED")

	// Unlock of a posted period is refused; row stays locked.
	_, err = plapp.NewUnlockHandler(repo, plapp.StaticAdjPostedProbe{Posted: true}, emitter).Handle(ctx,
		plapp.UnlockCommand{Period: period, User: "bob", Reason: "reopen"})
	assert.ErrorIs(t, err, periodlock.ErrUnlockPostedPeriod)
	locked, _ = repo.IsLocked(ctx, period, periodlock.CalcTypeActual)
	assert.True(t, locked)
	assert.Equal(t, 0, auditCount("ERP_PERIOD_UNLOCK"))

	// Unlock refused while a batch is LOCKED.
	lockedBatch := insertBatch(ctx, t, raw, period, 3, "LOCKED")
	_, err = plapp.NewUnlockHandler(repo, plapp.StaticAdjPostedProbe{}, emitter).Handle(ctx,
		plapp.UnlockCommand{Period: period, User: "bob", Reason: "reopen"})
	assert.ErrorIs(t, err, periodlock.ErrUnlockBatchLocked)
	_, err = raw.ExecContext(ctx, `DELETE FROM cst_erp_int_batch WHERE ceib_batch_id=$1`, lockedBatch)
	require.NoError(t, err)

	// Unlock succeeds: soft unlock, repush flag, audit with before-snapshot.
	res, err := plapp.NewUnlockHandler(repo, plapp.StaticAdjPostedProbe{}, emitter).Handle(ctx,
		plapp.UnlockCommand{Period: period, User: "bob", Reason: "reopen"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), res.RepushFlagged)
	locked, _ = repo.IsLocked(ctx, period, periodlock.CalcTypeActual)
	assert.False(t, locked)
	assert.False(t, forShare(ctx, t, raw, period))
	assert.True(t, needsRepush(ctx, t, raw, pushed))
	assert.False(t, needsRepush(ctx, t, raw, superseded))
	assert.Equal(t, 1, auditCount("ERP_PERIOD_UNLOCK"))
	var beforeLocked bool
	require.NoError(t, raw.QueryRowContext(ctx,
		`SELECT (cal_before_data->>'locked')::boolean FROM cost_audit_log WHERE cal_operation='ERP_PERIOD_UNLOCK' AND cal_entity_id=202608`).Scan(&beforeLocked))
	assert.True(t, beforeLocked)

	got, err = repo.Get(ctx, period, periodlock.CalcTypeActual)
	require.NoError(t, err)
	assert.False(t, got.IsLocked())
	assert.Equal(t, "bob", got.UnlockedBy())
	assert.Equal(t, "reopen", got.UnlockReason())

	// Repo-level unlock of an unlocked row is race-safe.
	stale := periodlock.Reconstruct(period, periodlock.CalcTypeActual, time.Now(), "a", "r", nil, ptrTime(time.Now()), "b", "r")
	_, err = repo.Unlock(ctx, stale)
	assert.ErrorIs(t, err, periodlock.ErrNotLocked)

	// Re-lock reuses the PK.
	_, err = plapp.NewLockHandler(repo, emitter).Handle(ctx, plapp.LockCommand{Period: period, User: "carol", Reason: "again"})
	require.NoError(t, err)
	got, err = repo.Get(ctx, period, periodlock.CalcTypeActual)
	require.NoError(t, err)
	assert.True(t, got.IsLocked())
	assert.Equal(t, "carol", got.LockedBy())
	assert.Nil(t, got.UnlockedAt())
	assert.Equal(t, 2, auditCount("ERP_PERIOD_LOCK"))

	// FOR SHARE blocks a concurrent unlock until the reader's tx ends.
	tx, err := raw.BeginTx(ctx, nil)
	require.NoError(t, err)
	isLocked, err := repo.IsLockedForShare(ctx, tx, period, periodlock.CalcTypeActual)
	require.NoError(t, err)
	assert.True(t, isLocked)
	done := make(chan error, 1)
	go func() {
		cctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		_, e := raw.ExecContext(cctx, `UPDATE cst_period_lock SET cpl_unlocked_at=NOW(), cpl_unlocked_by='z', cpl_unlock_reason='z' WHERE cpl_period=$1`, period)
		done <- e
	}()
	assert.Error(t, <-done, "unlock must wait on the FOR SHARE row lock")
	require.NoError(t, tx.Rollback())
}

func forShare(ctx context.Context, t *testing.T, db *sql.DB, period string) bool {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // read-only tx
	v, err := postgres.IsPeriodLockedForShare(ctx, tx, period, periodlock.CalcTypeActual)
	require.NoError(t, err)
	return v
}

func insertBatch(ctx context.Context, t *testing.T, db *sql.DB, period string, seq int, status string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO cst_erp_int_batch (ceib_period, ceib_seq, ceib_status, created_by)
		VALUES ($1, $2, $3, 'integ-test') RETURNING ceib_batch_id`, period, seq, status).Scan(&id))
	return id
}

func needsRepush(ctx context.Context, t *testing.T, db *sql.DB, id int64) bool {
	t.Helper()
	var v bool
	require.NoError(t, db.QueryRowContext(ctx, `SELECT ceib_needs_repush FROM cst_erp_int_batch WHERE ceib_batch_id=$1`, id).Scan(&v))
	return v
}

func ptrTime(v time.Time) *time.Time { return &v }
