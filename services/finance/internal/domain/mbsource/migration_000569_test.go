package mbsource_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
)

func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

// TestMigration000569_SharesRuleWithGo pins that the SQL backfill uses the SAME spin ordering
// (mbsource.SpinPickOrderSQL) as the runtime provider, plus the marker, backup and NOTICE contract.
func TestMigration000569_SharesRuleWithGo(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "migrations", "postgres")
	up, err := os.ReadFile(filepath.Join(dir, "000569_backfill_mb_source_from_shade.up.sql"))
	require.NoError(t, err)
	down, err := os.ReadFile(filepath.Join(dir, "000569_backfill_mb_source_from_shade.down.sql"))
	require.NoError(t, err)

	assert.Contains(t, squash(string(up)), squash(mbsource.SpinPickOrderSQL),
		"migration 000569 ORDER BY must equal mbsource.SpinPickOrderSQL")
	for _, frag := range []string{
		"backfill_mb_source_000569", "bak_000569_values", "bak_000569_capp",
		"RAISE NOTICE", "UPPER(TRIM(", "NOT m.cpm_is_locked", "<> 'MB'",
		"ORDER BY UPPER(TRIM(shade_code)), legacy_sys_id DESC",
	} {
		assert.Contains(t, string(up), frag)
	}
	assert.Contains(t, string(up), "COALESCE(x.cpp_value_text, '') <> ''", "never overwrite non-empty cells")
	assert.NotContains(t, strings.ToUpper(string(up)), "DROP TABLE mst_")
	assert.Contains(t, string(down), "DROP TABLE IF EXISTS bak_000569_values")
	assert.Contains(t, string(down), "backfill_mb_source_000569")
}
