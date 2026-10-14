// Migration round-trip tests for 000571 (RM group denier_use_draft) and 000572
// (melange product type backfill). Local PostgreSQL only, via
// internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
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
	preDenierDraft uint64 = 570
	melangeVer     uint64 = 572
)

func TestDenierDraftAndMelangeMigrations(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "denier_melange_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), melangeVer)
	require.NoError(t, m.UpTo(ctx, preDenierDraft))

	// Fixtures at 000570: spandex + POY groups, two oil groups mapped to PTY,
	// melange products (PTY / TCY / ATY legacy ids from the list) and one PTY
	// product outside the list.
	mustExec(ctx, t, db, `INSERT INTO cst_rm_group_head (group_code, group_name, created_by) VALUES
		('T571SPD', 'SPD0000001', 't'), ('T571POY', 'POY 80', 't')`)
	mustExec(ctx, t, db, `INSERT INTO cst_rm_group_head (group_code, group_name, is_oil_group, created_by) VALUES
		('T572OIL', 'CONING OIL', TRUE, 't'), ('T572OI2', 'OIL 2', TRUE, 't')`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_type_oil_group (cptog_type_id, cptog_group_head_id, cptog_is_default, cptog_created_by)
		SELECT pt.cpt_type_id, g.group_head_id, g.group_code = 'T572OIL', 't'
		FROM cost_product_type pt, cst_rm_group_head g
		WHERE pt.cpt_type_code = 'PTY' AND g.group_code IN ('T572OIL', 'T572OI2')
		ON CONFLICT ON CONSTRAINT uk_cptog_type_group DO NOTHING`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by, cpm_flex_02)
		SELECT v.c, (SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = v.t), 't572', 't572', 't572', v.f
		FROM (VALUES ('T572P', 'PTY', '8015'), ('T572T', 'TCY', '6972'), ('T572A', 'ATY', '10051'), ('T572X', 'PTY', '999999999')) v(c, t, f)`)

	typeOf := func(code string) string {
		var tc string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT pt.cpt_type_code FROM cost_product_master pm
			JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id WHERE pm.cpm_product_code = $1`, code).Scan(&tc))
		return tc
	}
	melClass := func() sql.NullString {
		var c sql.NullString
		require.NoError(t, db.QueryRowContext(ctx, `SELECT cpt_oil_class FROM cost_product_type WHERE cpt_type_code = 'MEL'`).Scan(&c))
		return c
	}
	assertUp := func(t *testing.T) {
		t.Helper()
		assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM cst_rm_group_head WHERE group_code = 'T571SPD' AND denier_use_draft`))
		assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM cst_rm_group_head WHERE group_code = 'T571POY' AND NOT denier_use_draft`))
		for _, c := range []string{"T572P", "T572T", "T572A"} {
			assert.Equal(t, "MEL", typeOf(c), c)
		}
		assert.Equal(t, "PTY", typeOf("T572X"), "product outside the list untouched")
		assert.Equal(t, sql.NullString{String: "PTY", Valid: true}, melClass())
		assert.Equal(t, 2, count(ctx, t, db, `SELECT count(*) FROM cost_product_type_oil_group o
			JOIN cost_product_type pt ON pt.cpt_type_id = o.cptog_type_id WHERE pt.cpt_type_code = 'MEL'`))
		assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM cost_product_type_oil_group o
			JOIN cost_product_type pt ON pt.cpt_type_id = o.cptog_type_id
			JOIN cst_rm_group_head g ON g.group_head_id = o.cptog_group_head_id
			WHERE pt.cpt_type_code = 'MEL' AND o.cptog_is_default AND g.group_code = 'T572OIL'`))
		assert.Equal(t, 3, count(ctx, t, db, `SELECT count(DISTINCT c.capp_product_sys_id) FROM cost_product_applicable_param c
			JOIN mst_parameter mp ON mp.id = c.capp_param_id AND mp.param_code IN ('OIL_NAME', 'OPU')
			JOIN cost_product_master pm ON pm.cpm_product_sys_id = c.capp_product_sys_id
			WHERE pm.cpm_product_code IN ('T572P', 'T572T', 'T572A')`), "moved products carry oil CAPP rows")
	}

	require.NoError(t, m.UpTo(ctx, melangeVer))
	assertUp(t)

	require.NoError(t, m.DownTo(ctx, preDenierDraft))
	assert.Equal(t, 0, count(ctx, t, db, `SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'cst_rm_group_head' AND column_name = 'denier_use_draft'`))
	assert.Equal(t, "PTY", typeOf("T572P"))
	assert.Equal(t, "TCY", typeOf("T572T"))
	assert.Equal(t, "ATY", typeOf("T572A"))
	assert.False(t, melClass().Valid, "MEL oil class reset by down")
	assert.Equal(t, 0, count(ctx, t, db, `SELECT count(*) FROM cost_product_type_oil_group WHERE cptog_created_by = 'migration_000572'`))
	assert.Equal(t, 0, count(ctx, t, db, `SELECT count(*) FROM cost_product_applicable_param WHERE capp_created_by = 'migration_000572'`))
	assert.False(t, tableExists(ctx, t, db, "cost_product_type_backfill_572"))

	require.NoError(t, m.UpTo(ctx, melangeVer))
	assertUp(t)
}
