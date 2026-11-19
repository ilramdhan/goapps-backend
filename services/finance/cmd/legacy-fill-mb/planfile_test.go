package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
)

const planHdr = "entity,id,sysid,field,goapps_value,legacy_value,match_method,ambiguous,usulan,note,class,decision,reason\n"

func TestParsePlanTamperedClass(t *testing.T) {
	_, err := parsePlan(bytes.NewBufferString(planHdr + "head,h1,10,status,a,b,,,OVERWRITE,,D,APPLY,\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match registry")
	rows, err := parsePlan(bytes.NewBufferString(planHdr + "head,h1,10,status,a,b,,,OVERWRITE,,,APPLY,\n"))
	require.NoError(t, err)
	assert.Equal(t, mblegacyfill.ClassL, rows[0].Class)
}

func TestParsePlanEmptyLegacy(t *testing.T) {
	_, err := parsePlan(bytes.NewBufferString(planHdr + "head,h1,10,status,a,,,,OVERWRITE,,L,APPLY,\n"))
	require.Error(t, err)
}

func TestParsePlanFractionalInt(t *testing.T) {
	_, err := parsePlan(bytes.NewBufferString(planHdr + "head,h1,10,fil,1,2.5,,,OVERWRITE,,D,APPLY,\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fractional")
}

func TestOnlySysIDKeepsHeadGroup(t *testing.T) {
	rows, err := parsePlan(bytes.NewBufferString(planHdr +
		"head,h1,10,status,a,b,,,OVERWRITE,,L,APPLY,\n" +
		"spin,s1,99,status,a,b,,,OVERWRITE,,L,APPLY,\n" +
		"head,h2,20,status,a,b,,,OVERWRITE,,L,APPLY,\n"))
	require.NoError(t, err)
	sh := map[string]string{"s1": "h1"}
	g := selectApplyRows(rows, sh, []string{"10"}, 0)
	require.Len(t, g, 1)
	assert.Len(t, g[0], 2)
	g = selectApplyRows(rows, sh, []string{"99"}, 0)
	require.Len(t, g, 1)
	assert.Len(t, g[0], 2)
}
