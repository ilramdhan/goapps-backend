package erpintegration

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// SheetErpCoverage is the single worksheet of the coverage export.
const SheetErpCoverage = "Coverage"

// ErpCoverageHeaders are the export columns (every CoverageLine field).
var ErpCoverageHeaders = []string{
	"Batch ID", "Kind", "Item Code", "Shade Code", "Grade Codes", "Product Sys ID",
	"Cost ID", "Cost Version", "Status", "Reason", "Qty Kg", "Candidates",
}

// ExportCoverage renders the coverage lines of a batch as .xlsx bytes and the
// suggested file name erp_coverage_<period>_<batchID>.xlsx.
func ExportCoverage(period string, batchID int64, lines []domain.CoverageLine) (content []byte, fileName string, err error) {
	f := excelize.NewFile()
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			content, fileName, err = nil, "", fmt.Errorf("coverage export: close workbook: %w", cerr)
		}
	}()
	if _, err = f.NewSheet(SheetErpCoverage); err != nil {
		return nil, "", fmt.Errorf("coverage export: new sheet: %w", err)
	}
	headers := ErpCoverageHeaders
	if err = f.SetSheetRow(SheetErpCoverage, "A1", &headers); err != nil {
		return nil, "", fmt.Errorf("coverage export: header: %w", err)
	}
	for i := range lines {
		l := lines[i]
		cell, cerr := excelize.CoordinatesToCellName(1, i+2)
		if cerr != nil {
			return nil, "", fmt.Errorf("coverage export: cell: %w", cerr)
		}
		row := []any{
			l.BatchID, string(l.Kind), l.ItemCode, l.ShadeCode, strings.Join(l.GradeCodes, ","),
			optInt64(l.ProductSysID), optInt64(l.CostID), optInt32(l.CostVersion),
			string(l.Status), l.Reason, l.QtyKg.String(), string(l.Candidates),
		}
		if err = f.SetSheetRow(SheetErpCoverage, cell, &row); err != nil {
			return nil, "", fmt.Errorf("coverage export: row %d: %w", i+2, err)
		}
	}
	if idx, ierr := f.GetSheetIndex(SheetErpCoverage); ierr == nil {
		f.SetActiveSheet(idx)
	}
	_ = f.DeleteSheet("Sheet1") //nolint:errcheck // default sheet may already be absent
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, "", fmt.Errorf("coverage export: write workbook: %w", err)
	}
	return buf.Bytes(), fmt.Sprintf("erp_coverage_%s_%d.xlsx", period, batchID), nil
}
