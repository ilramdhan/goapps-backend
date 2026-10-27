// Text-level guard for IAM 000096 (no DB needed): Workforce grants go only to the roles this migration
// created (description '[H004]...'), never to a pre-existing manual HR/DIREKSI_GM role (PR #236 review).
package postgres_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHRWorkforce096GrantsOnlyTaggedRoles(t *testing.T) {
	up, err := os.ReadFile("000096_seed_hr_workforce_permissions.up.sql")
	require.NoError(t, err)
	down, err := os.ReadFile("000096_seed_hr_workforce_permissions.down.sql")
	require.NoError(t, err)

	// Every grant to DIREKSI_GM/HR must carry the tag filter; SUPER_ADMIN is the only untagged grant.
	grants := regexp.MustCompile(`(?s)INSERT INTO role_permissions.*?ON CONFLICT`).FindAllString(string(up), -1)
	require.Len(t, grants, 3)
	for _, g := range grants {
		if strings.Contains(g, "'SUPER_ADMIN'") {
			continue
		}
		assert.Contains(t, g, "r.description LIKE '[H004]%'", g)
	}
	// Down only removes tagged roles.
	assert.Contains(t, string(down), "description LIKE '[H004]%'")
	assert.NotContains(t, string(down), "SELECT 1;")
}
