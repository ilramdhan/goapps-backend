// Migration round-trip test for 000614 (mst_machine_param_default + seed +
// CAPP/CPP backfill). Local PostgreSQL only, via internal/testutil/pgcontainer.
// Skipped unless INTEGRATION_TEST=true.
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
	preMachineParamDefault614 uint64 = 612
	machineParamDefault614Ver uint64 = 614
)

func TestMachineParamDefault614Migration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "machine_param_default_614_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), machineParamDefault614Ver)
	require.NoError(t, m.UpTo(ctx, preMachineParamDefault614))

	// Fixture: machines, params (only if missing), 3 products on 'sp dyeing '
	// (case/space variants) + 1 on Conning (no default).
	mustExec(ctx, t, db, `INSERT INTO mst_machine (mc_code, mc_name, created_by)
		SELECT v.c, v.c, 't614' FROM (VALUES ('SP Dyeing'), ('Hanking (Cone to Hank)'), ('Washing'), ('Conning (Hank to cone)')) v(c)
		WHERE NOT EXISTS (SELECT 1 FROM mst_machine m WHERE m.mc_code = v.c AND m.deleted_at IS NULL)`)
	for _, code := range []string{"MC_NAME", "MB_COST_MKT", "STEAM_COST_CNG"} {
		require.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM mst_parameter WHERE deleted_at IS NULL AND param_code = '`+code+`'`), code)
	}
	mustExec(ctx, t, db, `INSERT INTO cost_product_type (cpt_type_code, cpt_type_name)
		VALUES ('ACY', 'ACY') ON CONFLICT (cpt_type_code) DO NOTHING`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
		SELECT v.c, (SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'ACY'), 't614', 't614', 't614'
		FROM (VALUES ('T614A'), ('T614B'), ('T614C')) v(c)`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_filled_by, cpp_created_by)
		SELECT pm.cpm_product_sys_id, p.id, v.mc, 't614', 't614'
		FROM (VALUES ('T614A', ' sp dyeing '), ('T614B', 'SP Dyeing'), ('T614C', 'Conning (Hank to cone)')) v(code, mc)
		JOIN cost_product_master pm ON pm.cpm_product_code = v.code
		JOIN mst_parameter p ON p.param_code = 'MC_NAME' AND p.deleted_at IS NULL`)
	// T614B already has an edited MB_COST_MKT value (attached + 0.1).
	mustExec(ctx, t, db, `INSERT INTO cost_product_applicable_param (capp_product_sys_id, capp_param_id, capp_created_by)
		SELECT pm.cpm_product_sys_id, p.id, 't614' FROM cost_product_master pm
		JOIN mst_parameter p ON p.param_code = 'MB_COST_MKT' AND p.deleted_at IS NULL WHERE pm.cpm_product_code = 'T614B'`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
		SELECT pm.cpm_product_sys_id, p.id, 0.1, 't614', 't614' FROM cost_product_master pm
		JOIN mst_parameter p ON p.param_code = 'MB_COST_MKT' AND p.deleted_at IS NULL WHERE pm.cpm_product_code = 'T614B'`)

	mbCost := func(code string) (float64, bool) {
		var v float64
		err := db.QueryRowContext(ctx, `SELECT cpp.cpp_value_numeric FROM cost_product_parameter cpp
			JOIN cost_product_master pm ON pm.cpm_product_sys_id = cpp.cpp_product_sys_id
			JOIN mst_parameter p ON p.id = cpp.cpp_param_id AND p.param_code = 'MB_COST_MKT'
			WHERE pm.cpm_product_code = $1`, code).Scan(&v)
		return v, err == nil
	}
	attached := func(code string) int {
		return count(ctx, t, db, `SELECT count(*) FROM cost_product_applicable_param c
			JOIN cost_product_master pm ON pm.cpm_product_sys_id = c.capp_product_sys_id
			JOIN mst_parameter p ON p.id = c.capp_param_id AND p.param_code = 'MB_COST_MKT'
			WHERE pm.cpm_product_code = '`+code+`'`)
	}
	assertUp := func(t *testing.T) {
		t.Helper()
		assert.True(t, tableExists(ctx, t, db, "mst_machine_param_default"))
		assert.Equal(t, 3, count(ctx, t, db, `SELECT count(*) FROM mst_machine_param_default WHERE deleted_at IS NULL`))
		v, ok := mbCost("T614A")
		assert.True(t, ok, "missing value is filled")
		assert.InDelta(t, 0.08, v, 1e-9)
		v, _ = mbCost("T614B")
		assert.InDelta(t, 0.1, v, 1e-9, "existing value is kept")
		_, ok = mbCost("T614C")
		assert.False(t, ok, "machine without default untouched")
		assert.Equal(t, 1, attached("T614A"))
		assert.Equal(t, 1, attached("T614B"))
		assert.Equal(t, 0, attached("T614C"))
	}

	require.NoError(t, m.UpTo(ctx, machineParamDefault614Ver))
	assertUp(t)

	// Idempotent re-run of the up SQL.
	sqlUp, err := os.ReadFile("000614_machine_param_default.up.sql")
	require.NoError(t, err)
	mustExec(ctx, t, db, string(sqlUp))
	assertUp(t)

	require.NoError(t, m.DownTo(ctx, preMachineParamDefault614))
	assert.False(t, tableExists(ctx, t, db, "mst_machine_param_default"))
	_, ok := mbCost("T614A")
	assert.False(t, ok, "backfilled value removed")
	v, _ := mbCost("T614B")
	assert.InDelta(t, 0.1, v, 1e-9, "pre-existing value survives down")
	assert.Equal(t, 0, attached("T614A"))
	assert.Equal(t, 1, attached("T614B"))

	require.NoError(t, m.UpTo(ctx, machineParamDefault614Ver))
	assertUp(t)
}
