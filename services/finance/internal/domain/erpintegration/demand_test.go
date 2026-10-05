package erpintegration

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validErpRow() ErpDemandRow {
	return ErpDemandRow{
		Period: " 202608 ", TxnCode: " INVADJ ", ItemCode: " YRN001 ", ItemName: " Yarn ",
		GradeCode: " A ", ShadeCode: " S1 ", ItemCount: 2, RateVariants: 1,
		QtyKg: decimal.RequireFromString("12.5"), GoappsBatch: " B1 ", GoappsSource: " SRC ",
	}
}

func TestItemKind(t *testing.T) {
	assert.Equal(t, ItemKindMB, ItemKindForCode(" cmb123"))
	assert.Equal(t, ItemKindYarn, ItemKindForCode("YRN"))
	k, err := ParseItemKind("MB")
	require.NoError(t, err)
	assert.Equal(t, "MB", k.String())
	_, err = ParseItemKind("mb")
	assert.ErrorIs(t, err, ErrInvalidDemandLine)
}

func TestNewDemandLineFromErpRow(t *testing.T) {
	l, err := NewDemandLineFromErpRow(9, validErpRow(), t0)
	require.NoError(t, err)
	assert.Equal(t, int64(9), l.BatchID)
	assert.Equal(t, "202608", l.Period)
	assert.Equal(t, "INVADJ", l.TxnCode)
	assert.Equal(t, "YRN001", l.ItemCode)
	assert.Equal(t, ItemKindYarn, l.Kind)
	assert.Equal(t, "S1", l.ShadeCode)
	assert.Equal(t, "B1", l.GoappsBatch)
	assert.Equal(t, t0, l.LoadedAt)
	assert.Equal(t, DemandKey{TxnCode: "INVADJ", ItemCode: "YRN001", GradeCode: "A", ShadeCode: "S1"}, l.Key())

	row := validErpRow()
	row.ItemCode = "CMB77"
	row.TxnCode = TxnMbInvAdj
	l, err = NewDemandLineFromErpRow(9, row, t0)
	require.NoError(t, err)
	assert.Equal(t, ItemKindMB, l.Kind)

	row.TxnCode = "OTHER"
	_, err = NewDemandLineFromErpRow(9, row, t0)
	assert.ErrorIs(t, err, ErrInvalidDemandLine)
}

func TestDemandLineValidate(t *testing.T) {
	base, err := NewDemandLineFromErpRow(1, validErpRow(), t0)
	require.NoError(t, err)
	cases := map[string]func(l *DemandLine){
		"period":        func(l *DemandLine) { l.Period = "2026" },
		"txn":           func(l *DemandLine) { l.TxnCode = "X" },
		"kind":          func(l *DemandLine) { l.Kind = "Z" },
		"empty item":    func(l *DemandLine) { l.ItemCode = "" },
		"kind mismatch": func(l *DemandLine) { l.Kind = ItemKindMB },
		"item width":    func(l *DemandLine) { l.ItemCode = strings.Repeat("Y", 51) },
		"name width":    func(l *DemandLine) { l.ItemName = strings.Repeat("n", 501) },
		"shade width":   func(l *DemandLine) { l.ShadeCode = strings.Repeat("s", 21) },
		"source width":  func(l *DemandLine) { l.GoappsSource = strings.Repeat("s", 17) },
		"neg count":     func(l *DemandLine) { l.PostedItems = -1 },
		"neg qty":       func(l *DemandLine) { l.QtyKg = decimal.RequireFromString("-0.1") },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			l := base
			mut(&l)
			assert.ErrorIs(t, l.Validate(), ErrInvalidDemandLine)
		})
	}
	assert.NoError(t, base.Validate())
}
