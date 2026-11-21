// Migration round-trip test for 000613 (boughtout MB: zero waste/fixed/others/conv).
// Local PostgreSQL only, via internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
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
	preMBBoughtout613 uint64 = 612
	mbBoughtout613Ver uint64 = 613
)

func TestMBBoughtout613Migration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "mb_boughtout_613_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), mbBoughtout613Ver)
	require.NoError(t, m.UpTo(ctx, preMBBoughtout613))

	const fourCodes = `('F_MB_WASTE_VAL','F_MB_FIXED_COST','F_MB_COST_OTHERS','F_MB_CONV_COST')`
	wrapped := func() int {
		return count(ctx, t, db, `SELECT count(*) FROM mst_formula WHERE formula_code IN `+fourCodes+`
			AND deleted_at IS NULL AND expression LIKE 'IS_BOUGHTOUT == 1 ? 0 : (%'`)
	}
	edges := func() int {
		return count(ctx, t, db, `SELECT count(*) FROM formula_param fp
			JOIN mst_formula f ON f.id = fp.formula_id JOIN mst_parameter p ON p.id = fp.param_id
			WHERE p.param_code = 'IS_BOUGHTOUT' AND f.formula_code IN `+fourCodes)
	}
	require.Equal(t, 0, wrapped())
	require.Equal(t, 0, edges())

	require.NoError(t, m.UpTo(ctx, mbBoughtout613Ver))
	assert.Equal(t, 4, wrapped())
	assert.Equal(t, 4, edges())
	// F_MB_FINAL_COST is untouched.
	assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM mst_formula WHERE formula_code = 'F_MB_FINAL_COST'
		AND expression = 'IS_BOUGHTOUT == 1 ? MB_RM_COST : MB_RM_COST + MB_CONV_COST'`))

	// Idempotent re-run: no double wrap, no extra edges.
	sqlUp, err := os.ReadFile("000613_mb_boughtout_skip_conversion.up.sql")
	require.NoError(t, err)
	mustExec(ctx, t, db, string(sqlUp))
	assert.Equal(t, 4, wrapped())
	assert.Equal(t, 4, edges())

	require.NoError(t, m.DownTo(ctx, preMBBoughtout613))
	assert.Equal(t, 0, wrapped())
	assert.Equal(t, 0, edges())
	assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM mst_formula WHERE formula_code = 'F_MB_CONV_COST'
		AND expression = '(MB_NO_PROCESS * MB_FIXED_TOTAL) + MB_COST_OTHERS + MB_WASTE_VAL'`))

	require.NoError(t, m.UpTo(ctx, mbBoughtout613Ver))
	assert.Equal(t, 4, wrapped())
}
