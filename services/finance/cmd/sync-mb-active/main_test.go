package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func TestParseFlags(t *testing.T) {
	o, err := parseFlags([]string{"--plan", "--file", "f.csv", "--out-dir", "/tmp/x"})
	require.NoError(t, err)
	assert.True(t, o.plan)
	assert.Equal(t, "/tmp/x", o.outDir)

	for _, args := range [][]string{
		nil,
		{"--plan"},
		{"--plan", "--report", "--file", "f"},
		{"--plan", "--apply", "--file", "f"},
		{"--report", "--apply"},
		{"--plan", "--file", "f", "--phase", "A"},
		{"--plan", "--file", "f", "--out-dir", ""},
		{"--bogus"},
	} {
		_, err := parseFlags(args)
		assert.Error(t, err, "%v", args)
	}
	o, err = parseFlags([]string{"--apply", "--plan-rows", "r.csv", "--plan-heads", "h.csv", "--phase", "A", "--actor", "me", "--batch", "5", "--limit", "2", "--only-sysid", "1,2"})
	require.NoError(t, err)
	assert.True(t, o.apply)
	assert.Equal(t, []string{"1", "2"}, splitCSVList(o.onlySysID))
	for _, args := range [][]string{
		{"--apply"},
		{"--apply", "--plan-rows", "r", "--plan-heads", "h", "--phase", "A"},
		{"--apply", "--plan-rows", "r", "--plan-heads", "h", "--actor", "me"},
		{"--apply", "--plan-rows", "r", "--plan-heads", "h", "--phase", "Z", "--actor", "me"},
		{"--apply", "--plan-rows", "r", "--plan-heads", "h", "--phase", "A", "--actor", " "},
		{"--apply", "--plan-rows", "r", "--plan-heads", "h", "--phase", "A", "--actor", "me", "--limit", "-1"},
		{"--plan", "--file", "f", "--limit", "3"},
		{"--apply", "--plan-rows", "r", "--plan-heads", "h", "--phase", "C", "--actor", "me"},
	} {
		_, err := parseFlags(args)
		assert.Error(t, err, "%v", args)
	}
	o.phase = "D"
	assert.Contains(t, run(t.Context(), o).Error(), "not wired")

	o, err = parseFlags([]string{"--report", "--plan-rows", "r.csv", "--plan-heads", "h.csv", "--out", "x.csv"})
	require.NoError(t, err)
	assert.True(t, o.report)
	for _, args := range [][]string{
		{"--report"},
		{"--report", "--plan-rows", "r.csv"},
		{"--report", "--plan-heads", "h.csv"},
		{"--report", "--plan-rows", "r", "--plan-heads", "h", "--file", "f"},
		{"--report", "--plan-rows", "r", "--plan-heads", "h", "--phase", "A"},
	} {
		_, err := parseFlags(args)
		assert.Error(t, err, "%v", args)
	}
}

func TestNewHeadWorkflow_Wires(t *testing.T) {
	require.NotNil(t, newHeadWorkflow(postgres.NewDBFromSQL(nil)))
}
