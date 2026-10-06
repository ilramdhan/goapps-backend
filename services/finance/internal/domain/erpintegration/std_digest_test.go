package erpintegration

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func digestRow(item, grade, shade string, status DeriveStatus, std string) StdRow {
	r := StdRow{Key: ErpKey{ItemCode: item, GradeCode: grade, ShadeCode: shade}, Status: status,
		Source: SourceAX, FgType: "Type 1"}
	if std != "" {
		d := decimal.NewNullDecimal(decimal.RequireFromString(std))
		r.StdCost, r.ConvCost, r.ProdValLoss = d, d, d
	}
	return r
}

func TestComputeStdDigest_EmptySet_MD5OfEmpty(t *testing.T) {
	d, err := ComputeStdDigest(nil)
	require.NoError(t, err)
	assert.Equal(t, "d41d8cd98f00b204e9800998ecf8427e", d.RowsMD5)
	assert.Equal(t, int64(0), d.Totals.RowCount())
	assert.True(t, d.Totals.SumStd().IsZero())
}

func TestComputeStdDigest_OrderIndependentAndOKOnly(t *testing.T) {
	a := digestRow("YRN1", "AX", "S1", DeriveOK, "1.23456")
	b := digestRow("YRN1", "B", "S1", DeriveOK, "2")
	c := digestRow("YRN0", "AX", "S1", DeriveNoRule, "")
	d1, err := ComputeStdDigest([]StdRow{a, b, c})
	require.NoError(t, err)
	d2, err := ComputeStdDigest([]StdRow{c, b, a})
	require.NoError(t, err)
	assert.True(t, d1.Equal(d2))
	assert.Equal(t, int64(2), d1.Totals.RowCount())
	assert.True(t, d1.Totals.SumStd().Equal(decimal.RequireFromString("3.23456")))

	want := MD5Hex("YRN1|AX|S1|GOAPPS_AX||Type 1|||1.23456||||1.23456|1.23456\n" +
		"YRN1|B|S1|GOAPPS_AX||Type 1|||2.00000||||2.00000|2.00000")
	assert.Equal(t, want, d1.RowsMD5)

	b.StdCost = decimal.NewNullDecimal(decimal.RequireFromString("2.00001"))
	d3, err := ComputeStdDigest([]StdRow{a, b, c})
	require.NoError(t, err)
	assert.NotEqual(t, d1.RowsMD5, d3.RowsMD5)
	assert.False(t, d1.Equal(d3))
}

func TestCanonicalStdLine_ByteOrderSort(t *testing.T) {
	// Upper case sorts before lower case byte-wise (PG COLLATE "C").
	assert.True(t, lessErpKey(ErpKey{ItemCode: "Z"}, ErpKey{ItemCode: "a"}))
	assert.True(t, lessErpKey(ErpKey{ItemCode: "A", GradeCode: "A"}, ErpKey{ItemCode: "A", GradeCode: "B"}))
	assert.True(t, lessErpKey(ErpKey{ItemCode: "A", GradeCode: "A", ShadeCode: ""}, ErpKey{ItemCode: "A", GradeCode: "A", ShadeCode: "1"}))
}
