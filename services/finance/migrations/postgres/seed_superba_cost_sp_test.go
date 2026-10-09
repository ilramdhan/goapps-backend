package postgres_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Text-level guard for the generated seed 000566 (no DB needed).
func TestSeedSuperbaCostSpMigrationText(t *testing.T) {
	raw, err := os.ReadFile("000566_seed_cost_superba_cost_sp.up.sql")
	require.NoError(t, err)
	sql := string(raw)

	tuple := regexp.MustCompile(`(?m)^\s+\((\d+), '((?:[^']|'')*)', (NULL|'(?:[^']|'')*'), (\d+(?:\.\d+)?), (NULL|\d+(?:\.\d+)?)\),?$`)
	matches := tuple.FindAllStringSubmatch(sql, -1)
	assert.Len(t, matches, 644, "exactly 644 value tuples")

	ids := map[string]bool{}
	for _, m := range matches {
		assert.False(t, ids[m[1]], "duplicate legacy sys id %s", m[1])
		ids[m[1]] = true
		assert.Equal(t, strings.TrimSpace(m[2]), m[2], "shade must be trimmed")
	}

	// Spot checks.
	assert.Contains(t, sql, "(20241101895, 'MC-0547', 'MOSCOW MONO BN SD-60535', 0.08, NULL)")
	assert.Contains(t, sql, "–70179-30641", "row 201 en dash must be proper UTF-8")
	for _, r := range sql {
		assert.False(t, r >= 0x7f && r <= 0x9f, "C1 control char in seed")
	}

	// Safety: only seed-owned rows are overwritten on conflict.
	assert.Contains(t, sql, "ON CONFLICT (legacy_sys_id) DO UPDATE")
	assert.Contains(t, sql, "WHERE t.source = 'SEED'")
	assert.Contains(t, sql, "'SEED', 'seed_000566'")
}
