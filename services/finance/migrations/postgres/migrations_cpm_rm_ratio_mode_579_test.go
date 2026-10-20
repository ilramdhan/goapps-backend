// Migration round-trip test for 000579 (cost_product_master.cpm_rm_ratio_mode,
// per-product RM ratio mode). Local PostgreSQL only, via
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
	preCPMRMRatioMode579 uint64 = 578
	cpmRMRatioMode579Ver uint64 = 579
)

func TestCPMRMRatioMode579Migration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "cpm_rm_ratio_mode_579_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), cpmRMRatioMode579Ver)
	require.NoError(t, m.UpTo(ctx, preCPMRMRatioMode579))

	mustExec(ctx, t, db, `INSERT INTO cost_product_type (cpt_type_code, cpt_type_name)
		VALUES ('ACY', 'ACY') ON CONFLICT (cpt_type_code) DO NOTHING`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
		SELECT v.c, (SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'ACY'), 't579', 't579', 't579'
		FROM (VALUES ('T579A'), ('T579B')) v(c)`)

	columnExists := func() bool {
		var ok bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'cost_product_master' AND column_name = 'cpm_rm_ratio_mode')`).Scan(&ok))
		return ok
	}
	constraintExists := func() bool {
		return count(ctx, t, db, `SELECT count(*) FROM pg_constraint WHERE conname = 'chk_cpm_rm_ratio_mode'`) == 1
	}
	assertUp := func(t *testing.T) {
		t.Helper()
		assert.True(t, columnExists())
		assert.True(t, constraintExists())
	}

	require.NoError(t, m.UpTo(ctx, cpmRMRatioMode579Ver))
	assertUp(t)

	// Existing rows stay NULL (follow product type).
	assert.Equal(t, 2, count(ctx, t, db, `SELECT count(*) FROM cost_product_master
		WHERE cpm_product_code IN ('T579A','T579B') AND cpm_rm_ratio_mode IS NULL`))

	// Valid values.
	mustExec(ctx, t, db, `UPDATE cost_product_master SET cpm_rm_ratio_mode = 'DENIER' WHERE cpm_product_code = 'T579A'`)
	mustExec(ctx, t, db, `UPDATE cost_product_master SET cpm_rm_ratio_mode = 'INPUT' WHERE cpm_product_code = 'T579B'`)
	mustExec(ctx, t, db, `UPDATE cost_product_master SET cpm_rm_ratio_mode = NULL WHERE cpm_product_code = 'T579B'`)
	// Invalid values.
	assertExecFails(ctx, t, db, `UPDATE cost_product_master SET cpm_rm_ratio_mode = 'X' WHERE cpm_product_code = 'T579B'`, "chk_cpm_rm_ratio_mode")
	assertExecFails(ctx, t, db, `UPDATE cost_product_master SET cpm_rm_ratio_mode = 'denier' WHERE cpm_product_code = 'T579B'`, "chk_cpm_rm_ratio_mode")

	// Idempotent re-run of the up SQL itself; data preserved.
	sqlUp, err := os.ReadFile("000579_cpm_rm_ratio_mode.up.sql")
	require.NoError(t, err)
	mustExec(ctx, t, db, string(sqlUp))
	assertUp(t)
	var mode sql.NullString
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT cpm_rm_ratio_mode FROM cost_product_master WHERE cpm_product_code = 'T579A'`).Scan(&mode))
	assert.Equal(t, "DENIER", mode.String)

	require.NoError(t, m.DownTo(ctx, preCPMRMRatioMode579))
	assert.False(t, columnExists())
	assert.False(t, constraintExists())
	assert.Equal(t, 2, count(ctx, t, db, `SELECT count(*) FROM cost_product_master WHERE cpm_product_code IN ('T579A','T579B')`))

	// Down then Up again: same result.
	require.NoError(t, m.UpTo(ctx, cpmRMRatioMode579Ver))
	assertUp(t)
}
