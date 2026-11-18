package main

import (
	"bytes"
	"flag"
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

	// Phase D needs --batch like C.
	for _, ph := range []string{"C", "D"} {
		_, err = parseFlags([]string{"--apply", "--plan-rows", "r", "--plan-heads", "h", "--phase", ph, "--actor", "me"})
		assert.Error(t, err, "phase %s without --batch", ph)
		o, err = parseFlags([]string{"--apply", "--plan-rows", "r", "--plan-heads", "h", "--phase", ph, "--actor", "me", "--batch", "10"})
		require.NoError(t, err, "phase %s with --batch", ph)
		assert.Equal(t, ph, o.phase)
	}
	_, err = parseFlags([]string{"--apply", "--plan-rows", "r", "--plan-heads", "h", "--phase", "D", "--actor", "me", "--batch", "0"})
	assert.Error(t, err)

	// --revert
	o, err = parseFlags([]string{"--revert", "--input", "a.csv", "--input", "b.csv", "--actor", "me", "--limit", "3", "--only-sysid", "1"})
	require.NoError(t, err)
	assert.True(t, o.revert)
	assert.Equal(t, []string{"a.csv", "b.csv"}, []string(o.input))
	for _, args := range [][]string{
		{"--revert"},
		{"--revert", "--input", "a.csv"},
		{"--revert", "--actor", "me"},
		{"--revert", "--input", "a.csv", "--actor", " "},
		{"--revert", "--input", "a.csv", "--actor", "me", "--phase", "A"},
		{"--revert", "--input", "a.csv", "--actor", "me", "--batch", "2"},
		{"--revert", "--input", "a.csv", "--actor", "me", "--plan-rows", "r"},
		{"--revert", "--apply", "--input", "a.csv", "--actor", "me"},
		{"--revert", "--input", "a.csv", "--actor", "me", "--limit", "-1"},
		{"--apply", "--plan-rows", "r", "--plan-heads", "h", "--phase", "A", "--actor", "me", "--input", "a.csv"},
		{"--plan", "--file", "f", "--input", "a.csv"},
		{"--report", "--plan-rows", "r", "--plan-heads", "h", "--input", "a.csv"},
	} {
		_, err := parseFlags(args)
		assert.Error(t, err, "%v", args)
	}

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

func TestParseFlagsHelpPrintsUsage(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		var buf bytes.Buffer
		_, err := parseFlagsTo([]string{arg}, &buf)
		require.ErrorIs(t, err, flag.ErrHelp, arg)
		out := buf.String()
		assert.Contains(t, out, "Usage of "+cmdName, arg)
		for _, f := range []string{"-plan", "-apply", "-phase", "-prev-results", "-revert"} {
			assert.Contains(t, out, f, arg)
		}
	}
	var buf bytes.Buffer
	_, err := parseFlagsTo([]string{"--bogus"}, &buf)
	require.Error(t, err)
	assert.NotErrorIs(t, err, flag.ErrHelp)
	assert.Empty(t, buf.String(), "parse errors stay silent (main logs them)")
}
