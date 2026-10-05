package erpintegration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

type btLister struct{ rows []domain.StdRow }

func (l btLister) List(context.Context, int64) ([]domain.StdRow, error) { return l.rows, nil }

type btReader struct {
	rows  []domain.LegacyAdjRate
	reads int
}

func (r *btReader) ReadLegacyAdjRates(context.Context, string) ([]domain.LegacyAdjRate, error) {
	r.reads++
	return r.rows, nil
}

func btHandler(t *testing.T, order *[]string, failAt string) (*BacktestHandler, *btReader, *memBatches) {
	t.Helper()
	mb := newMemBatches()
	create := NewCreateBatchHandler(mb, nil, nil)
	mk := func(name string) backtestStepFn {
		return func(context.Context, int64, string) error {
			*order = append(*order, name)
			if name == failAt {
				return errors.New("boom")
			}
			return nil
		}
	}
	rd := &btReader{rows: []domain.LegacyAdjRate{{Key: domain.ErpKey{ItemCode: "POY1 ", GradeCode: "A", ShadeCode: "X"}, MaxRate: decimal.NullDecimal{Decimal: decimal.NewFromInt(2), Valid: true}}}}
	std := domain.StdRow{Key: domain.ErpKey{ItemCode: "POY1", GradeCode: "A", ShadeCode: "X"}, Basis: erprule.BasisSPPTY,
		StdCost: decimal.NullDecimal{Decimal: decimal.NewFromInt(3), Valid: true}}
	return &BacktestHandler{
		create: create.Handle,
		steps:  []backtestStepFn{mk("load"), mk("cov"), mk("derive"), mk("validate")},
		lister: btLister{rows: []domain.StdRow{std}}, reader: rd,
	}, rd, mb
}

func TestBacktest_RefusedPeriods(t *testing.T) {
	for _, p := range []string{"202605", "202603", "202610", "2026-06", ""} {
		var order []string
		h, rd, _ := btHandler(t, &order, "")
		_, id, err := h.Run(context.Background(), BacktestCommand{Period: p, Actor: "alice"})
		require.ErrorIs(t, err, ErrBacktestPeriodRefused, p)
		require.ErrorIs(t, err, domain.ErrInvalidBatchPeriod)
		assert.Zero(t, id)
		assert.Empty(t, order)
		assert.Zero(t, rd.reads)
	}
}

func TestBacktest_RunsShadowFlowAndCompares(t *testing.T) {
	var order []string
	h, rd, _ := btHandler(t, &order, "")
	rep, id, err := h.Run(context.Background(), BacktestCommand{Period: "202604", Actor: "alice"})
	require.NoError(t, err)
	assert.NotZero(t, id)
	assert.Equal(t, []string{"load", "cov", "derive", "validate"}, order)
	assert.Equal(t, 1, rd.reads)
	require.Len(t, rep.Lines, 1)
	assert.Equal(t, domain.BacktestDiff, rep.Lines[0].Class)
	assert.True(t, rep.Failed)
}

func TestBacktest_StepErrorStops(t *testing.T) {
	var order []string
	h, rd, _ := btHandler(t, &order, "derive")
	_, id, err := h.Run(context.Background(), BacktestCommand{Period: "202606", Actor: "alice"})
	require.Error(t, err)
	assert.NotZero(t, id)
	assert.Equal(t, []string{"load", "cov", "derive"}, order)
	assert.Zero(t, rd.reads)
}
