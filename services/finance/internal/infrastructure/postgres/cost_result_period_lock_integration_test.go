// Integration test for the supersedePrevious period-lock guard (plan-02
// P1-T4, AC-09). LOCAL PostgreSQL only, via internal/testutil/pgcontainer (a
// throwaway postgres:16-alpine container, or a LOCAL TEST_DATABASE_URL).
//
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

func TestCostResultSupersedePeriodLockIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "cost_result_period_lock_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))

	db := postgres.NewDBFromSQL(raw)
	results := postgres.NewCostResultRepository(db)
	locks := postgres.NewPeriodLockRepository(db)
	productID, headID := seedLockProductAndRoute(ctx, t, raw)
	const period = "202608"

	newRes := func(ct costcalc.CalculationType, total float64) *costcalc.Result {
		return costcalc.NewResult(
			productID, period, ct, headID, 1,
			total, 6, 4, total, 0, "USD",
			nil, nil, nil, nil, fmt.Sprintf("h-%s-%v", ct, total), 0, "integ-test",
			0, 0, 0, 0, 0, 0, 0,
		)
	}
	activeTotal := func(ct costcalc.CalculationType) (float64, int) {
		var total float64
		var version int
		require.NoError(t, raw.QueryRowContext(ctx, `
			SELECT cpc_total_cost, cpc_version FROM cst_product_cost
			 WHERE cpc_product_sys_id=$1 AND cpc_period=$2 AND cpc_calculation_type=$3
			   AND cpc_status <> 'SUPERSEDED'`, productID, period, string(ct)).Scan(&total, &version))
		return total, version
	}

	// Unlocked: behavior unchanged (v1 then supersede to v2).
	for _, ct := range []costcalc.CalculationType{costcalc.CalcTypeActual, costcalc.CalcTypeForecast} {
		_, _, _, _, err = results.UpsertWithSupersede(ctx, newRes(ct, 10))
		require.NoError(t, err)
		_, prevVersion, _, _, err := results.UpsertWithSupersede(ctx, newRes(ct, 11))
		require.NoError(t, err)
		assert.Equal(t, 1, prevVersion)
	}

	// Tx A locks P and commits.
	l, err := periodlock.NewPeriodLock(period, periodlock.CalcTypeActual, "alice", "close", time.Now())
	require.NoError(t, err)
	require.NoError(t, locks.Lock(ctx, l))

	// Tx B: ACTUAL supersede fails (both the autocommit and the caller-tx
	// variants) and nothing changes.
	_, _, _, _, err = results.UpsertWithSupersede(ctx, newRes(costcalc.CalcTypeActual, 99))
	require.ErrorIs(t, err, costcalc.ErrPeriodLocked)
	txB, err := raw.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, _, _, _, err = results.UpsertWithSupersedeTx(ctx, txB, newRes(costcalc.CalcTypeActual, 98))
	require.ErrorIs(t, err, costcalc.ErrPeriodLocked)
	require.NoError(t, txB.Rollback())
	total, version := activeTotal(costcalc.CalcTypeActual)
	assert.InDelta(t, 11, total, 1e-9)
	assert.Equal(t, 2, version)

	// FORECAST is unaffected by the ACTUAL lock.
	_, _, _, _, err = results.UpsertWithSupersede(ctx, newRes(costcalc.CalcTypeForecast, 12))
	require.NoError(t, err)
	total, version = activeTotal(costcalc.CalcTypeForecast)
	assert.InDelta(t, 12, total, 1e-9)
	assert.Equal(t, 3, version)

	// Unlock -> supersede works again.
	_, err = raw.ExecContext(ctx, `UPDATE cst_period_lock SET cpl_unlocked_at=NOW(), cpl_unlocked_by='bob', cpl_unlock_reason='reopen' WHERE cpl_period=$1`, period)
	require.NoError(t, err)
	_, _, _, _, err = results.UpsertWithSupersede(ctx, newRes(costcalc.CalcTypeActual, 13))
	require.NoError(t, err)
	total, version = activeTotal(costcalc.CalcTypeActual)
	assert.InDelta(t, 13, total, 1e-9)
	assert.Equal(t, 3, version)

	// Race: tx A re-locks the soft-unlocked row but has not committed. Tx B's
	// supersede blocks on FOR SHARE, then sees the committed lock and fails.
	txA, err := raw.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = txA.ExecContext(ctx, `UPDATE cst_period_lock SET cpl_unlocked_at=NULL, cpl_unlocked_by=NULL, cpl_unlock_reason=NULL WHERE cpl_period=$1`, period)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		_, _, _, _, e := results.UpsertWithSupersede(ctx, newRes(costcalc.CalcTypeActual, 97))
		done <- e
	}()
	select {
	case e := <-done:
		t.Fatalf("supersede must wait on tx A's row lock, returned early: %v", e)
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, txA.Commit())
	select {
	case e := <-done:
		require.ErrorIs(t, e, costcalc.ErrPeriodLocked)
	case <-time.After(10 * time.Second):
		t.Fatal("supersede did not resume after tx A committed")
	}
	total, _ = activeTotal(costcalc.CalcTypeActual)
	assert.InDelta(t, 13, total, 1e-9)

	// Repository-level verify / approve guard: with P locked, a direct
	// MarkVerified / MarkApproved on the ACTUAL row is refused in-tx (no
	// reliance on the handler pre-check), while FORECAST still transitions.
	activeID := func(ct costcalc.CalculationType) (int64, string) {
		var id int64
		var status string
		require.NoError(t, raw.QueryRowContext(ctx, `
			SELECT cpc_cost_id, cpc_status FROM cst_product_cost
			 WHERE cpc_product_sys_id=$1 AND cpc_period=$2 AND cpc_calculation_type=$3
			   AND cpc_status <> 'SUPERSEDED'`, productID, period, string(ct)).Scan(&id, &status))
		return id, status
	}
	actualID, _ := activeID(costcalc.CalcTypeActual)
	require.ErrorIs(t, results.MarkVerified(ctx, actualID, "v"), costcalc.ErrPeriodLocked)
	_, err = raw.ExecContext(ctx, `UPDATE cst_product_cost SET cpc_status='VERIFIED' WHERE cpc_cost_id=$1`, actualID)
	require.NoError(t, err)
	require.ErrorIs(t, results.MarkApproved(ctx, actualID, "a"), costcalc.ErrPeriodLocked)
	_, st := activeID(costcalc.CalcTypeActual)
	assert.Equal(t, "VERIFIED", st)
	forecastID, _ := activeID(costcalc.CalcTypeForecast)
	require.NoError(t, results.MarkVerified(ctx, forecastID, "v"))
	require.NoError(t, results.MarkApproved(ctx, forecastID, "a"))
	// A missing row keeps the pre-lock contract.
	require.ErrorIs(t, results.MarkVerified(ctx, 987654321, "v"), costcalc.ErrCostInvalidStatus)
}

func seedLockProductAndRoute(ctx context.Context, t *testing.T, db *sql.DB) (int64, int64) {
	t.Helper()
	var typeID int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT cpt_type_id FROM cost_product_type ORDER BY cpt_type_id LIMIT 1`).Scan(&typeID))
	var productID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO cost_product_master (
			cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by
		) VALUES ('PLK-IT-1', $1, 'period lock test product', 'integ-test', 'integ-test')
		RETURNING cpm_product_sys_id`, typeID).Scan(&productID))
	var headID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO cost_route_head (
			crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by
		) VALUES ($1, 'DRAFT', 1, 'integ-test', 'integ-test')
		RETURNING crh_head_id`, productID).Scan(&headID))
	return productID, headID
}
