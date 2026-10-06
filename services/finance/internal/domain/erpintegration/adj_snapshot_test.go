package erpintegration

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snapND(s string) decimal.NullDecimal {
	return decimal.NewNullDecimal(decimal.RequireFromString(s))
}

func snapRow(head, item int64, txn string) AdjSnapshotRow {
	return AdjSnapshotRow{
		HeadSysID: head, ItemSysID: item, TxnCode: txn,
		ItemCode: "POY001", GradeCode: "A", ShadeCode: "S1",
		QtyBu: snapND("2500"), Rate: snapND("1.5"), Val: snapND("3.75"),
	}
}

func TestAdjSnapshotRow_RowHashCanonical(t *testing.T) {
	a := snapRow(1, 11, TxnInvAdj)
	b := a
	b.Rate = snapND("1.50000")
	assert.Equal(t, a.RowHash(), b.RowHash(), "trailing zeros do not change the hash")

	b.NewRate, b.NewVal = snapND("9"), snapND("9")
	assert.Equal(t, a.RowHash(), b.RowHash(), "projected values are not hashed")

	c := a
	c.Rate = snapND("1.6")
	assert.NotEqual(t, a.RowHash(), c.RowHash())
	f := "x"
	d := a
	d.Flex[2] = &f
	assert.NotEqual(t, a.RowHash(), d.RowHash())
	p := "P"
	e := a
	e.HeadPostStatus = &p
	assert.NotEqual(t, a.RowHash(), e.RowHash(), "head eligibility is hashed")
	n := a
	n.Rate = decimal.NullDecimal{}
	assert.NotEqual(t, a.RowHash(), n.RowHash(), "NULL differs from a value")
}

func TestAdjSetHash_OrderIndependent(t *testing.T) {
	r1, r2 := snapRow(1, 11, TxnInvAdj), snapRow(2, 21, TxnMbInvAdj)
	assert.Equal(t, AdjSetHash([]AdjSnapshotRow{r1, r2}), AdjSetHash([]AdjSnapshotRow{r2, r1}))
	assert.NotEqual(t, AdjSetHash([]AdjSnapshotRow{r1}), AdjSetHash([]AdjSnapshotRow{r1, r2}))
	assert.Len(t, AdjSetHash(nil), 64)
}

func TestSplitAdjSnapshot_DenyList(t *testing.T) {
	p := "P"
	ap, one := int64(3), int64(1)
	posted := snapRow(2, 21, TxnInvAdj)
	posted.HeadPostStatus = &p
	postedAppr := snapRow(2, 22, TxnInvAdj)
	postedAppr.HeadPostStatus, postedAppr.HeadApprStatus = &p, &ap
	appr := snapRow(3, 31, TxnMbInvAdjRp)
	appr.HeadApprStatus = &ap
	notFinal := snapRow(4, 41, TxnMbInvAdjRp)
	notFinal.HeadApprStatus = &one
	other := snapRow(5, 51, "GRN")

	elig, ex := SplitAdjSnapshot([]AdjSnapshotRow{snapRow(1, 11, TxnInvAdj), posted, postedAppr, appr, notFinal, other})
	require.Len(t, elig, 2)
	assert.Equal(t, int64(11), elig[0].ItemSysID)
	assert.Equal(t, int64(41), elig[1].ItemSysID, "APPR != 3 stays eligible")
	assert.Equal(t, AdjExclusions{PostedHeads: 1, PostedItems: 2, ApprovedHeads: 1, ApprovedItems: 1, OtherTxnItems: 1}, ex)
}

func TestProjectAdjRow(t *testing.T) {
	r := snapRow(1, 11, TxnInvAdj)
	same, err := ProjectAdjRow(r, nil, 7)
	require.NoError(t, err)
	assert.False(t, same.IsProjected())

	notOK := StdRow{Status: DeriveOK}
	same, err = ProjectAdjRow(r, &notOK, 7)
	require.NoError(t, err)
	assert.False(t, same.IsProjected(), "std without a cost is not valued")

	std := StdRow{
		Key: r.Key(), Status: DeriveOK, StdCost: snapND("2.4"),
		ConvCost: snapND("0.5"), ChpItemCode: "CHP1", FgType: "FG", Source: StdSource("MB"),
	}
	got, err := ProjectAdjRow(r, &std, 7)
	require.NoError(t, err)
	require.True(t, got.IsProjected())
	assert.True(t, got.NewRate.Decimal.Equal(decimal.RequireFromString("2.4")))
	assert.True(t, got.NewVal.Decimal.Equal(decimal.RequireFromString("6")), got.NewVal.Decimal.String())
	require.NotNil(t, got.NewFlex)
	assert.Equal(t, "CHP1", got.NewFlex[3])
	assert.Equal(t, "FG", got.NewFlex[4])
	assert.Equal(t, "7", got.NewFlex[12])
	assert.Equal(t, "MB", got.NewFlex[13])
	assert.Equal(t, r.RowHash(), got.RowHash(), "projection leaves the current columns intact")

	m := StdRowsByKey([]StdRow{std, {Key: ErpKey{ItemCode: "X"}, Status: DeriveOK}})
	assert.Len(t, m, 1)
	assert.Equal(t, "202608/7", PreviewConfirmText("202608", 7))
}
