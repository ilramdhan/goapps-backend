package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	o, err = parseFlags([]string{"--apply"})
	require.NoError(t, err)
	assert.Contains(t, run(t.Context(), o).Error(), "not implemented yet")

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
