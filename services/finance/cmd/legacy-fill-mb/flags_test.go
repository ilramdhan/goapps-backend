package main

import (
	"bytes"
	"errors"
	"flag"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlagsValid(t *testing.T) {
	o, err := parseFlags([]string{"--plan", "--proposal", "p.csv", "--out-dir", "/tmp/x"})
	require.NoError(t, err)
	assert.True(t, o.plan)
	assert.Equal(t, "/tmp/x", o.outDir)

	o, err = parseFlags([]string{"--apply", "--plan-file", "pl.csv", "--actor", "me", "--batch", "50", "--limit", "10", "--only-sysid", "1,2"})
	require.NoError(t, err)
	assert.Equal(t, 50, o.batch)
	assert.Equal(t, []string{"1", "2"}, splitList(o.onlySysID))

	o, err = parseFlags([]string{"--revert", "--input", "a.csv,b.csv", "--input", "c.csv", "--actor", "me"})
	require.NoError(t, err)
	assert.Equal(t, stringList{"a.csv", "b.csv", "c.csv"}, o.input)
}

func TestParseFlagsInvalid(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--plan"},
		{"--plan", "--apply", "--proposal", "p"},
		{"--plan", "--proposal", "p", "--actor", "x"},
		{"--plan", "--proposal", "p", "--out-dir", ""},
		{"--apply", "--plan-file", "f"},
		{"--apply", "--plan-file", "f", "--actor", " "},
		{"--apply", "--actor", "a"},
		{"--apply", "--plan-file", "f", "--actor", "a", "--batch", "-1"},
		{"--apply", "--plan-file", "f", "--actor", "a", "--proposal", "p"},
		{"--revert", "--actor", "a"},
		{"--revert", "--input", "r.csv"},
		{"--revert", "--input", "r.csv", "--actor", "a", "--limit", "3"},
		{"--bogus"},
	} {
		_, err := parseFlags(args)
		assert.Error(t, err, "args %v", args)
	}
}

func TestParseFlagsHelp(t *testing.T) {
	var buf bytes.Buffer
	_, err := parseFlagsTo([]string{"--help"}, &buf)
	require.True(t, errors.Is(err, flag.ErrHelp))
	assert.Contains(t, buf.String(), "--plan")
	assert.Contains(t, buf.String(), "-only-sysid")
}

func TestSelectApplyRows(t *testing.T) {
	csvText := "entity,id,sysid,field,goapps_value,legacy_value,match_method,ambiguous,usulan,note,class,decision,reason\n" +
		"spin,s1,10,status,a,b,,,OVERWRITE,,L,APPLY,\n" +
		"head,h1,10,status,a,b,,,OVERWRITE,,L,APPLY,\n" +
		"head,h2,20,notes,a,b,,,OVERWRITE,,L,CHANGED,\n" +
		"spin,s3,30,notes,a,b,,,FILL_EMPTY,,L,APPLY,\n"
	rows, err := parsePlan(bytes.NewBufferString(csvText))
	require.NoError(t, err)
	require.Len(t, rows, 4)

	g := selectApplyRows(rows, nil, nil, 0)
	require.Len(t, g, 3)                  // unresolved spins group alone
	assert.Equal(t, "h1", g[0][0].Row.ID) // head-bearing group first
	assert.Len(t, selectApplyRows(rows, nil, []string{"30"}, 0), 1)
	assert.Len(t, selectApplyRows(rows, nil, nil, 1), 1)

	// a spin with another SYSID joins its head's group
	joined := selectApplyRows(rows, map[string]string{"s1": "h1", "s3": "h1"}, nil, 0)
	require.Len(t, joined, 1)
	assert.Len(t, joined[0], 3)
	assert.Len(t, chunkGroups(g, 1), 3)
	assert.Len(t, chunkGroups(g, 0), 1)

	_, err = parsePlan(bytes.NewBufferString(csvText[:20] + "\n"))
	assert.Error(t, err)
}

func TestParsePlanKeepsRejectRows(t *testing.T) {
	csvText := "entity,id,sysid,field,goapps_value,legacy_value,match_method,ambiguous,usulan,note,class,decision,reason\n" +
		"head,h1,10,entry_status,a,b,,,,,X,REJECT,not writable\n" +
		"bogus,,10,whatever,a,b,,,,,X,REJECT,unknown entity\n" +
		"head,h1,10,status,a,b,,,OVERWRITE,,L,APPLY,\n"
	rows, err := parsePlan(bytes.NewBufferString(csvText))
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, mblegacyfill.DecisionReject, rows[0].Decision)
	assert.Len(t, selectApplyRows(rows, nil, nil, 0), 1)

	bad := "entity,id,sysid,field,goapps_value,legacy_value,match_method,ambiguous,usulan,note,class,decision,reason\n" +
		"head,h1,10,entry_status,a,b,,,,,L,APPLY,\n"
	_, err = parsePlan(bytes.NewBufferString(bad))
	assert.Error(t, err)
}

func TestParseFlagsPostEffectsOnly(t *testing.T) {
	o, err := parseFlags([]string{"--post-effects-only", "--input", "a.csv,b.csv", "--actor", "me", "--out-dir", "/tmp/x"})
	require.NoError(t, err)
	assert.True(t, o.postEffectsOnly)
	assert.Equal(t, stringList{"a.csv", "b.csv"}, o.input)

	for _, args := range [][]string{
		{"--post-effects-only"},
		{"--post-effects-only", "--input", "a.csv"},
		{"--post-effects-only", "--input", "a.csv", "--actor", " "},
		{"--post-effects-only", "--input", "a.csv", "--actor", "x", "--plan"},
		{"--post-effects-only", "--input", "a.csv", "--actor", "x", "--apply"},
		{"--post-effects-only", "--input", "a.csv", "--actor", "x", "--revert"},
		{"--post-effects-only", "--input", "a.csv", "--actor", "x", "--plan-file", "f"},
		{"--post-effects-only", "--input", "a.csv", "--actor", "x", "--batch", "5"},
	} {
		_, err := parseFlags(args)
		assert.Error(t, err, "args=%v", args)
	}
}
