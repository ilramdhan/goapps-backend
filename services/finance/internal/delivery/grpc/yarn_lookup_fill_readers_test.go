package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// newMBHeadWithLDR builds a minimal MB Head carrying both LDR values.
func newMBHeadWithLDR(t *testing.T, ldrPrsn, runLdrPct *float64) *mbhead.Entity {
	t.Helper()
	e, err := mbhead.New(mbhead.NewParams{
		MBCosting:    "MB001",
		MBHLdrPrsn:   ldrPrsn,
		MBHRunLdrPct: runLdrPct,
		CreatedBy:    "admin",
	})
	require.NoError(t, err)
	return e
}

// TestMBHeadNumericReaders_LDRColumns covers the head side of Task 3.1: both
// mbh_run_ldr_pct (actual LDR used in production) and mbh_ldr_prsn (planned LDR)
// must be registered so a lookup_source_column pointing at them resolves.
func TestMBHeadNumericReaders_LDRColumns(t *testing.T) {
	planned, actual := 63.0, 89.0
	e := newMBHeadWithLDR(t, &planned, &actual)

	for col, want := range map[string]float64{
		"mbh_run_ldr_pct": actual,
		"mbh_ldr_prsn":    planned,
	} {
		reader, ok := mbHeadNumericReaders[col]
		require.Truef(t, ok, "no reader registered for %s", col)

		got, has := reader(e)
		assert.Truef(t, has, "%s should report a value", col)
		assert.Equalf(t, want, got, "%s value mismatch", col)
	}
}

// TestMBHeadNumericReaders_LDRColumnsNil verifies a nil LDR reports no value
// rather than filling a misleading zero.
func TestMBHeadNumericReaders_LDRColumnsNil(t *testing.T) {
	e := newMBHeadWithLDR(t, nil, nil)

	for _, col := range []string{"mbh_run_ldr_pct", "mbh_ldr_prsn"} {
		reader, ok := mbHeadNumericReaders[col]
		require.Truef(t, ok, "no reader registered for %s", col)

		_, has := reader(e)
		assert.Falsef(t, has, "%s should report no value when nil", col)
	}
}

// TestMBHeadEntity_RunLdrPctRoundTrip guards the entity plumbing the readers
// depend on: constructor, getter, and Update all carry mbh_run_ldr_pct.
func TestMBHeadEntity_RunLdrPctRoundTrip(t *testing.T) {
	e := newMBHeadWithLDR(t, nil, nil)
	assert.Nil(t, e.MBHRunLdrPct())

	actual := 89.5
	require.NoError(t, e.Update(mbhead.UpdateInput{MBHRunLdrPct: &actual}, "admin"))
	require.NotNil(t, e.MBHRunLdrPct())
	assert.InDelta(t, actual, *e.MBHRunLdrPct(), 0.0001)
}
