package erpintegration

import (
	"bytes"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBacktestReport_WriteCSV(t *testing.T) {
	d := func(s string) decimal.NullDecimal {
		return decimal.NullDecimal{Decimal: decimal.RequireFromString(s), Valid: true}
	}
	rep := BacktestReport{Lines: []BacktestLine{
		{Key: ErpKey{ItemCode: "POY1", GradeCode: "AX", ShadeCode: "NL"}, Class: BacktestDiff, Basis: "SP1",
			GoApps: d("1.5"), Legacy: d("1.25"), Delta: d("0.25"), DeltaPct: d("20"), Fail: true},
		{Key: ErpKey{ItemCode: "P,2"}, Class: BacktestOnlyLegacy, Legacy: d("2")},
	}}
	var buf bytes.Buffer
	require.NoError(t, rep.WriteCSV(&buf, "202608"))
	assert.Equal(t, "period,item,grade,shade,class,basis,goapps,legacy,delta,delta_pct,fail\n"+
		"202608,POY1,AX,NL,DIFF,SP1,1.5,1.25,0.25,20,true\n"+
		"202608,\"P,2\",,,ONLY_LEGACY,,,2,,,false\n", buf.String())
}
