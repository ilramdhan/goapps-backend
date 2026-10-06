package erpintegration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestIsCurrencyAcceptable_V09Interim(t *testing.T) {
	t.Parallel()
	cases := []struct {
		label   string
		outlier bool
		want    bool
	}{
		{"USD", false, true},
		{"USD", true, true}, // USD always accepted
		{"usd", false, true},
		{" USD ", true, true},
		{"IDR", false, true},
		{"IDR", true, false},
		{"idr", false, true},
		{"EUR", false, false},
		{"", false, false},
		{"", true, false},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, IsCurrencyAcceptable(c.label, c.outlier), "label=%q outlier=%v", c.label, c.outlier)
	}
}

func TestIsCurrencyOutlier(t *testing.T) {
	t.Parallel()
	cases := []struct {
		item string
		cost string
		want bool
	}{
		{"POY0000275", "20", false},
		{"POY0000275", "20.000001", true},
		{"pty0000600", "21", true}, // case-insensitive prefix
		{"TTY0000020", "19.99", false},
		{"HOY1", "25", true},
		{"ACY1", "25", true},
		{"ITY1", "25", true},
		{"MMK1", "25", true},
		{"CMB0001", "92", false}, // unguarded: only the 100 bound
		{"CMB0001", "100", false},
		{"CMB0001", "100.000001", true},
		{"", "99", false},
		{"", "19271.5", true},
		{"POY1", "19271.5", true},
		{"XPOY", "50", false}, // prefix must be at the start
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, IsCurrencyOutlier(c.item, d(c.cost)), "item=%q cost=%s", c.item, c.cost)
	}
}

func TestCurrencySanityFilter_Normalize(t *testing.T) {
	t.Parallel()
	f, err := CurrencySanityFilter{}.Normalize()
	require.NoError(t, err)
	assert.Equal(t, DefaultOutlierLimit, f.OutlierLimit)

	f, err = CurrencySanityFilter{Period: " 202609 ", OutlierLimit: MaxOutlierLimit + 1}.Normalize()
	require.NoError(t, err)
	assert.Equal(t, "202609", f.Period)
	assert.Equal(t, MaxOutlierLimit, f.OutlierLimit)

	for _, bad := range []string{"2026", "202613", "999901", "20260a"} {
		_, err = CurrencySanityFilter{Period: bad}.Normalize()
		assert.Truef(t, errors.Is(err, ErrInvalidPeriod), bad)
	}
}

type fakeSanityReader struct {
	gotFilter CurrencySanityFilter
	gotPeriod string
	outliers  []CurrencyOutlier
	total     int64
	periods   []CurrencyPeriodSummary
	err       error
}

func (f *fakeSanityReader) LabelDistribution(_ context.Context, fl CurrencySanityFilter) ([]CurrencyLabelCount, error) {
	f.gotFilter = fl
	return []CurrencyLabelCount{{Period: "202609", Currency: "IDR", Rows: 5}}, f.err
}

func (f *fakeSanityReader) Percentiles(context.Context, CurrencySanityFilter) (CurrencyPercentiles, error) {
	return CurrencyPercentiles{Rows: 5, Max: decimal.NewNullDecimal(d("92"))}, nil
}

func (f *fakeSanityReader) ListOutliers(context.Context, CurrencySanityFilter) ([]CurrencyOutlier, int64, error) {
	return f.outliers, f.total, nil
}

func (f *fakeSanityReader) PeriodSummaries(_ context.Context, p string) ([]CurrencyPeriodSummary, error) {
	f.gotPeriod = p
	return f.periods, nil
}

func TestCurrencySanityService_Report(t *testing.T) {
	t.Parallel()
	r := &fakeSanityReader{
		outliers: []CurrencyOutlier{{CostID: 1, Period: "202605", CostPerUnit: d("19271.5")}},
		total:    9704,
		periods: []CurrencyPeriodSummary{
			{Period: "202605", OverOverall: 9704},
			{Period: "202609", OverOverall: 0},
		},
	}
	rep, err := NewCurrencySanityService(r).Report(context.Background(), CurrencySanityFilter{LinkedOnly: true})
	require.NoError(t, err)
	assert.True(t, r.gotFilter.LinkedOnly)
	assert.Equal(t, DefaultOutlierLimit, r.gotFilter.OutlierLimit)
	assert.Equal(t, "", r.gotPeriod)
	assert.Equal(t, int64(9704), rep.OutlierTotal)
	assert.True(t, rep.OutlierCapped)
	require.Len(t, rep.Periods, 2)
	assert.False(t, rep.Periods[0].RelabelSafe)
	assert.True(t, rep.Periods[0].ExcludedByPlan)
	assert.True(t, rep.Periods[1].RelabelSafe)
	assert.False(t, rep.Periods[1].ExcludedByPlan)
}

func TestCurrencySanityService_Errors(t *testing.T) {
	t.Parallel()
	_, err := NewCurrencySanityService(&fakeSanityReader{}).Report(context.Background(), CurrencySanityFilter{Period: "bad"})
	require.ErrorIs(t, err, ErrInvalidPeriod)

	boom := errors.New("boom")
	_, err = NewCurrencySanityService(&fakeSanityReader{err: boom}).Report(context.Background(), CurrencySanityFilter{})
	require.ErrorIs(t, err, boom)
}
