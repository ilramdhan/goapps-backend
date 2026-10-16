// Migration round-trip test for 000576 (OIL_NAME normalized to the allowed
// oil groups of the product type, plus PTY <- 202006077 / TTS <- 202006101
// extra mappings). Local PostgreSQL only, via internal/testutil/pgcontainer.
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
	preOilName576 uint64 = 575
	oilName576Ver uint64 = 576
)

func TestOilName576Migration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "oil_name_576_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), oilName576Ver)
	require.NoError(t, m.UpTo(ctx, preOilName576))

	// Fixtures at 000575: the two real oil groups, oil classes and the default
	// mappings prod has (PTY/MEL -> 202006101, TTS -> 202006077).
	mustExec(ctx, t, db, `INSERT INTO cst_rm_group_head (group_code, group_name, is_oil_group, created_by) VALUES
		('202006101', 'CONING OIL', TRUE, 't'), ('202006077', 'SPIN FINISH OIL', TRUE, 't')`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_type (cpt_type_code, cpt_type_name)
		VALUES ('PTY', 'PTY'), ('MEL', 'MEL'), ('TTS', 'TTS')
		ON CONFLICT (cpt_type_code) DO NOTHING`)
	mustExec(ctx, t, db, `UPDATE cost_product_type SET cpt_oil_class = CASE cpt_type_code WHEN 'TTS' THEN 'SUPERBA' ELSE 'PTY' END
		WHERE cpt_type_code IN ('PTY', 'MEL', 'TTS')`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_type_oil_group (cptog_type_id, cptog_group_head_id, cptog_is_default, cptog_created_by)
		SELECT pt.cpt_type_id, g.group_head_id, TRUE, 't'
		FROM (VALUES ('PTY', '202006101'), ('MEL', '202006101'), ('TTS', '202006077')) v(tc, gc)
		JOIN cost_product_type pt ON pt.cpt_type_code = v.tc
		JOIN cst_rm_group_head g ON g.group_code = v.gc
		ON CONFLICT ON CONSTRAINT uk_cptog_type_group DO NOTHING`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
		SELECT v.c, (SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = v.t), 't576', 't576', 't576'
		FROM (VALUES ('T576M1', 'MEL'), ('T576M2', 'MEL'), ('T576P1', 'PTY'), ('T576P2', 'PTY'),
		             ('T576P3', 'PTY'), ('T576T1', 'TTS')) v(c, t)`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_filled_by, cpp_created_by)
		SELECT pm.cpm_product_sys_id, mp.id, v.val, 'orig', 't576'
		FROM (VALUES ('T576M1', 'CONING OIL'), ('T576M2', 'MGTC 14'), ('T576P1', ' 202006077 '),
		             ('T576P2', 'MGTC 14'), ('T576P3', '202006101'), ('T576T1', '202006101')) v(c, val)
		JOIN cost_product_master pm ON pm.cpm_product_code = v.c
		JOIN mst_parameter mp ON mp.param_code = 'OIL_NAME' AND mp.deleted_at IS NULL`)

	type oilRow struct{ val, filledBy, updatedBy sql.NullString }
	oil := func(code string) oilRow {
		var r oilRow
		require.NoError(t, db.QueryRowContext(ctx, `SELECT p.cpp_value_text, p.cpp_filled_by, p.cpp_updated_by
			FROM cost_product_parameter p
			JOIN cost_product_master pm ON pm.cpm_product_sys_id = p.cpp_product_sys_id
			JOIN mst_parameter mp ON mp.id = p.cpp_param_id AND mp.param_code = 'OIL_NAME'
			WHERE pm.cpm_product_code = $1`, code).Scan(&r.val, &r.filledBy, &r.updatedBy))
		return r
	}
	mapped := func(typeCode, groupCode string) int {
		return count(ctx, t, db, `SELECT count(*) FROM cost_product_type_oil_group o
			JOIN cost_product_type pt ON pt.cpt_type_id = o.cptog_type_id
			JOIN cst_rm_group_head g ON g.group_head_id = o.cptog_group_head_id
			WHERE NOT o.cptog_is_default AND pt.cpt_type_code = '`+typeCode+`' AND g.group_code = '`+groupCode+`'`)
	}
	assertRewritten := func(t *testing.T, code string) {
		t.Helper()
		r := oil(code)
		assert.Equal(t, "202006101", r.val.String, code)
		assert.Equal(t, "migration_000576", r.filledBy.String, code)
		assert.Equal(t, "migration_000576", r.updatedBy.String, code)
	}
	assertUntouched := func(t *testing.T, code, want string) {
		t.Helper()
		r := oil(code)
		assert.Equal(t, want, r.val.String, code)
		assert.Equal(t, "orig", r.filledBy.String, code)
	}
	assertUp := func(t *testing.T) {
		t.Helper()
		assertRewritten(t, "T576M1")
		assertRewritten(t, "T576M2")
		assertRewritten(t, "T576P2")
		assertUntouched(t, "T576P1", " 202006077 ")
		assertUntouched(t, "T576P3", "202006101")
		assertUntouched(t, "T576T1", "202006101")
		assert.Equal(t, 1, mapped("PTY", "202006077"))
		assert.Equal(t, 1, mapped("TTS", "202006101"))
		assert.Equal(t, 3, count(ctx, t, db, `SELECT count(*) FROM bak_oil_name_000576`))
		assert.Equal(t, 2, count(ctx, t, db, `SELECT count(*) FROM bak_oil_group_map_000576`))
	}

	require.NoError(t, m.UpTo(ctx, oilName576Ver))
	assertUp(t)

	// Idempotent re-run of the up SQL itself: nothing new backed up or changed.
	sqlUp, err := os.ReadFile("000576_normalize_oil_name_allowed_groups.up.sql")
	require.NoError(t, err)
	mustExec(ctx, t, db, string(sqlUp))
	assertUp(t)

	require.NoError(t, m.DownTo(ctx, preOilName576))
	assertUntouched(t, "T576M1", "CONING OIL")
	assertUntouched(t, "T576M2", "MGTC 14")
	assertUntouched(t, "T576P1", " 202006077 ")
	assertUntouched(t, "T576P2", "MGTC 14")
	assertUntouched(t, "T576P3", "202006101")
	assertUntouched(t, "T576T1", "202006101")
	assert.Equal(t, 0, mapped("PTY", "202006077"))
	assert.Equal(t, 0, mapped("TTS", "202006101"))
	assert.Equal(t, 3, count(ctx, t, db, `SELECT count(*) FROM cost_product_type_oil_group WHERE cptog_is_default`),
		"pre-existing default mappings kept")
	assert.False(t, tableExists(ctx, t, db, "bak_oil_name_000576"))
	assert.False(t, tableExists(ctx, t, db, "bak_oil_group_map_000576"))

	// Down then Up again: same result, backup counts stable.
	require.NoError(t, m.UpTo(ctx, oilName576Ver))
	assertUp(t)
}
