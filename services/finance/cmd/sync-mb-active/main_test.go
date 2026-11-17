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
	for _, m := range []string{"--report", "--apply"} {
		o, err := parseFlags([]string{m})
		require.NoError(t, err)
		assert.Error(t, run(t.Context(), o))
		assert.Contains(t, run(t.Context(), o).Error(), "not implemented yet")
	}
}
