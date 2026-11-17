// Migration round-trip test for 000610 (unlink MC_SPEED / MC_EFFICIENCY from
// the MC_NAME fill group). Local PostgreSQL only, via internal/testutil/pgcontainer.
// Skipped unless INTEGRATION_TEST=true. Asserts configuration rows only; product
// values (cost_product_parameter, mst_machine) are not touched by the migration.
package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

const (
	preUnlinkMCSpeedEff610 uint64 = 609
	unlinkMCSpeedEff610Ver uint64 = 610
)

func TestUnlinkMCSpeedEff610Migration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "unlink_mc_speed_eff_610_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), unlinkMCSpeedEff610Ver)
	require.NoError(t, m.UpTo(ctx, preUnlinkMCSpeedEff610))

	const linkedQ = `SELECT count(*) FROM mst_parameter
		WHERE param_code = $1 AND lookup_fill_group_code = 'MC_NAME' AND lookup_source_column = $2 AND deleted_at IS NULL`
	const registryQ = `SELECT count(*) FROM mst_lookup_master_column
		WHERE lmc_master_code = 'MACHINE' AND lmc_column_name IN ('mc_speed','mc_efficiency')`
	countLinked := func(code, col string) int {
		var n int
		require.NoError(t, db.QueryRowContext(ctx, linkedQ, code, col).Scan(&n))
		return n
	}
	machineChildren := func() int {
		return count(ctx, t, db, `SELECT count(*) FROM mst_parameter
			WHERE lookup_fill_group_code = 'MC_NAME' AND deleted_at IS NULL`)
	}

	// Pre-state (609): linked and registered.
	require.Equal(t, 1, countLinked("MC_SPEED", "mc_speed"))
	require.Equal(t, 1, countLinked("MC_EFFICIENCY", "mc_efficiency"))
	require.Equal(t, 2, count(ctx, t, db, registryQ))
	preChildren := machineChildren()
	preMachineRegistry := count(ctx, t, db, `SELECT count(*) FROM mst_lookup_master_column WHERE lmc_master_code = 'MACHINE'`)

	// Up (610): unlinked, registry rows removed, other children untouched.
	require.NoError(t, m.UpTo(ctx, unlinkMCSpeedEff610Ver))
	assert.Equal(t, 2, count(ctx, t, db, `SELECT count(*) FROM mst_parameter
		WHERE param_code IN ('MC_SPEED','MC_EFFICIENCY') AND lookup_fill_group_code IS NULL
		AND lookup_source_column IS NULL AND deleted_at IS NULL AND updated_by = 'migration_000610'`))
	assert.Equal(t, 0, count(ctx, t, db, registryQ))
	assert.Equal(t, preMachineRegistry-2, count(ctx, t, db, `SELECT count(*) FROM mst_lookup_master_column WHERE lmc_master_code = 'MACHINE'`))
	assert.Equal(t, preChildren-2, machineChildren())
	assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM mst_parameter WHERE param_code = 'NO_OF_POSITION' AND lookup_fill_group_code = 'MC_NAME' AND lookup_source_column = 'no_of_position' AND deleted_at IS NULL`))

	// Re-running the up SQL is a no-op.
	sqlUp, err := os.ReadFile("000610_unlink_mc_speed_eff_from_machine.up.sql")
	require.NoError(t, err)
	mustExec(ctx, t, db, string(sqlUp))
	assert.Equal(t, 0, count(ctx, t, db, registryQ))

	// Down (back to 609): links and registry rows restored.
	require.NoError(t, m.DownTo(ctx, preUnlinkMCSpeedEff610))
	assert.Equal(t, 1, countLinked("MC_SPEED", "mc_speed"))
	assert.Equal(t, 1, countLinked("MC_EFFICIENCY", "mc_efficiency"))
	assert.Equal(t, 2, count(ctx, t, db, registryQ))
	assert.Equal(t, preChildren, machineChildren())

	// Down SQL is idempotent: re-running it does not duplicate registry rows.
	sqlDown, err := os.ReadFile("000610_unlink_mc_speed_eff_from_machine.down.sql")
	require.NoError(t, err)
	mustExec(ctx, t, db, string(sqlDown))
	assert.Equal(t, 2, count(ctx, t, db, registryQ))
	assert.Equal(t, preMachineRegistry, count(ctx, t, db, `SELECT count(*) FROM mst_lookup_master_column WHERE lmc_master_code = 'MACHINE'`))

	// Up again after down: same result.
	require.NoError(t, m.UpTo(ctx, unlinkMCSpeedEff610Ver))
	assert.Equal(t, 0, count(ctx, t, db, registryQ))
}
