package lookupsync

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type write struct {
	op       string // NUM | TXT | DEL
	param    uuid.UUID
	value    string
	filledBy string
	actor    string
}

type fakeApplyTx struct {
	reader  SyncReader
	locked  bool
	missing bool
	writes  []write
	failOn  string
}

func (f *fakeApplyTx) Reader() SyncReader { return f.reader }

func (f *fakeApplyTx) LockProduct(context.Context, int64) (string, bool, bool, error) {
	if f.missing {
		return "", false, false, nil
	}
	return "P-1", f.locked, true, nil
}

func (f *fakeApplyTx) UpsertNumeric(_ context.Context, _ int64, p uuid.UUID, v decimal.Decimal, by, actor string) error {
	f.writes = append(f.writes, write{"NUM", p, v.String(), by, actor})
	return f.fail("NUM")
}

func (f *fakeApplyTx) UpsertText(_ context.Context, _ int64, p uuid.UUID, v, by, actor string) error {
	f.writes = append(f.writes, write{"TXT", p, v, by, actor})
	return f.fail("TXT")
}

func (f *fakeApplyTx) DeleteValue(_ context.Context, _ int64, p uuid.UUID) (bool, error) {
	f.writes = append(f.writes, write{op: "DEL", param: p})
	return true, f.fail("DEL")
}

func (f *fakeApplyTx) fail(op string) error {
	if f.failOn == op {
		return errors.New("boom")
	}
	return nil
}

type fakeRunner struct {
	tx        *fakeApplyTx
	committed bool
}

func (r *fakeRunner) InTx(_ context.Context, fn func(ApplyTx) error) error {
	if err := fn(r.tx); err != nil {
		return err
	}
	r.committed = true
	return nil
}

// applyFixture: the planner's own reader is empty, so any plan must come from
// the tx-bound reader.
func applyFixture(t *testing.T) (*fixture, *fakeRunner, *Applier) {
	f := newFixture(t)
	f.res.results["MACHINE|M1|MC_CODE"] = FillResult{Numeric: map[string]float64{"MC_SPEED": 1200.5}, Text: map[string]string{"MC_LABEL": "Fast"}}
	f.reader.rows = []TriggerRow{mcRow(1, "M1")}
	runner := &fakeRunner{tx: &fakeApplyTx{reader: f.reader}}
	return f, runner, NewApplier(NewPlanner(f.res, f.kids, &fakeReader{}), runner)
}

func TestApplyProduct_SetAndClear(t *testing.T) {
	f, runner, a := applyFixture(t)
	f.applicable(1, f.speed, num("1000"))
	f.applicable(1, f.label, nil)
	f.applicable(1, f.rpm, num("30"))

	res, err := a.ApplyProduct(context.Background(), 1, Scope{}, "alice")
	require.NoError(t, err)
	assert.True(t, runner.committed)
	assert.Equal(t, ProductApplyResult{ProductSysID: 1, ProductCode: "P-1", ValuesWritten: 2, ValuesCleared: 1}, res)
	assert.ElementsMatch(t, []write{
		{"NUM", f.speed.ID(), "1200.5", FilledByLookupSync, "alice"},
		{"TXT", f.label.ID(), "Fast", FilledByLookupSync, "alice"},
		{op: "DEL", param: f.rpm.ID()},
	}, runner.tx.writes)
}

func TestApplyProduct_LockedSkipsWithoutWrites(t *testing.T) {
	f, runner, a := applyFixture(t)
	f.applicable(1, f.speed, num("1000"))
	runner.tx.locked = true

	res, err := a.ApplyProduct(context.Background(), 1, Scope{}, "alice")
	require.NoError(t, err)
	assert.True(t, res.Skipped)
	assert.Equal(t, SkipLocked, res.SkipReason)
	assert.Equal(t, "P-1", res.ProductCode)
	assert.Empty(t, runner.tx.writes)
	assert.True(t, runner.committed)
}

func TestApplyProduct_MissingProductSkipped(t *testing.T) {
	_, runner, a := applyFixture(t)
	runner.tx.missing = true
	res, err := a.ApplyProduct(context.Background(), 1, Scope{}, "alice")
	require.NoError(t, err)
	assert.Equal(t, SkipProductNotFound, res.SkipReason)
	assert.Empty(t, runner.tx.writes)
}

func TestApplyProduct_UnchangedIsNoOp(t *testing.T) {
	f, runner, a := applyFixture(t)
	f.applicable(1, f.speed, num("1200.500000"))
	f.applicable(1, f.label, txt("Fast"))
	f.applicable(1, f.rpm, nil)

	res, err := a.ApplyProduct(context.Background(), 1, Scope{}, "alice")
	require.NoError(t, err)
	assert.False(t, res.Skipped)
	assert.Zero(t, res.ValuesWritten)
	assert.Zero(t, res.ValuesCleared)
	assert.Empty(t, runner.tx.writes)
}

func TestApplyProduct_NonApplicableChildUntouched(t *testing.T) {
	f, runner, a := applyFixture(t)
	f.applicable(1, f.speed, num("1000")) // label/rpm not applicable

	res, err := a.ApplyProduct(context.Background(), 1, Scope{}, "alice")
	require.NoError(t, err)
	assert.Equal(t, 1, res.ValuesWritten)
	require.Len(t, runner.tx.writes, 1)
	assert.Equal(t, f.speed.ID(), runner.tx.writes[0].param)
}

func TestApplyProduct_WriteErrorRollsBack(t *testing.T) {
	f, runner, a := applyFixture(t)
	f.applicable(1, f.speed, num("1000"))
	runner.tx.failOn = "NUM"

	_, err := a.ApplyProduct(context.Background(), 1, Scope{}, "alice")
	require.Error(t, err)
	assert.False(t, runner.committed)
}
