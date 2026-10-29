// Text-level guard for IAM 000098 (no DB needed): etlstatus grants to HR/DIREKSI_GM only on roles tagged
// '[H004]' (same rule as 000096); down removes only the permission and its grants.
package postgres_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHREtlStatus098GrantsOnlyTaggedRoles(t *testing.T) {
	up, err := os.ReadFile("000098_seed_hr_etlstatus_permission.up.sql")
	require.NoError(t, err)
	down, err := os.ReadFile("000098_seed_hr_etlstatus_permission.down.sql")
	require.NoError(t, err)

	assert.Contains(t, string(up), "'finance.bi.etlstatus.view'")
	grants := regexp.MustCompile(`(?s)INSERT INTO role_permissions.*?ON CONFLICT`).FindAllString(string(up), -1)
	require.Len(t, grants, 2)
	for _, g := range grants {
		if strings.Contains(g, "'SUPER_ADMIN'") {
			continue
		}
		assert.Contains(t, g, "r.description LIKE '[H004]%'", g)
	}
	assert.NotContains(t, string(down), "mst_role")
	assert.Contains(t, string(down), "DELETE FROM mst_permission")
}
