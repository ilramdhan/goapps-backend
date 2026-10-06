package erpintegration

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func okCoverage() CoverageLine {
	p, c := int64(1), int64(2)
	return CoverageLine{BatchID: 1, Kind: ItemKindYarn, ItemCode: "YRN", ShadeCode: "S1",
		ProductSysID: &p, CostID: &c, Status: CoverageOK, QtyKg: decimal.RequireFromString("1")}
}

func TestCoverageStatus(t *testing.T) {
	all := AllCoverageStatuses()
	require.Len(t, all, 7)
	all[0] = "X"
	assert.Equal(t, CoverageOK, AllCoverageStatuses()[0], "copy")
	for _, s := range AllCoverageStatuses() {
		got, err := ParseCoverageStatus(s.String())
		require.NoError(t, err)
		assert.Equal(t, s, got)
		assert.Equal(t, s != CoverageOK, s.IsBlocking())
	}
	_, err := ParseCoverageStatus("MULTI_MAPPING")
	assert.ErrorIs(t, err, ErrInvalidCoverageLine)
}

func TestCoverageLineValidate(t *testing.T) {
	assert.NoError(t, okCoverage().Validate())
	assert.Equal(t, CoverageKey{ItemCode: "YRN", ShadeCode: "S1"}, okCoverage().Key())

	gap := okCoverage()
	gap.Status, gap.ProductSysID, gap.CostID = CoverageNoMapping, nil, nil
	assert.NoError(t, gap.Validate(), "a gap needs no ids")

	cases := map[string]func(l *CoverageLine){
		"kind":        func(l *CoverageLine) { l.Kind = "Z" },
		"status":      func(l *CoverageLine) { l.Status = "Z" },
		"empty item":  func(l *CoverageLine) { l.ItemCode = " " },
		"item width":  func(l *CoverageLine) { l.ItemCode = strings.Repeat("Y", 51) },
		"shade width": func(l *CoverageLine) { l.ShadeCode = strings.Repeat("s", 21) },
		"neg qty":     func(l *CoverageLine) { l.QtyKg = decimal.RequireFromString("-1") },
		"ok no cost":  func(l *CoverageLine) { l.CostID = nil },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			l := okCoverage()
			mut(&l)
			assert.ErrorIs(t, l.Validate(), ErrInvalidCoverageLine)
		})
	}
}

func TestCoverageCounts(t *testing.T) {
	assert.False(t, CoverageCounts{}.AllOK())
	c := CoverageCounts{CoverageOK: 3}
	assert.Equal(t, int64(3), c.Total())
	assert.True(t, c.AllOK())
	c[CoverageNoCost] = 1
	assert.Equal(t, int64(4), c.Total())
	assert.False(t, c.AllOK())
}
