package costcalc

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLoadTxWeightRulesQuery_JoinsThroughGroupMapping asserts the engine
// resolves rules via product type -> group mapping -> live group -> rules,
// and no longer via the pre-000536 direct type join.
func TestLoadTxWeightRulesQuery_JoinsThroughGroupMapping(t *testing.T) {
	q := loadTxWeightRulesQuery

	assert.Contains(t, q, "JOIN mst_yarn_tx_weight_group_type gt")
	assert.Contains(t, q, "gt.product_type_id = pm.cpm_product_type_id")
	assert.Contains(t, q, "JOIN mst_yarn_tx_weight_group g")
	assert.Contains(t, q, "g.ytwg_id = gt.ytwg_id")
	assert.Contains(t, q, "g.deleted_at IS NULL")
	assert.Contains(t, q, "tw.ytw_group_id = g.ytwg_id")
	assert.Contains(t, q, "tw.deleted_at IS NULL")
	assert.Contains(t, q, "pm.cpm_product_sys_id = ANY($1)")
	assert.NotContains(t, q, "ytw_product_type_id = pm")
}

// TestMigration000536_CreatesTxWeightGroups asserts the group schema, the
// one-group-per-type constraint, the 1:1 backfill and the index swap, and
// that down fans the group rules back out to per-type rows.
func TestMigration000536_CreatesTxWeightGroups(t *testing.T) {
	up, down := readMigrationPair(t, "000536_create_yarn_tx_weight_group")

	assert.Contains(t, up, "BEGIN;")
	assert.Contains(t, up, "COMMIT;")
	assert.Contains(t, up, "CREATE TABLE IF NOT EXISTS mst_yarn_tx_weight_group")
	assert.Contains(t, up, "CREATE TABLE IF NOT EXISTS mst_yarn_tx_weight_group_type")
	assert.Contains(t, up, "uq_mst_yarn_tx_weight_group_type_product_type UNIQUE (product_type_id)")
	assert.Contains(t, up, "uix_mst_yarn_tx_weight_group_code")
	assert.Contains(t, up, "ADD COLUMN IF NOT EXISTS ytw_group_id")
	assert.Contains(t, up, "ALTER COLUMN ytw_product_type_id DROP NOT NULL")
	assert.Contains(t, up, "ON CONFLICT (product_type_id) DO NOTHING")
	assert.Contains(t, up, "DROP INDEX IF EXISTS uix_mst_yarn_tx_weight_type_grade")
	assert.Contains(t, up, "CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_yarn_tx_weight_group_grade")
	assert.Contains(t, up, "RAISE NOTICE")
	assert.Contains(t, up, "'seed_000536'")

	assert.Contains(t, down, "information_schema")
	assert.Contains(t, down, "'rollback_000536'")
	assert.Contains(t, down, "uix_mst_yarn_tx_weight_type_grade")
	assert.Contains(t, down, "SET NOT NULL")
	assert.Contains(t, down, "DROP COLUMN IF EXISTS ytw_group_id")
	assert.Contains(t, down, "DROP TABLE IF EXISTS mst_yarn_tx_weight_group_type")
	assert.Contains(t, down, "DROP TABLE IF EXISTS mst_yarn_tx_weight_group")
	// Junction table must be dropped before the group table it references.
	assert.Less(t,
		strings.Index(down, "DROP TABLE IF EXISTS mst_yarn_tx_weight_group_type"),
		strings.Index(down, "DROP TABLE IF EXISTS mst_yarn_tx_weight_group;"))
}

// TestMigration000537_SharesTTYAndDTYConfigs asserts every type the user
// listed is mapped to the anchor group resolved by code (never literal ids),
// conflicts are skipped, and down removes only its own rows.
func TestMigration000537_SharesTTYAndDTYConfigs(t *testing.T) {
	up, down := readMigrationPair(t, "000537_seed_yarn_tx_weight_group_shared_types")

	ttyShared := []string{"TTS", "TTM", "TTH", "TPY", "TPS", "TPM", "TFY", "TCY", "TCS", "TCM", "TCH", "PTS", "ATT"}
	for _, code := range ttyShared {
		assert.Contains(t, up, "('TTY', '"+code+"')", "TTY share missing %s", code)
	}
	dtyShared := []string{"PTY", "ATY", "PLY", "MEL"}
	for _, code := range dtyShared {
		assert.Contains(t, up, "('DTY', '"+code+"')", "DTY share missing %s", code)
	}
	pairs := regexp.MustCompile(`\('(TTY|DTY)', '[A-Z0-9]+'\)`).FindAllString(up, -1)
	assert.Len(t, pairs, len(ttyShared)+len(dtyShared), "unexpected extra shared type")

	assert.Contains(t, up, "BEGIN;")
	assert.Contains(t, up, "COMMIT;")
	assert.Contains(t, up, "ON CONFLICT (product_type_id) DO NOTHING")
	assert.Contains(t, up, "g.deleted_at IS NULL")
	assert.Contains(t, up, "RAISE NOTICE")
	assert.Contains(t, up, "'seed_000537'")
	assert.NotRegexp(t, `ytwg_id\s*=\s*'[0-9a-f-]{36}'`, up, "group must be resolved by anchor code, not literal id")
	assert.NotRegexp(t, `cpt_type_id\s*(=|IN)\s*\(?\d`, up, "types must be resolved by code, not literal id")

	assert.Contains(t, down, "to_regclass")
	assert.Contains(t, down, "created_by = 'seed_000537'")
	assert.NotContains(t, down, "DROP ")
}
