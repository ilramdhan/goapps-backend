package erpintegration

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

func btND(s string) decimal.NullDecimal {
	return decimal.NullDecimal{Decimal: decimal.RequireFromString(s), Valid: true}
}

func btStd(item string, b erprule.Basis, std string) StdRow {
	r := StdRow{Key: ErpKey{ItemCode: item, GradeCode: "A", ShadeCode: "X"}, Basis: b}
	if std != "" {
		r.StdCost = btND(std)
	}
	return r
}

func btLeg(item, rate string) LegacyAdjRate {
	l := LegacyAdjRate{Key: ErpKey{ItemCode: item + " ", GradeCode: "A", ShadeCode: " X"}, Items: 1, RateVariants: 1}
	if rate != "" {
		l.MaxRate = btND(rate)
	}
	return l
}

func TestCompareBacktest_Classes(t *testing.T) {
	rep := CompareBacktest(
		[]StdRow{
			btStd("C", erprule.BasisCost, "1.5"),
			btStd("A", erprule.BasisSPPTY, "2"),
			btStd("B", erprule.BasisCost, "3"),
			btStd("D", erprule.BasisCost, "1"),
		},
		[]LegacyAdjRate{btLeg("A", "2.00001"), btLeg("B", "3"), btLeg("C", "1"), btLeg("E", "9"), btLeg("D", "0")},
	)
	require.Len(t, rep.Lines, 5)
	got := map[string]BacktestLine{}
	for i, l := range rep.Lines {
		got[l.Key.ItemCode] = l
		assert.Equal(t, string(rune('A'+i)), l.Key.ItemCode, "ordered by key")
	}
	assert.Equal(t, BacktestDiff, got["A"].Class)
	assert.True(t, got["A"].Fail, "SP* diff fails")
	assert.Equal(t, BacktestMatch, got["B"].Class)
	assert.Equal(t, BacktestDiff, got["C"].Class)
	assert.False(t, got["C"].Fail, "COST diff does not fail")
	assert.Equal(t, "0.5", got["C"].Delta.Decimal.String())
	assert.Equal(t, "50", got["C"].DeltaPct.Decimal.String())
	assert.True(t, got["D"].Legacy.Valid)
	assert.False(t, got["D"].DeltaPct.Valid, "legacy 0 -> NULL pct")
	assert.Equal(t, BacktestOnlyLegacy, got["E"].Class)
	assert.True(t, rep.Failed)
	assert.Equal(t, 1, rep.Counts[BacktestMatch])
	assert.Equal(t, 3, rep.Counts[BacktestDiff])
	assert.Equal(t, 1, rep.Counts[BacktestOnlyLegacy])
}

func TestCompareBacktest_OnlyGoAppsSPFails(t *testing.T) {
	rep := CompareBacktest([]StdRow{btStd("A", erprule.BasisSPBSD, "1")}, nil)
	require.Len(t, rep.Lines, 1)
	assert.Equal(t, BacktestOnlyGoApps, rep.Lines[0].Class)
	assert.True(t, rep.Failed)
	rep = CompareBacktest([]StdRow{btStd("A", erprule.BasisCost, "1")}, nil)
	assert.False(t, rep.Failed)
}

func TestCompareBacktest_RoundingHalfAwayFromZero(t *testing.T) {
	rep := CompareBacktest([]StdRow{btStd("A", erprule.BasisCost, "1")}, []LegacyAdjRate{btLeg("A", "3")})
	assert.Equal(t, "-66.66667", rep.Lines[0].DeltaPct.Decimal.String())
	rep = CompareBacktest([]StdRow{btStd("A", erprule.BasisCost, "1.000005")}, []LegacyAdjRate{btLeg("A", "1.00000")})
	assert.Equal(t, BacktestDiff, rep.Lines[0].Class)
}
