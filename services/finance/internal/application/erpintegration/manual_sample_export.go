package erpintegration

import (
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// Manual-sample sheet names and selection bounds (recon section 11).
const (
	SheetManualSample       = "Sample"
	SheetManualInstructions = "Instructions"
	ManualSampleMin         = 30
	ManualSampleMax         = 60
	manualSamplePerStratum  = 5
)

// ManualSampleHeaders are the export columns; the last three are for Finance input.
var ManualSampleHeaders = []string{
	"item", "grade", "shade", "prod_type", "fg_type", "grade_group", "basis", "ms_batch_item",
	"chp_cost", "chp_con_kg", "ax_conv_cost", "value_loss", "selling_price", "conv_cost",
	"ax_cost", "std_cost", "prod_value_loss", "ax_cost_sys_id", "ax_cost_version",
	"manual_ax_cost", "manual_std", "manual_note",
}

const manualSampleRule = "ROUND(manual_ax_cost,5) = ax_cost AND ROUND(manual_std,5) = std_cost for every row"

// SelectManualSample deterministically picks up to perStratum rows per
// (basis x prod_type x hasMB) stratum from the OK rows ordered by key, tops
// up to min from the remaining rows, and caps at max.
func SelectManualSample(rows []domain.StdRow, minRows, maxRows int) []domain.StdRow {
	ok := make([]domain.StdRow, 0, len(rows))
	for i := range rows {
		if rows[i].Status == domain.DeriveOK {
			ok = append(ok, rows[i])
		}
	}
	sort.SliceStable(ok, func(i, j int) bool { return ok[i].Key.String() < ok[j].Key.String() })
	counts := map[string]int{}
	picked := make([]bool, len(ok))
	out := make([]domain.StdRow, 0, maxRows)
	for i := range ok {
		if len(out) >= maxRows {
			break
		}
		k := fmt.Sprintf("%s|%s|%t", ok[i].Basis, ok[i].ProdType, ok[i].MsBatchItem != "")
		if counts[k] >= manualSamplePerStratum {
			continue
		}
		counts[k]++
		picked[i] = true
		out = append(out, ok[i])
	}
	for i := range ok {
		if len(out) >= minRows {
			break
		}
		if !picked[i] {
			out = append(out, ok[i])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key.String() < out[j].Key.String() })
	return out
}

func fixed5(d decimal.NullDecimal) string {
	if !d.Valid {
		return ""
	}
	return d.Decimal.StringFixed(5)
}

// ExportManualSample renders the sample rows as .xlsx bytes and the file
// name erp_manual_sample_<period>_<batchID>.xlsx.
func ExportManualSample(period string, batchID int64, rows []domain.StdRow) (content []byte, fileName string, err error) {
	f := excelize.NewFile()
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			content, fileName, err = nil, "", fmt.Errorf("manual sample export: close workbook: %w", cerr)
		}
	}()
	for _, s := range []string{SheetManualSample, SheetManualInstructions} {
		if _, err = f.NewSheet(s); err != nil {
			return nil, "", fmt.Errorf("manual sample export: new sheet: %w", err)
		}
	}
	headers := ManualSampleHeaders
	if err = f.SetSheetRow(SheetManualSample, "A1", &headers); err != nil {
		return nil, "", fmt.Errorf("manual sample export: header: %w", err)
	}
	for i := range rows {
		r := rows[i]
		cell, cerr := excelize.CoordinatesToCellName(1, i+2)
		if cerr != nil {
			return nil, "", fmt.Errorf("manual sample export: cell: %w", cerr)
		}
		row := []any{
			r.Key.ItemCode, r.Key.GradeCode, r.Key.ShadeCode, string(r.ProdType), r.FgType, string(r.GradeGroup),
			string(r.Basis), r.MsBatchItem,
			fixed5(r.ChpCost), fixed5(r.ChpConKg), fixed5(r.AxConvCost), fixed5(r.ValueLoss), fixed5(r.SellingPrice),
			fixed5(r.ConvCost), fixed5(r.AxCost), fixed5(r.StdCost), fixed5(r.ProdValLoss),
			optInt64(r.AxCostSysID), optInt32(r.AxCostVersion), "", "", "",
		}
		if err = f.SetSheetRow(SheetManualSample, cell, &row); err != nil {
			return nil, "", fmt.Errorf("manual sample export: row %d: %w", i+2, err)
		}
	}
	instr := []any{"Pass rule", manualSampleRule}
	if err = f.SetSheetRow(SheetManualInstructions, "A1", &instr); err != nil {
		return nil, "", fmt.Errorf("manual sample export: instructions: %w", err)
	}
	if idx, ierr := f.GetSheetIndex(SheetManualSample); ierr == nil {
		f.SetActiveSheet(idx)
	}
	_ = f.DeleteSheet("Sheet1") //nolint:errcheck // default sheet may already be absent
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, "", fmt.Errorf("manual sample export: write workbook: %w", err)
	}
	return buf.Bytes(), fmt.Sprintf("erp_manual_sample_%s_%d.xlsx", period, batchID), nil
}
