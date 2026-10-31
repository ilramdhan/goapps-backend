// Text-level guard for IAM 000099 (no DB needed): MB_ADMIN never receives the
// SUPER_ADMIN-only unrevoke/bulk permissions, SUPER_ADMIN grants are never revoked,
// and down undoes the role.
package postgres_test

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMBRoles099(t *testing.T) {
	up, err := os.ReadFile("000099_restructure_mb_roles.up.sql")
	require.NoError(t, err)
	down, err := os.ReadFile("000099_restructure_mb_roles.down.sql")
	require.NoError(t, err)

	// Strip comments so the header's "⛔ not given" notes don't count.
	stripComments := regexp.MustCompile(`(?m)--.*$`)
	upSQL := stripComments.ReplaceAllString(string(up), "")
	downSQL := stripComments.ReplaceAllString(string(down), "")

	assert.Contains(t, upSQL, "'MB_ADMIN'")
	for _, code := range []string{
		"finance.mb.head.unrevoke",
		"finance.mb.head.bulkunvalidate",
		"finance.mb.head.bulksubmit",
		"finance.mb.head.bulkvalidate",
	} {
		assert.NotContains(t, upSQL, code, "SUPER_ADMIN-only permission leaked into 000099")
	}

	assert.NotContains(t, upSQL, "SUPER_ADMIN", "000099 must not touch SUPER_ADMIN")
	assert.NotContains(t, downSQL, "SUPER_ADMIN", "000099 down must not touch SUPER_ADMIN")
	assert.NotContains(t, upSQL, "DELETE FROM mst_permission", "000099 must not delete permissions")

	assert.Contains(t, downSQL, "DELETE FROM mst_role WHERE role_code = 'MB_ADMIN'")
	assert.NotContains(t, downSQL, "DELETE FROM mst_permission", "root.view belongs to seeds/main.go")
}
