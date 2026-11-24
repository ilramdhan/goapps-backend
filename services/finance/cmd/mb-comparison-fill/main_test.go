package main

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseArgs(t *testing.T) {
	o, err := parseArgs([]string{"plan", "--file", "d.csv", "--orion", "o.csv"}, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, "plan", o.mode)

	o, err = parseArgs([]string{"apply", "--plan-file", "p.csv", "--actor", "mbcmp-local", "--limit", "5"}, io.Discard)
	require.NoError(t, err)
	assert.False(t, o.commit, "apply defaults to dry-run")

	o, err = parseArgs([]string{"revert", "--results", "a.csv, b.csv", "--actor", "mbcmp-local"}, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.csv", "b.csv"}, o.results)

	for _, args := range [][]string{
		nil, {"nope"}, {"plan"}, {"plan", "--file", "d"}, {"apply", "--actor", "mbcmp-x"},
		{"apply", "--plan-file", "p", "--actor", "bob"}, {"apply", "--plan-file", "p", "--actor", "mbcmp-x", "--limit", "-1"},
		{"revert", "--actor", "mbcmp-x"}, {"revert", "--results", "a"},
	} {
		_, err := parseArgs(args, io.Discard)
		assert.Error(t, err, "%v", args)
	}
}
