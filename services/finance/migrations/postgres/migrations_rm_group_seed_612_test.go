package postgres_test

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Text-level guard for seed 000612 (no DB needed).
func TestRmGroupSeed612MigrationText(t *testing.T) {
	up, err := os.ReadFile("000612_seed_rm_group_espet_mb_07.up.sql")
	require.NoError(t, err)
	down, err := os.ReadFile("000612_seed_rm_group_espet_mb_07.down.sql")
	require.NoError(t, err)
	sql := string(up)

	codes := []string{"202611954"}
	head := regexp.MustCompile(`(?m)^\('(\d{9})', '[^']+', '[^']+ \[legacy_code=[^\]]+\]', TRUE\),?;?$`)
	item := regexp.MustCompile(`(?m)^\('(\d{9})', '', '(NA|NL)', TRUE, FALSE\),?;?$`)
	heads := head.FindAllStringSubmatch(sql, -1)
	items := item.FindAllStringSubmatch(sql, -1)
	require.Len(t, heads, len(codes), "1 head tuple")
	require.Len(t, items, len(codes), "1 dummy item tuple")
	for i, c := range codes {
		assert.Equal(t, c, heads[i][1])
		assert.Equal(t, c, items[i][1])
	}

	// Additive + scoped to this seed's own marker.
	assert.Contains(t, sql, "ON CONFLICT (group_code) WHERE deleted_at IS NULL DO NOTHING")
	assert.Contains(t, sql, "h.created_by = 'seed_000612'")
	// Temp staging tables use ON COMMIT DROP; any other destructive verb is forbidden.
	stmts := regexp.MustCompile(`(?i)ON COMMIT DROP`).ReplaceAllString(sql, "")
	assert.NotRegexp(t, `(?i)\b(DELETE|DROP|TRUNCATE|UPDATE)\b`, stmts)
	assert.Contains(t, string(down), "WHERE created_by = 'seed_000612'")
}
