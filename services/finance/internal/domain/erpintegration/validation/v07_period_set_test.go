package validation

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

func item(id int64, code, grade, shade, rate string) AdjSetItem {
	it := AdjSetItem{ItemSysID: id, ItemCode: code, GradeCode: grade, ShadeCode: shade}
	if rate != "" {
		it.CurrentRate = nd(rate)
	}
	return it
}

func TestPrevalidatePeriodSet_Pass(t *testing.T) {
	heads := []AdjSetHead{
		{HeadSysID: 1, TxnCode: "INVADJ", Items: []AdjSetItem{item(10, "POY1", "AX", "N", "1.5"), item(11, "HOY1", "B", "N", "20")}},
		{HeadSysID: 2, TxnCode: "MBINVADJ", Items: []AdjSetItem{item(20, "CMB1", "A", "R", "500"), item(21, "XYZ1", "A", "R", "")}},
	}
	res := PrevalidatePeriodSet("202608", heads)
	assert.True(t, res.OK())
	assert.Equal(t, 2, res.HeadCount)
	assert.Equal(t, 4, res.ItemCount)
	assert.Empty(t, res.OffendingHeadIDs())
	assert.Empty(t, res.Findings())
}

func TestPrevalidatePeriodSet_ReturnsEveryOffendingHead(t *testing.T) {
	heads := []AdjSetHead{
		{HeadSysID: 9, TxnCode: "INVADJ", Items: []AdjSetItem{item(92, "TTY1", "AX", "N", "0"), item(91, "MMK1", "AX", "N", "21")}},
		{HeadSysID: 3, TxnCode: "INVADJ", Items: []AdjSetItem{item(30, "POY1", "AX", "N", "1")}},
		{HeadSysID: 5, TxnCode: "INVADJ", Items: []AdjSetItem{item(50, "PTY1", "B", "N", "")}},
	}
	res := PrevalidatePeriodSet("202608", heads)
	require.False(t, res.OK())
	assert.Equal(t, []int64{5, 9}, res.OffendingHeadIDs())
	assert.Equal(t, []int64{91, 92}, []int64{res.Offending[1].Items[0].ItemSysID, res.Offending[1].Items[1].ItemSysID})
	fs := res.Findings()
	require.Len(t, fs, 3)
	for _, f := range fs {
		assert.Equal(t, CodeV07, f.Code)
		assert.Equal(t, ScopePeriodSet, f.Scope)
		assert.Equal(t, SeverityError, f.Severity)
	}
	assert.Contains(t, fs[0].Message, "rate NULL")
	assert.Equal(t, int64(5), fs[0].HeadSysID)
}

func TestProjectRates_UsesBatchStd(t *testing.T) {
	heads := []AdjSetHead{{HeadSysID: 1, Items: []AdjSetItem{
		item(1, " POY1 ", "AX", "N", "50"), // current bad, projected good
		item(2, "POY1", "B", "N", "1"),     // current good, projected bad
		item(3, "POY2", "AX", "N", "25"),   // no std row: current stays
	}}}
	rows := []erpintegration.StdRow{
		axRow("POY1", "N", "1.6", 1),
		derivedRow("POY1", "B", "N", "21", 1),
		{Key: key("POY2", "AX", "N"), Status: erpintegration.DeriveNoAX},
	}
	proj := ProjectRates(heads, rows)
	assert.False(t, heads[0].Items[0].ProjectedRate.Valid, "input not modified")
	assert.True(t, proj[0].Items[0].EffectiveRate().Decimal.Equal(decimal.RequireFromString("1.6")))
	res := PrevalidatePeriodSet("202608", proj)
	require.Len(t, res.Offending, 1)
	ids := []int64{}
	for _, it := range res.Offending[0].Items {
		ids = append(ids, it.ItemSysID)
	}
	assert.Equal(t, []int64{2, 3}, ids)
}
