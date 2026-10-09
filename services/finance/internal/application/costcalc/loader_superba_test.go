package costcalc

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadSuperbaCost_Integration runs loadSuperbaCostQuery against a migrated
// DB (>= 000565): case/space-insensitive shade match, duplicate shade -> max
// legacy_sys_id, unmatched/empty shade -> Found=false, non-SUPERBA absent.
func TestLoadSuperbaCost_Integration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		envOr("TEST_DB_HOST", "localhost"), envOr("TEST_DB_PORT", "5434"),
		envOr("TEST_DB_USER", "finance"), envOr("TEST_DB_PASSWORD", "finance123"),
		envOr("TEST_DB_NAME", "finance_db"))
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, waitDB(db, 10*time.Second))
	t.Cleanup(func() { _ = db.Close() })

	const actor = "loader-superba-test"
	prefix := uniqueCodePrefix(t, "S")
	shade := "ZZ" + prefix
	base := time.Now().UnixNano() % 1_000_000_000_000

	var sbType, otherType int
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO cost_product_type (cpt_type_code, cpt_type_name, cpt_oil_class)
		VALUES ($1, 'loader superba test', 'SUPERBA') RETURNING cpt_type_id`, prefix[:5]).Scan(&sbType))
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT cpt_type_id FROM cost_product_type WHERE cpt_oil_class IS NULL ORDER BY cpt_type_id LIMIT 1`).Scan(&otherType))

	mk := func(suffix string, typeID int, sh any) int64 {
		var id int64
		require.NoError(t, db.QueryRowContext(ctx, `
			INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_shade_code, cpm_created_by, cpm_updated_by)
			VALUES ($1, $2, 'loader superba test', $3, $4, $4) RETURNING cpm_product_sys_id`,
			prefix+suffix, typeID, sh, actor).Scan(&id))
		return id
	}
	pDup := mk("-A", sbType, "  "+shade+"  ")
	pMiss := mk("-B", sbType, shade+"NOPE")
	pEmpty := mk("-C", sbType, nil)
	pOther := mk("-D", otherType, shade)

	ins := func(off int64, sh string, old float64, active bool) {
		_, e := db.ExecContext(ctx, `
			INSERT INTO cost_superba_cost_sp (legacy_sys_id, shade_code, colour_name, old_value, source, is_active, created_by)
			VALUES ($1, $2, $3, $4, 'MANUAL', $5, $6)`, base+off, sh, "C"+sh, old, active, actor)
		require.NoError(t, e)
	}
	ins(1, shade, 0.1, true)
	ins(3, " "+strings.ToLower(shade)+" ", 0.3, true) // lower-case variant, max id, must win
	ins(2, shade, 0.2, true)
	ins(9, shade, 0.9, false) // inactive, ignored

	t.Cleanup(func() {
		for _, id := range []int64{pDup, pMiss, pEmpty, pOther} {
			_, _ = db.ExecContext(ctx, `DELETE FROM cost_product_master WHERE cpm_product_sys_id = $1`, id)
		}
		_, _ = db.ExecContext(ctx, `DELETE FROM cost_product_type WHERE cpt_type_id = $1`, sbType)
		_, _ = db.ExecContext(ctx, `DELETE FROM cost_superba_cost_sp WHERE created_by = $1`, actor)
	})

	got, err := NewProductLoader(db).LoadSuperbaCost(ctx, []int64{pDup, pMiss, pEmpty, pOther})
	require.NoError(t, err)
	require.Contains(t, got, pDup)
	assert.True(t, got[pDup].Found)
	assert.InDelta(t, 0.3, got[pDup].OldValue, 1e-9, "duplicate shade -> max legacy_sys_id, case/space-insensitive")
	assert.Equal(t, base+3, got[pDup].LegacySysID)
	assert.Equal(t, shade, got[pDup].ShadeCode, "shade trimmed")
	require.Contains(t, got, pMiss)
	assert.False(t, got[pMiss].Found)
	require.Contains(t, got, pEmpty)
	assert.False(t, got[pEmpty].Found)
	assert.NotContains(t, got, pOther, "non-SUPERBA products are never looked up")
}
