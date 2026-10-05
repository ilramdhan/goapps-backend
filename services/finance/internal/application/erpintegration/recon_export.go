package erpintegration

import (
	"fmt"
	"strconv"

	"github.com/xuri/excelize/v2"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// Recon export sheet names.
const (
	SheetReconSummary = "Summary"
	SheetReconRows    = "Rows"
)

// ReconRowHeaders are the Rows-sheet columns.
var ReconRowHeaders = []string{
	"item", "grade", "shade", "basis", "goapps std_cost", "erp_rate", "rate_variants", "qty_kg", "value", "status", "flex13",
}

// ExportRecon renders the recon outcome of a batch as .xlsx bytes and the
// file name erp_recon_<period>_<batchID>.xlsx. DIFF rows are highlighted.
//
//nolint:gocognit,gocyclo // linear workbook assembly
func ExportRecon(period string, batchID int64, rows []domain.ReconExportRow) (content []byte, fileName string, err error) {
	f := excelize.NewFile()
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			content, fileName, err = nil, "", fmt.Errorf("recon export: close workbook: %w", cerr)
		}
	}()
	for _, s := range []string{SheetReconSummary, SheetReconRows} {
		if _, err = f.NewSheet(s); err != nil {
			return nil, "", fmt.Errorf("recon export: new sheet: %w", err)
		}
	}
	counts := map[domain.ReconStatus]int{}
	for i := range rows {
		counts[rows[i].Status]++
	}
	sum := [][]any{
		{"status", "count"},
		{string(domain.ReconMatch), counts[domain.ReconMatch]},
		{string(domain.ReconDiff), counts[domain.ReconDiff]},
		{string(domain.ReconNotInAdj), counts[domain.ReconNotInAdj]},
		{"TOTAL", len(rows)},
	}
	for i := range sum {
		if err = f.SetSheetRow(SheetReconSummary, "A"+strconv.Itoa(i+1), &sum[i]); err != nil {
			return nil, "", fmt.Errorf("recon export: summary: %w", err)
		}
	}
	headers := ReconRowHeaders
	if err = f.SetSheetRow(SheetReconRows, "A1", &headers); err != nil {
		return nil, "", fmt.Errorf("recon export: header: %w", err)
	}
	diffStyle, err := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"#FFC7CE"}, Pattern: 1}})
	if err != nil {
		return nil, "", fmt.Errorf("recon export: style: %w", err)
	}
	for i := range rows {
		r := rows[i]
		n := i + 2
		variants := ""
		if r.RateVariants != nil {
			variants = strconv.FormatInt(*r.RateVariants, 10)
		}
		row := []any{
			r.Key.ItemCode, r.Key.GradeCode, r.Key.ShadeCode, r.Basis, fixed5(r.StdCost),
			fixed5(r.ErpRate), variants, fixed5(r.QtyKg), fixed5(r.Value), string(r.Status), r.Flex13,
		}
		if err = f.SetSheetRow(SheetReconRows, "A"+strconv.Itoa(n), &row); err != nil {
			return nil, "", fmt.Errorf("recon export: row %d: %w", n, err)
		}
		if r.Status == domain.ReconDiff {
			if err = f.SetCellStyle(SheetReconRows, "A"+strconv.Itoa(n), "K"+strconv.Itoa(n), diffStyle); err != nil {
				return nil, "", fmt.Errorf("recon export: style row %d: %w", n, err)
			}
		}
	}
	if idx, ierr := f.GetSheetIndex(SheetReconSummary); ierr == nil {
		f.SetActiveSheet(idx)
	}
	_ = f.DeleteSheet("Sheet1") //nolint:errcheck // default sheet may already be absent
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, "", fmt.Errorf("recon export: write workbook: %w", err)
	}
	return buf.Bytes(), fmt.Sprintf("erp_recon_%s_%d.xlsx", period, batchID), nil
}
