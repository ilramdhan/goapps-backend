// Migration round-trip test for 000580 (seed legacy RM denier overrides,
// SPANDEX_DRAFT and per-product RM ratio mode). Local PostgreSQL only, via
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
	preSeedRMDenier580 uint64 = 579
	seedRMDenier580Ver uint64 = 580
	seed580Marker             = "migration_000580"
)

func TestSeedRMDenierOverride580Migration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "seed_rm_denier_override_580_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), seedRMDenier580Ver)
	require.NoError(t, m.UpTo(ctx, preSeedRMDenier580))

	// Fixture: a subset of the seed codes plus an unrelated product.
	mustExec(ctx, t, db, `INSERT INTO cost_product_type (cpt_type_code, cpt_type_name)
		SELECT v.c, v.c FROM (VALUES ('ACY'), ('ATY'), ('POY'), ('PTY')) v(c)
		ON CONFLICT (cpt_type_code) DO NOTHING`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
		SELECT v.c, (SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = v.t), 't580', 't580', 't580'
		FROM (VALUES
			('CSTACY2606000042', 'ACY'), -- seeded override POY 300
			('CSTPOY2606002092', 'POY'),
			('CSTACY2606000010', 'ACY'), -- manual override must survive; SPANDEX_DRAFT 3 -> 3.3
			('CSTPTY2606003800', 'PTY'),
			('CSTACY2606000011', 'ACY'), -- SPANDEX_DRAFT manually edited to 4: unchanged
			('CSTATY2606000030', 'ATY'), -- mode NULL -> DENIER
			('CSTPOY2606003243', 'POY'),
			('CSTATY2606000055', 'ATY'), -- mode already INPUT: unchanged
			('CSTATY2606000068', 'ATY'), -- mode NULL -> DENIER, then edited back to INPUT
			('T580X', 'ACY')             -- unrelated
		) v(c, t)`)
	productID := func(code string) string {
		var id int64
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code = $1`, code).Scan(&id))
		return itoa(id)
	}
	mustExec(ctx, t, db, `INSERT INTO cost_product_rm_denier_override
		(cpdo_product_sys_id, cpdo_rm_type, cpdo_rm_product_sys_id, cpdo_denier, cpdo_reason, cpdo_created_by, cpdo_updated_by)
		VALUES (`+productID("CSTACY2606000010")+`, 'PRODUCT', `+productID("CSTPTY2606003800")+`, 999, 'manual', 'u1', 'u1')`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
		SELECT m.cpm_product_sys_id, p.id, v.val, 't580', 't580'
		FROM (VALUES ('CSTACY2606000010', 3), ('CSTACY2606000011', 4), ('T580X', 3)) v(c, val)
		JOIN cost_product_master m ON m.cpm_product_code = v.c
		JOIN mst_parameter p ON p.param_code = 'SPANDEX_DRAFT' AND p.deleted_at IS NULL
		ON CONFLICT ON CONSTRAINT cpp_unique_product_param DO UPDATE SET cpp_value_numeric = EXCLUDED.cpp_value_numeric`)
	mustExec(ctx, t, db, `UPDATE cost_product_master SET cpm_rm_ratio_mode = 'INPUT' WHERE cpm_product_code = 'CSTATY2606000055'`)

	draft := func(code string) float64 {
		var v float64
		require.NoError(t, db.QueryRowContext(ctx, `SELECT c.cpp_value_numeric FROM cost_product_parameter c
			JOIN cost_product_master m ON m.cpm_product_sys_id = c.cpp_product_sys_id
			JOIN mst_parameter p ON p.id = c.cpp_param_id AND p.param_code = 'SPANDEX_DRAFT' AND p.deleted_at IS NULL
			WHERE m.cpm_product_code = $1`, code).Scan(&v))
		return v
	}
	mode := func(code string) sql.NullString {
		var v sql.NullString
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT cpm_rm_ratio_mode FROM cost_product_master WHERE cpm_product_code = $1`, code).Scan(&v))
		return v
	}
	overrideDenier := func(owner, rm string) (float64, string) {
		var d float64
		var by string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT o.cpdo_denier, o.cpdo_created_by
			FROM cost_product_rm_denier_override o
			JOIN cost_product_master a ON a.cpm_product_sys_id = o.cpdo_product_sys_id
			JOIN cost_product_master r ON r.cpm_product_sys_id = o.cpdo_rm_product_sys_id
			WHERE a.cpm_product_code = $1 AND r.cpm_product_code = $2`, owner, rm).Scan(&d, &by))
		return d, by
	}
	markerRows := func() int {
		return count(ctx, t, db, `SELECT count(*) FROM cost_product_rm_denier_override WHERE cpdo_created_by = '`+seed580Marker+`'`)
	}
	assertSeeded := func(t *testing.T) {
		t.Helper()
		assert.Equal(t, 2, markerRows(), "only overrides whose owner and RM resolve")
		d, by := overrideDenier("CSTACY2606000042", "CSTPOY2606002092")
		assert.InDelta(t, 300, d, 1e-9)
		assert.Equal(t, seed580Marker, by)
		var reason string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT cpdo_reason FROM cost_product_rm_denier_override o
			JOIN cost_product_master a ON a.cpm_product_sys_id = o.cpdo_product_sys_id
			WHERE a.cpm_product_code = 'CSTACY2606000042'`).Scan(&reason))
		assert.Equal(t, "Seed rev denier legacy TOP 55 (Orion VAL) ACY0000098 / POY0000299", reason)
		d, by = overrideDenier("CSTACY2606000010", "CSTPTY2606003800")
		assert.InDelta(t, 999, d, 1e-9, "manual override not overwritten")
		assert.Equal(t, "u1", by)
		d, _ = overrideDenier("CSTATY2606000030", "CSTPOY2606003243")
		assert.InDelta(t, 300, d, 1e-9)

		assert.InDelta(t, 3.3, draft("CSTACY2606000010"), 1e-9)
		assert.InDelta(t, 4, draft("CSTACY2606000011"), 1e-9, "manual edit kept")
		assert.InDelta(t, 3, draft("T580X"), 1e-9)

		assert.Equal(t, "DENIER", mode("CSTATY2606000030").String)
		assert.Equal(t, "INPUT", mode("CSTATY2606000055").String, "explicit mode kept")
		assert.False(t, mode("T580X").Valid)
		assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM bak_000580_spandex_draft`))
		assert.Equal(t, 2, count(ctx, t, db, `SELECT count(*) FROM bak_000580_rm_ratio_mode`))
	}

	require.NoError(t, m.UpTo(ctx, seedRMDenier580Ver))
	assert.Equal(t, "DENIER", mode("CSTATY2606000068").String)
	assertSeeded(t)

	// Idempotent re-run of the up SQL itself: no-op.
	sqlUp, err := os.ReadFile("000580_seed_rm_denier_override_legacy.up.sql")
	require.NoError(t, err)
	mustExec(ctx, t, db, string(sqlUp))
	assertSeeded(t)
	assert.Equal(t, 3, count(ctx, t, db, `SELECT count(*) FROM cost_product_rm_denier_override`))

	// A later manual edit of a seeded mode must survive the down migration.
	mustExec(ctx, t, db, `UPDATE cost_product_master SET cpm_rm_ratio_mode = 'INPUT' WHERE cpm_product_code = 'CSTATY2606000068'`)

	require.NoError(t, m.DownTo(ctx, preSeedRMDenier580))
	assert.Equal(t, 0, markerRows())
	d, by := overrideDenier("CSTACY2606000010", "CSTPTY2606003800")
	assert.InDelta(t, 999, d, 1e-9)
	assert.Equal(t, "u1", by)
	assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM cost_product_rm_denier_override`))
	assert.InDelta(t, 3, draft("CSTACY2606000010"), 1e-9, "restored")
	assert.InDelta(t, 4, draft("CSTACY2606000011"), 1e-9)
	assert.InDelta(t, 3, draft("T580X"), 1e-9)
	assert.False(t, mode("CSTATY2606000030").Valid, "restored to NULL")
	assert.Equal(t, "INPUT", mode("CSTATY2606000055").String)
	assert.Equal(t, "INPUT", mode("CSTATY2606000068").String, "manual edit after seed kept")
	assert.False(t, mode("T580X").Valid)
	assert.False(t, tableExists(ctx, t, db, "bak_000580_spandex_draft"))
	assert.False(t, tableExists(ctx, t, db, "bak_000580_rm_ratio_mode"))

	// Down then Up again: re-seeds.
	mustExec(ctx, t, db, `UPDATE cost_product_master SET cpm_rm_ratio_mode = NULL WHERE cpm_product_code = 'CSTATY2606000068'`)
	require.NoError(t, m.UpTo(ctx, seedRMDenier580Ver))
	assertSeeded(t)
}
