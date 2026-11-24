// Migration round-trip test for 000616 (actual CONV_FACTOR / RP_DOZING inputs).
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

func TestMBActualConvRPDozing616Migration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	const ver uint64 = 616
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "mb_actual_conv_rp_dozing_616_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), ver)
	require.NoError(t, m.UpTo(ctx, ver-1))

	expr := func(code string) string {
		var s string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT expression FROM mst_formula WHERE formula_code=$1 AND deleted_at IS NULL`, code).Scan(&s))
		return s
	}
	edge := func(f, p string) int {
		return count(ctx, t, db, `SELECT count(*) FROM formula_param fp JOIN mst_formula f ON f.id=fp.formula_id
			JOIN mst_parameter p ON p.id=fp.param_id WHERE f.formula_code='`+f+`' AND p.param_code='`+p+`'`)
	}
	// Fresh DBs do not carry the prod-edited texts; pin them so the exact-text guards match.
	const costOld = `IS_SUPERBA == 1 ? SUPERBA_MB_COST : (((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0)`
	mustExec(ctx, t, db, `UPDATE mst_formula SET expression='1' WHERE formula_code='F_YARN_CONV_FACTOR'`)
	mustExec(ctx, t, db, `UPDATE mst_formula SET expression='MB_SP_DOZING > 0 ? MB_SP_DOZING : 0' WHERE formula_code='F_YARN_RP_DOZING'`)
	mustExec(ctx, t, db, `UPDATE mst_formula SET expression='`+costOld+`' WHERE formula_code='F_YARN_MB_COST'`)

	require.NoError(t, m.UpTo(ctx, ver))
	assert.Equal(t, `CONV_FACTOR_ACTUAL > 0 ? CONV_FACTOR_ACTUAL : 1`, expr("F_YARN_CONV_FACTOR"))
	assert.Equal(t, `RP_DOZING_ACTUAL > 0 ? RP_DOZING_ACTUAL : (MB_SP_DOZING > 0 ? MB_SP_DOZING : 0)`, expr("F_YARN_RP_DOZING"))
	assert.Contains(t, expr("F_YARN_MB_COST"), "RP_DOZING_ACTUAL > 0 ? (MB_RATE_MKT * RP_DOZING_ACTUAL / 100.0)")
	assert.Equal(t, 1, edge("F_YARN_CONV_FACTOR", "CONV_FACTOR_ACTUAL"))
	assert.Equal(t, 1, edge("F_YARN_RP_DOZING", "RP_DOZING_ACTUAL"))
	assert.Equal(t, 1, edge("F_YARN_MB_COST", "RP_DOZING_ACTUAL"))
	assert.Equal(t, 2, count(ctx, t, db, `SELECT count(*) FROM mst_parameter WHERE param_code IN ('CONV_FACTOR_ACTUAL','RP_DOZING_ACTUAL') AND param_category='INPUT' AND deleted_at IS NULL`))

	require.NoError(t, m.DownTo(ctx, ver-1))
	assert.Equal(t, "1", expr("F_YARN_CONV_FACTOR"))
	assert.Equal(t, `MB_SP_DOZING > 0 ? MB_SP_DOZING : 0`, expr("F_YARN_RP_DOZING"))
	assert.Equal(t, costOld, expr("F_YARN_MB_COST"))
	assert.Equal(t, 0, edge("F_YARN_CONV_FACTOR", "CONV_FACTOR_ACTUAL")+edge("F_YARN_RP_DOZING", "RP_DOZING_ACTUAL")+edge("F_YARN_MB_COST", "RP_DOZING_ACTUAL"))
	assert.Equal(t, 0, count(ctx, t, db, `SELECT count(*) FROM mst_parameter WHERE param_code IN ('CONV_FACTOR_ACTUAL','RP_DOZING_ACTUAL')`))
	assert.Equal(t, 0, count(ctx, t, db, `SELECT count(*) FROM information_schema.tables WHERE table_name='bak_000616_formula'`))
}
