package erpintegration

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

func p9StdRows() []domain.StdRow {
	var rows []domain.StdRow
	for i := 0; i < 40; i++ {
		basis := []erprule.Basis{erprule.Basis("COST"), erprule.Basis("SPPTY"), erprule.Basis("SPITY"), erprule.Basis("SPBSD")}[i%4]
		rows = append(rows, domain.StdRow{
			Key:    domain.ErpKey{ItemCode: fmt.Sprintf("I%03d", i), GradeCode: "A", ShadeCode: "N"},
			Status: domain.DeriveOK, Basis: basis,
			StdCost: decimal.NullDecimal{Decimal: decimal.RequireFromString("1.5"), Valid: true},
		})
	}
	return rows
}

func TestSelectManualSample(t *testing.T) {
	rows := p9StdRows()
	a := SelectManualSample(rows, 30, 60)
	b := SelectManualSample(rows, 30, 60)
	require.Equal(t, a, b)
	require.GreaterOrEqual(t, len(a), 30)
	seen := map[erprule.Basis]bool{}
	for i := range a {
		seen[a[i].Basis] = true
	}
	require.Len(t, seen, 4)
}

func TestExportManualSampleAndRecon(t *testing.T) {
	content, name, err := ExportManualSample("202601", 7, SelectManualSample(p9StdRows(), 30, 60))
	require.NoError(t, err)
	require.Equal(t, "erp_manual_sample_202601_7.xlsx", name)
	f, err := excelize.OpenReader(bytes.NewReader(content))
	require.NoError(t, err)
	rs, err := f.GetRows(SheetManualSample)
	require.NoError(t, err)
	require.Equal(t, ManualSampleHeaders[:3], rs[0][:3])
	require.GreaterOrEqual(t, len(rs), 31)

	rr := []domain.ReconExportRow{
		{ReconRow: domain.ReconRow{Key: domain.ErpKey{ItemCode: "X", GradeCode: "A", ShadeCode: "N"}, Status: domain.ReconDiff}},
		{ReconRow: domain.ReconRow{Key: domain.ErpKey{ItemCode: "Y", GradeCode: "A", ShadeCode: "N"}, Status: domain.ReconMatch}},
	}
	content, name, err = ExportRecon("202601", 7, rr)
	require.NoError(t, err)
	require.Equal(t, "erp_recon_202601_7.xlsx", name)
	f, err = excelize.OpenReader(bytes.NewReader(content))
	require.NoError(t, err)
	rs, err = f.GetRows(SheetReconRows)
	require.NoError(t, err)
	require.Len(t, rs, 3)
	require.Equal(t, "status", rs[0][9])
}
