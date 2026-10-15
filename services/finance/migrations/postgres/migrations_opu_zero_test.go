// Migration round-trip test for 000573 (OPU = 0 for finance-confirmed no-oil
// products). Local PostgreSQL only, via internal/testutil/pgcontainer.
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

const (
	preOpuZero uint64 = 572
	opuZeroVer uint64 = 573
)

func TestOpuZeroMigration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "opu_zero_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), opuZeroVer)
	require.NoError(t, m.UpTo(ctx, preOpuZero))

	// Fixtures: legacy ids 100/10000/10001/10018 are in the list, 999999999 is not.
	// T573A OPU=2.2, T573B CAPP with no value, T573C no OPU CAPP, T573D already 0,
	// T573X (not in list) OPU=5.
	mustExec(ctx, t, db, `INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by, cpm_flex_02)
		SELECT v.c, (SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'PTY'), 't573', 't573', 't573', v.f
		FROM (VALUES ('T573A','100'), ('T573B','10000'), ('T573C','10001'), ('T573D','10018'), ('T573X','999999999')) v(c, f)`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_applicable_param (capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by)
		SELECT pm.cpm_product_sys_id, mp.id, FALSE, 't573'
		FROM cost_product_master pm, mst_parameter mp
		WHERE mp.param_code = 'OPU' AND mp.deleted_at IS NULL
		  AND pm.cpm_product_code IN ('T573A','T573B','T573D','T573X')
		ON CONFLICT ON CONSTRAINT capp_unique_product_param DO NOTHING`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
		SELECT pm.cpm_product_sys_id, mp.id, v.val, 't573', 't573'
		FROM (VALUES ('T573A', 2.2), ('T573D', 0), ('T573X', 5)) v(c, val)
		JOIN cost_product_master pm ON pm.cpm_product_code = v.c
		JOIN mst_parameter mp ON mp.param_code = 'OPU' AND mp.deleted_at IS NULL`)

	opu := func(code string) sql.NullFloat64 {
		var v sql.NullFloat64
		err := db.QueryRowContext(ctx, `SELECT p.cpp_value_numeric FROM cost_product_parameter p
			JOIN cost_product_master pm ON pm.cpm_product_sys_id = p.cpp_product_sys_id
			JOIN mst_parameter mp ON mp.id = p.cpp_param_id AND mp.param_code = 'OPU'
			WHERE pm.cpm_product_code = $1`, code).Scan(&v)
		if err == sql.ErrNoRows {
			return sql.NullFloat64{}
		}
		require.NoError(t, err)
		return v
	}
	rowCount := func(code string) int {
		return count(ctx, t, db, `SELECT count(*) FROM cost_product_parameter p
			JOIN cost_product_master pm ON pm.cpm_product_sys_id = p.cpp_product_sys_id
			JOIN mst_parameter mp ON mp.id = p.cpp_param_id AND mp.param_code = 'OPU'
			WHERE pm.cpm_product_code = '`+code+`'`)
	}
	zero := sql.NullFloat64{Float64: 0, Valid: true}
	assertUp := func(t *testing.T) {
		t.Helper()
		assert.Equal(t, zero, opu("T573A"))
		assert.Equal(t, zero, opu("T573B"))
		assert.Equal(t, zero, opu("T573D"))
		assert.Equal(t, 0, rowCount("T573C"), "no OPU CAPP -> untouched, no row added")
		assert.Equal(t, sql.NullFloat64{Float64: 5, Valid: true}, opu("T573X"), "outside the list untouched")
	}

	require.NoError(t, m.UpTo(ctx, opuZeroVer))
	assertUp(t)
	assert.Equal(t, 3, count(ctx, t, db, `SELECT count(*) FROM cost_product_parameter_opu_backup_573`))

	// Idempotent re-run of the up SQL itself.
	sqlUp, err := os.ReadFile("000573_set_opu_zero_for_no_oil_products.up.sql")
	require.NoError(t, err)
	mustExec(ctx, t, db, string(sqlUp))
	assertUp(t)
	assert.Equal(t, 3, count(ctx, t, db, `SELECT count(*) FROM cost_product_parameter_opu_backup_573`))

	require.NoError(t, m.DownTo(ctx, preOpuZero))
	assert.Equal(t, sql.NullFloat64{Float64: 2.2, Valid: true}, opu("T573A"))
	assert.False(t, opu("T573B").Valid)
	assert.Equal(t, 0, rowCount("T573B"), "row inserted by up is removed by down")
	assert.Equal(t, zero, opu("T573D"))
	assert.Equal(t, sql.NullFloat64{Float64: 5, Valid: true}, opu("T573X"))
	assert.False(t, tableExists(ctx, t, db, "cost_product_parameter_opu_backup_573"))

	require.NoError(t, m.UpTo(ctx, opuZeroVer))
	assertUp(t)
}
