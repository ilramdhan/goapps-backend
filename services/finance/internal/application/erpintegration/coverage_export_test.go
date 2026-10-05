package erpintegration

import (
	"bytes"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

func TestExportCoverage(t *testing.T) {
	psid := int64(7)
	lines := []domain.CoverageLine{
		{BatchID: 5, Kind: "YARN", ItemCode: "A1", ShadeCode: "S", GradeCodes: []string{"AX", "AY"}, ProductSysID: &psid, Status: "MATCHED", QtyKg: decimal.RequireFromString("12.5")},
		{BatchID: 5, Kind: "YARN", ItemCode: "B2", Status: "UNMAPPED", Reason: "no link", QtyKg: decimal.Zero},
	}
	content, name, err := ExportCoverage("202607", 5, lines)
	if err != nil {
		t.Fatal(err)
	}
	if name != "erp_coverage_202607_5.xlsx" {
		t.Fatalf("name = %s", name)
	}
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows(SheetErpCoverage)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0][0] != "Batch ID" || rows[0][len(ErpCoverageHeaders)-1] != "Candidates" {
		t.Fatalf("bad header %v", rows[0])
	}
	if rows[1][2] != "A1" || rows[1][4] != "AX,AY" {
		t.Fatalf("bad row %v", rows[1])
	}
}
