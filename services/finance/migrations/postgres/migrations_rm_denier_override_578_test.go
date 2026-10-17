// Migration round-trip test for 000578 (per-product RM denier override table
// and cost_product_master.cpm_duplicated_from_sys_id). Local PostgreSQL only,
// via internal/testutil/pgcontainer. Skipped unless INTEGRATION_TEST=true.
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
	preRMDenierOverride578 uint64 = 577
	rmDenierOverride578Ver uint64 = 578
)

func TestRMDenierOverride578Migration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	db := srv.CreateDatabase(t, "rm_denier_override_578_roundtrip", "")
	m, err := pgcontainer.NewMigrator(ctx, db, migDir)
	require.NoError(t, err)
	require.GreaterOrEqual(t, m.Latest(), rmDenierOverride578Ver)
	require.NoError(t, m.UpTo(ctx, preRMDenierOverride578))

	mustExec(ctx, t, db, `INSERT INTO cost_product_type (cpt_type_code, cpt_type_name)
		VALUES ('ACY', 'ACY') ON CONFLICT (cpt_type_code) DO NOTHING`)
	mustExec(ctx, t, db, `INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
		SELECT v.c, (SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'ACY'), 't578', 't578', 't578'
		FROM (VALUES ('T578A'), ('T578B'), ('T578R')) v(c)`)
	productID := func(code string) string {
		var id int64
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code = $1`, code).Scan(&id))
		return itoa(id)
	}
	a, b, r := productID("T578A"), productID("T578B"), productID("T578R")
	columnExists := func() bool {
		var ok bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'cost_product_master' AND column_name = 'cpm_duplicated_from_sys_id')`).Scan(&ok))
		return ok
	}

	ins := `INSERT INTO cost_product_rm_denier_override
		(cpdo_product_sys_id, cpdo_rm_type, cpdo_rm_product_sys_id, cpdo_rm_group_code, cpdo_denier, cpdo_reason, cpdo_created_by, cpdo_updated_by) VALUES `

	assertUp := func(t *testing.T) {
		t.Helper()
		assert.True(t, tableExists(ctx, t, db, "cost_product_rm_denier_override"))
		assert.True(t, columnExists())
	}

	require.NoError(t, m.UpTo(ctx, rmDenierOverride578Ver))
	assertUp(t)

	// Valid rows: one PRODUCT and one GROUP override for A.
	mustExec(ctx, t, db, ins+`(`+a+`, 'PRODUCT', `+r+`, NULL, 75, 'special yarn', 't', 't')`)
	mustExec(ctx, t, db, ins+`(`+a+`, 'GROUP', NULL, 'G578', 20, 'draft final', 't', 't')`)
	// Same RM on another product is fine.
	mustExec(ctx, t, db, ins+`(`+b+`, 'PRODUCT', `+r+`, NULL, 70, 'other', 't', 't')`)

	// Constraints.
	assertExecFails(ctx, t, db, ins+`(`+b+`, 'GROUP', NULL, 'G578', 0, 'zero', 't', 't')`, "chk_cpdo_denier_positive")
	assertExecFails(ctx, t, db, ins+`(`+b+`, 'GROUP', NULL, 'G578', -1, 'neg', 't', 't')`, "chk_cpdo_denier_positive")
	assertExecFails(ctx, t, db, ins+`(`+b+`, 'GROUP', NULL, 'G578', 10, '   ', 't', 't')`, "chk_cpdo_reason_not_blank")
	assertExecFails(ctx, t, db, ins+`(`+b+`, 'ITEM', NULL, 'G578', 10, 'x', 't', 't')`, "check constraint") // type or identity check
	assertExecFails(ctx, t, db, ins+`(`+b+`, 'PRODUCT', `+r+`, 'G578', 10, 'both', 't', 't')`, "chk_cpdo_rm_identity")
	assertExecFails(ctx, t, db, ins+`(`+b+`, 'PRODUCT', NULL, NULL, 10, 'none', 't', 't')`, "chk_cpdo_rm_identity")
	assertExecFails(ctx, t, db, ins+`(`+b+`, 'GROUP', NULL, NULL, 10, 'none', 't', 't')`, "chk_cpdo_rm_identity")
	assertExecFails(ctx, t, db, ins+`(`+b+`, 'GROUP', `+r+`, 'G578', 10, 'both', 't', 't')`, "chk_cpdo_rm_identity")
	// Uniqueness per (product, RM identity).
	assertExecFails(ctx, t, db, ins+`(`+a+`, 'PRODUCT', `+r+`, NULL, 80, 'dup', 't', 't')`, "uk_cpdo_product_rm_product")
	assertExecFails(ctx, t, db, ins+`(`+a+`, 'GROUP', NULL, 'G578', 21, 'dup', 't', 't')`, "uk_cpdo_product_rm_group")

	// Duplicate link: ON DELETE SET NULL.
	mustExec(ctx, t, db, `UPDATE cost_product_master SET cpm_duplicated_from_sys_id = `+a+` WHERE cpm_product_sys_id = `+b)
	assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM cost_product_master WHERE cpm_duplicated_from_sys_id = `+a))

	// Idempotent re-run of the up SQL itself.
	sqlUp, err := os.ReadFile("000578_rm_denier_override.up.sql")
	require.NoError(t, err)
	mustExec(ctx, t, db, string(sqlUp))
	assertUp(t)
	assert.Equal(t, 3, count(ctx, t, db, `SELECT count(*) FROM cost_product_rm_denier_override`))

	// Deleting an RM product cascades its override rows; deleting the parent
	// clears the link on the copy and cascades the parent's own overrides.
	mustExec(ctx, t, db, `DELETE FROM cost_product_rm_denier_override WHERE cpdo_product_sys_id = `+b)
	mustExec(ctx, t, db, `DELETE FROM cost_product_master WHERE cpm_product_sys_id = `+r)
	assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM cost_product_rm_denier_override`))
	mustExec(ctx, t, db, `DELETE FROM cost_product_master WHERE cpm_product_sys_id = `+a)
	assert.Equal(t, 0, count(ctx, t, db, `SELECT count(*) FROM cost_product_rm_denier_override`))
	var link sql.NullInt64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT cpm_duplicated_from_sys_id FROM cost_product_master WHERE cpm_product_sys_id = `+b).Scan(&link))
	assert.False(t, link.Valid, "link set NULL when the source is deleted")

	require.NoError(t, m.DownTo(ctx, preRMDenierOverride578))
	assert.False(t, tableExists(ctx, t, db, "cost_product_rm_denier_override"))
	assert.False(t, columnExists())
	assert.Equal(t, 1, count(ctx, t, db, `SELECT count(*) FROM cost_product_master WHERE cpm_product_code = 'T578B'`))

	// Down then Up again: same result.
	require.NoError(t, m.UpTo(ctx, rmDenierOverride578Ver))
	assertUp(t)
}
