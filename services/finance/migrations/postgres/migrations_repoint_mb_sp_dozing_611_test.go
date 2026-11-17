// Tests for 000611 (repoint MB_SP_DOZING lookup_source_column mbs_dozing ->
// mbs_run_ldr_pct, D31 L1). Text test needs no DB; the integration test runs on
// a testcontainers PostgreSQL and is skipped unless INTEGRATION_TEST=true.
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
	preRepointMBSPDozing611 uint64 = 610
	repointMBSPDozing611Ver uint64 = 611
)

// Text-level guard. The additive-only "no UPDATE" rule used by other migration
// text tests is intentionally NOT applied: this migration is a guarded data fix.
func TestRepointMBSPDozing611MigrationText(t *testing.T) {
	up := readMig(t, "000611_repoint_mb_sp_dozing_run_ldr.up.sql")
	down := readMig(t, "000611_repoint_mb_sp_dozing_run_ldr.down.sql")

	for name, sql := range map[string]string{"up": up, "down": down} {
		assert.Contains(t, sql, "BEGIN;", name)
		assert.Contains(t, sql, "COMMIT;", name)
		assert.NotContains(t, sql, "::", name)
		assert.Contains(t, sql, "UPDATE mst_parameter", name)
		assert.Contains(t, sql, "param_code = 'MB_SP_DOZING'", name)
		assert.NotContains(t, sql, "DELETE", name)
		assert.NotContains(t, sql, "DROP", name)
	}
	assert.Contains(t, up, "SET lookup_source_column = 'mbs_run_ldr_pct'")
	assert.Contains(t, up, "AND lookup_source_column = 'mbs_dozing'", "up must be guarded")
	assert.Contains(t, down, "SET lookup_source_column = 'mbs_dozing'")
	assert.Contains(t, down, "AND lookup_source_column = 'mbs_run_ldr_pct'", "down must be guarded")
}

func TestRepointMBSPDozing611Migration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "repoint_mb_sp_dozing_611_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), repointMBSPDozing611Ver)
	require.NoError(t, m.UpTo(ctx, preRepointMBSPDozing611))

	colQ := func(col string) int {
		return count(ctx, t, db, `SELECT count(*) FROM mst_parameter
			WHERE param_code = 'MB_SP_DOZING' AND deleted_at IS NULL AND lookup_source_column = '`+col+`'`)
	}
	registered := count(ctx, t, db, `SELECT count(*) FROM mst_lookup_master_column
		WHERE lmc_master_code = 'MB_SPIN' AND lmc_column_name = 'mbs_run_ldr_pct'`)
	require.Equal(t, 1, registered, "mbs_run_ldr_pct must be a registered lookup column")

	// Pre-state (610): legacy column.
	require.Equal(t, 1, colQ("mbs_dozing"))

	// Up: repointed.
	require.NoError(t, m.UpTo(ctx, repointMBSPDozing611Ver))
	assert.Equal(t, 1, colQ("mbs_run_ldr_pct"))
	assert.Equal(t, 0, colQ("mbs_dozing"))

	// Up SQL twice: idempotent no-op.
	sqlUp, err := os.ReadFile("000611_repoint_mb_sp_dozing_run_ldr.up.sql")
	require.NoError(t, err)
	mustExec(ctx, t, db, string(sqlUp))
	mustExec(ctx, t, db, string(sqlUp))
	assert.Equal(t, 1, colQ("mbs_run_ldr_pct"))

	// Down: restored.
	require.NoError(t, m.DownTo(ctx, preRepointMBSPDozing611))
	assert.Equal(t, 1, colQ("mbs_dozing"))
	assert.Equal(t, 0, colQ("mbs_run_ldr_pct"))

	// Up again after down.
	require.NoError(t, m.UpTo(ctx, repointMBSPDozing611Ver))
	assert.Equal(t, 1, colQ("mbs_run_ldr_pct"))
}
