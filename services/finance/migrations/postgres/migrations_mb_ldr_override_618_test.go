// Migration round-trip test for 000618 (MB_LDR_OVERRIDE inline in RP_DOZING / MB_COST / MB_FLAG).
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

func TestMBLDROverride618Migration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	const ver uint64 = 618
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "mb_ldr_override_618_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), ver)
	require.NoError(t, m.UpTo(ctx, ver-1))

	expr := func(code string) string {
		var s string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT expression FROM mst_formula WHERE formula_code=$1 AND deleted_at IS NULL`, code).Scan(&s))
		return s
	}
	edge := func(f string) int {
		return count(ctx, t, db, `SELECT count(*) FROM formula_param fp JOIN mst_formula f ON f.id=fp.formula_id
			JOIN mst_parameter p ON p.id=fp.param_id WHERE f.formula_code='`+f+`' AND p.param_code='MB_LDR_OVERRIDE'`)
	}
	// Pin the exact expected pre-618 texts (fresh DBs do not carry the prod-edited ones).
	const (
		rpOld   = `RP_DOZING_ACTUAL > 0 ? RP_DOZING_ACTUAL : (MB_SP_DOZING > 0 ? MB_SP_DOZING : 0)`
		costOld = `IS_SUPERBA == 1 ? SUPERBA_MB_COST : (RP_DOZING_ACTUAL > 0 ? (MB_RATE_MKT * RP_DOZING_ACTUAL / 100.0) : (((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0))`
		flagOld = `MB_SP_DOZING > 0 ? 'Y' : 'N'`
		driftd  = `MB_SP_DOZING > 0 ? 'Y' : 'N' /* drifted */`
	)
	pin := func(code, e string) {
		_, err := db.ExecContext(ctx, `UPDATE mst_formula SET expression=$1 WHERE formula_code=$2`, e, code)
		require.NoError(t, err)
	}
	pin("F_YARN_RP_DOZING", rpOld)
	pin("F_YARN_MB_COST", costOld)
	pin("F_YARN_MB_FLAG", driftd) // partial drift: 2 of 3 match -> migration must abort, nothing changes

	require.Error(t, m.UpTo(ctx, ver))
	assert.Equal(t, rpOld, expr("F_YARN_RP_DOZING"))
	assert.Equal(t, costOld, expr("F_YARN_MB_COST"))
	assert.Equal(t, driftd, expr("F_YARN_MB_FLAG"))
	assert.Equal(t, 0, count(ctx, t, db, `SELECT count(*) FROM mst_parameter WHERE param_code='MB_LDR_OVERRIDE'`), "aborted migration leaves no param")

	// Restore the exact flag text: all three are rewritten.
	pin("F_YARN_MB_FLAG", flagOld)
	require.NoError(t, m.UpTo(ctx, ver))
	const flagNew = `(MB_LDR_OVERRIDE > 0 ? MB_LDR_OVERRIDE : MB_SP_DOZING) > 0 ? 'Y' : 'N'`
	assert.Contains(t, expr("F_YARN_RP_DOZING"), "RP_DOZING_ACTUAL > 0 ? RP_DOZING_ACTUAL : (MB_LDR_OVERRIDE > 0 ? MB_LDR_OVERRIDE :")
	assert.Contains(t, expr("F_YARN_MB_COST"), "(MB_LDR_OVERRIDE > 0 ? MB_LDR_OVERRIDE : MB_SP_DOZING)")
	assert.Equal(t, flagNew, expr("F_YARN_MB_FLAG"))
	for _, f := range []string{"F_YARN_RP_DOZING", "F_YARN_MB_COST", "F_YARN_MB_FLAG"} {
		assert.Equal(t, 1, edge(f), f)
	}
	assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM mst_parameter WHERE param_code='MB_LDR_OVERRIDE' AND param_category='INPUT' AND deleted_at IS NULL`))

	// Re-running the up SQL is a no-op (expressions and edge counts unchanged).
	before := []string{expr("F_YARN_RP_DOZING"), expr("F_YARN_MB_COST"), expr("F_YARN_MB_FLAG")}
	up, err := os.ReadFile("000618_mb_ldr_override.up.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(up))
	require.NoError(t, err)
	assert.Equal(t, before, []string{expr("F_YARN_RP_DOZING"), expr("F_YARN_MB_COST"), expr("F_YARN_MB_FLAG")})
	assert.Equal(t, 3, edge("F_YARN_RP_DOZING")+edge("F_YARN_MB_COST")+edge("F_YARN_MB_FLAG"))

	// Down restores the exact old texts, edges, param and backup table.
	require.NoError(t, m.DownTo(ctx, ver-1))
	assert.Equal(t, rpOld, expr("F_YARN_RP_DOZING"))
	assert.Equal(t, costOld, expr("F_YARN_MB_COST"))
	assert.Equal(t, flagOld, expr("F_YARN_MB_FLAG"))
	assert.Equal(t, 0, edge("F_YARN_RP_DOZING")+edge("F_YARN_MB_COST")+edge("F_YARN_MB_FLAG"))
	assert.Equal(t, 0, count(ctx, t, db, `SELECT count(*) FROM mst_parameter WHERE param_code='MB_LDR_OVERRIDE'`))
	assert.Equal(t, 0, count(ctx, t, db, `SELECT count(*) FROM information_schema.tables WHERE table_name='bak_000618_formula'`))
}
