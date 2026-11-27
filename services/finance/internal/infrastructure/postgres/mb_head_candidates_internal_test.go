package postgres

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Pins the shape of the candidate queries (no SQL mock in this package; behaviour is covered by
// the integration test).
func TestCandidateSQLShape(t *testing.T) {
	assert.Contains(t, listValidatedActiveSQL, "mbh_is_active = TRUE", "push-to-head: active only")
	assert.Contains(t, listValidatedActiveSQL, "deleted_at IS NULL")
	q := listBatchCandidatesSQL
	assert.Contains(t, q, "WITH RECURSIVE")
	assert.Contains(t, q, "\n\t\t  UNION\n", "UNION (not ALL) keeps the recursion cycle-safe")
	assert.False(t, strings.Contains(q, "UNION ALL"))
	assert.Contains(t, q, "v.mbcv_version = p.mbh_current_version", "current composition only")
	assert.Contains(t, q, "mbcv_source_type = 'MB'")
	assert.Contains(t, q, "ref.deleted_at IS NULL")
}
