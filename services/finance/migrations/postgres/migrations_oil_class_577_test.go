// Migration round-trip test for 000577 (oil class PTY for ATY/ACY, PTY oil-group
// mapping copy, oil CAPP rows for OPU > 0, OIL_NAME normalization). Local
// PostgreSQL only, via internal/testutil/pgcontainer. Skipped unless
// INTEGRATION_TEST=true.
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
	preOilClass577 uint64 = 576
	oilClass577Ver uint64 = 577
)

func TestOilClass577Migration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "oil_class_577_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), oilClass577Ver)
	require.NoError(t, m.UpTo(ctx, preOilClass577))

	// Fixtures at 000576: oil groups, PTY with oil class + default/allowed
	// mapping; ATY/ACY without oil class or mapping.
	mustExec(ctx, t, db, `INSERT INTO cst_rm_group_head (group_code, group_name, is_oil_group, created_by) VALUES
		('202006101', 'CONING OIL', TRUE, 't'), ('202006077', 'SPIN FINISH OIL', TRUE, 't')
		ON CONFLICT DO NOTHING`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_type (cpt_type_code, cpt_type_name)
		VALUES ('PTY', 'PTY'), ('ATY', 'ATY'), ('ACY', 'ACY')
		ON CONFLICT (cpt_type_code) DO NOTHING`)
	mustExec(ctx, t, db, `UPDATE cost_product_type SET cpt_oil_class = CASE cpt_type_code WHEN 'PTY' THEN 'PTY' ELSE NULL END
		WHERE cpt_type_code IN ('PTY', 'ATY', 'ACY')`)
	mustExec(ctx, t, db, `DELETE FROM cost_product_type_oil_group og USING cost_product_type pt
		WHERE pt.cpt_type_id = og.cptog_type_id AND pt.cpt_type_code IN ('PTY', 'ATY', 'ACY')`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_type_oil_group (cptog_type_id, cptog_group_head_id, cptog_is_default, cptog_created_by)
		SELECT pt.cpt_type_id, g.group_head_id, v.d, 't'
		FROM (VALUES ('202006101', TRUE), ('202006077', FALSE)) v(gc, d)
		JOIN cst_rm_group_head g ON g.group_code = v.gc
		CROSS JOIN cost_product_type pt WHERE pt.cpt_type_code = 'PTY'`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
		SELECT v.c, (SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = v.t), 't577', 't577', 't577'
		FROM (VALUES ('T577A1', 'ATY'), ('T577A2', 'ATY'), ('T577C1', 'ACY'), ('T577C2', 'ACY'), ('T577P1', 'PTY')) v(c, t)`)
	// OPU values: A1 5, A2 0, C1 3, C2 none, P1 5.
	mustExec(ctx, t, db, `INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
		SELECT pm.cpm_product_sys_id, mp.id, v.val, 'orig', 't577'
		FROM (VALUES ('T577A1', 5), ('T577A2', 0), ('T577C1', 3), ('T577P1', 5)) v(c, val)
		JOIN cost_product_master pm ON pm.cpm_product_code = v.c
		JOIN mst_parameter mp ON mp.param_code = 'OPU' AND mp.deleted_at IS NULL`)
	// OIL_NAME text: A1/A2 'CONING OIL' (free text), P1 'MGTC 14' (PTY must stay).
	mustExec(ctx, t, db, `INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_filled_by, cpp_created_by)
		SELECT pm.cpm_product_sys_id, mp.id, v.val, 'orig', 't577'
		FROM (VALUES ('T577A1', 'CONING OIL'), ('T577A2', 'CONING OIL'), ('T577P1', 'MGTC 14')) v(c, val)
		JOIN cost_product_master pm ON pm.cpm_product_code = v.c
		JOIN mst_parameter mp ON mp.param_code = 'OIL_NAME' AND mp.deleted_at IS NULL`)

	oilClass := func(typeCode string) sql.NullString {
		var s sql.NullString
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT cpt_oil_class FROM cost_product_type WHERE cpt_type_code = $1`, typeCode).Scan(&s))
		return s
	}
	mappings := func(typeCode string, def bool) int {
		return count(ctx, t, db, `SELECT count(*) FROM cost_product_type_oil_group o
			JOIN cost_product_type pt ON pt.cpt_type_id = o.cptog_type_id
			WHERE pt.cpt_type_code = '`+typeCode+`' AND o.cptog_is_default = `+map[bool]string{true: "TRUE", false: "FALSE"}[def])
	}
	cappCount := func(code string) int {
		return count(ctx, t, db, `SELECT count(*) FROM cost_product_applicable_param c
			JOIN cost_product_master pm ON pm.cpm_product_sys_id = c.capp_product_sys_id
			WHERE pm.cpm_product_code = '`+code+`' AND c.capp_created_by = 'migration_000577'`)
	}
	type oilRow struct {
		found                    bool
		val, filledBy, updatedBy sql.NullString
	}
	oil := func(code string) oilRow {
		var r oilRow
		err := db.QueryRowContext(ctx, `SELECT p.cpp_value_text, p.cpp_filled_by, p.cpp_updated_by
			FROM cost_product_parameter p
			JOIN cost_product_master pm ON pm.cpm_product_sys_id = p.cpp_product_sys_id
			JOIN mst_parameter mp ON mp.id = p.cpp_param_id AND mp.param_code = 'OIL_NAME'
			WHERE pm.cpm_product_code = $1`, code).Scan(&r.val, &r.filledBy, &r.updatedBy)
		if err == nil {
			r.found = true
		} else {
			require.ErrorIs(t, err, sql.ErrNoRows)
		}
		return r
	}
	assertDefault := func(t *testing.T, code, filledBy string) {
		t.Helper()
		r := oil(code)
		require.True(t, r.found, code)
		assert.Equal(t, "202006101", r.val.String, code)
		assert.Equal(t, filledBy, r.filledBy.String, code)
		assert.Equal(t, "migration_000577", r.updatedBy.String, code)
	}
	assertUp := func(t *testing.T) {
		t.Helper()
		assert.Equal(t, "PTY", oilClass("ATY").String)
		assert.Equal(t, "PTY", oilClass("ACY").String)
		for _, ty := range []string{"ATY", "ACY"} {
			assert.Equal(t, 1, mappings(ty, true), ty)
			assert.Equal(t, 1, mappings(ty, false), ty)
		}
		assert.Equal(t, 7, cappCount("T577A1"))
		assert.Equal(t, 7, cappCount("T577C1"))
		assert.Equal(t, 0, cappCount("T577A2"), "OPU 0 gets no CAPP")
		assert.Equal(t, 0, cappCount("T577C2"), "no OPU gets no CAPP")
		assert.Equal(t, 0, cappCount("T577P1"))
		assertDefault(t, "T577A1", "migration_000577")
		assertDefault(t, "T577A2", "migration_000577")
		assertDefault(t, "T577C1", "migration_000577") // inserted row
		assert.False(t, oil("T577C2").found)
		p := oil("T577P1")
		assert.Equal(t, "MGTC 14", p.val.String, "PTY untouched")
		assert.Equal(t, "orig", p.filledBy.String)
		assert.Equal(t, 2, count(ctx, t, db, `SELECT count(*) FROM bak_oil_name_000577 WHERE had_row`))
		assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM bak_oil_name_000577 WHERE NOT had_row`))
		assert.Equal(t, 4, count(ctx, t, db, `SELECT count(*) FROM bak_oil_group_map_000577`))
		assert.Equal(t, 2, count(ctx, t, db, `SELECT count(*) FROM bak_oil_class_000577`))
	}

	require.NoError(t, m.UpTo(ctx, oilClass577Ver))
	assertUp(t)

	// Idempotent re-run of the up SQL itself.
	sqlUp, err := os.ReadFile("000577_oil_class_aty_acy.up.sql")
	require.NoError(t, err)
	mustExec(ctx, t, db, string(sqlUp))
	assertUp(t)

	require.NoError(t, m.DownTo(ctx, preOilClass577))
	assert.False(t, oilClass("ATY").Valid)
	assert.False(t, oilClass("ACY").Valid)
	assert.Equal(t, "PTY", oilClass("PTY").String)
	for _, ty := range []string{"ATY", "ACY"} {
		assert.Equal(t, 0, mappings(ty, true), ty)
		assert.Equal(t, 0, mappings(ty, false), ty)
	}
	assert.Equal(t, 1, mappings("PTY", true))
	assert.Equal(t, 1, mappings("PTY", false))
	for _, c := range []string{"T577A1", "T577A2", "T577C1", "T577C2", "T577P1"} {
		assert.Equal(t, 0, cappCount(c), c)
	}
	for _, c := range []string{"T577A1", "T577A2"} {
		r := oil(c)
		assert.Equal(t, "CONING OIL", r.val.String, c)
		assert.Equal(t, "orig", r.filledBy.String, c)
	}
	assert.False(t, oil("T577C1").found, "inserted OIL_NAME row removed")
	assert.Equal(t, "MGTC 14", oil("T577P1").val.String)
	for _, tb := range []string{"bak_oil_name_000577", "bak_oil_group_map_000577", "bak_oil_class_000577"} {
		assert.False(t, tableExists(ctx, t, db, tb), tb)
	}

	// Down then Up again: same result.
	require.NoError(t, m.UpTo(ctx, oilClass577Ver))
	assertUp(t)
}
