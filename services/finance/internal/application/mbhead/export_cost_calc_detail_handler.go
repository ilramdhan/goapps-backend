package mbhead

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"
)

// DefaultCostCalcDetailType is the calculation type the calc-detail dump falls back to
// when the caller sends none. Pinning ONE type is deliberate: without it the row count
// would be n_rm_lines × n_calc_type and the workbook would explode.
const DefaultCostCalcDetailType = "ACTUAL"

// costCalcDetailSheetName is the single sheet the calc-detail workbook carries.
const costCalcDetailSheetName = "MB Cost Calc Detail"

// ExportCostCalcDetailCommand parameterizes the flat MB cost-calculation detail dump.
type ExportCostCalcDetailCommand struct {
	// ActiveOnly nil means no active filter at all — absence stays absence (D13).
	ActiveOnly *bool
	// Period is YYYYMM, or empty for "latest calculated period per head".
	Period string
	// CalculationType is empty for DefaultCostCalcDetailType.
	CalculationType string
	// CalcStatus filters cst_product_cost.cpc_status. Empty means no status filter —
	// ⛔ never defaulted, absence is a legitimate choice.
	CalcStatus string
	// IncludeRejected false (the zero value) EXCLUDES REJECTED heads.
	IncludeRejected bool
}

// ExportCostCalcDetailHandler handles the ExportMBCostCalcDetail query.
//
// ⛔ READ-ONLY: it holds only a reader port and issues no writes.
//
// ⛔ DELIBERATELY SEPARATE from ExportFullHandler. That handler owns the 37-column
// recipe+cost report, which must stay byte-for-byte unchanged. This one is a different
// report at a different grain (RM lines from the persisted COST SNAPSHOT, not
// composition rows) with a different, snake_case column set. ⛔ Do not unify them.
type ExportCostCalcDetailHandler struct {
	reader CostCalcDetailReader
}

// NewExportCostCalcDetailHandler creates a new ExportCostCalcDetailHandler.
func NewExportCostCalcDetailHandler(reader CostCalcDetailReader) *ExportCostCalcDetailHandler {
	return &ExportCostCalcDetailHandler{reader: reader}
}

// costCalcDetailHeaders lists the 29 calc-dump column headers in column order.
//
// ⚠ These are snake_case ON PURPOSE — this workbook is a machine-readable calc dump
// consumed by downstream tooling that keys on these exact names, not a human-facing
// report. ⛔ Do not "tidy" them into Title Case, and do not reuse recipeFullHeaders.
var costCalcDetailHeaders = []string{
	// MB identity
	"mb_code", "mb_name",
	// MB frozen input params
	"total_fix", "pct_waste", "pct_quality_loss", "pct_efficiency", "dev_expense",
	"packing", "prod_per_day", "throughput_per_hr", "no_process",
	// MB calc results
	"mb_net_prod", "mb_waste_val", "mb_fixed_total", "mb_cost_others", "mb_rm_cost",
	"mb_conv_cost", "mb_total_cost", "mb_cost_per_unit",
	// Cost snapshot traceability
	"calc_version", "calc_status", "row_no",
	// RM line
	"rm_type", "rm_ref", "rm_group_name", "komposisi",
	"rm_rate_actual", "rm_rate_tier", "cost_actual",
}

// Handle executes the calc-detail dump and returns the workbook bytes and file name.
//
// ⛔ NO WORKBOOK PASSWORD: the output is deliberately plaintext.
func (h *ExportCostCalcDetailHandler) Handle(
	ctx context.Context, cmd ExportCostCalcDetailCommand,
) (content []byte, fileName string, err error) {
	calcType := cmd.CalculationType
	if calcType == "" {
		calcType = DefaultCostCalcDetailType
	}

	rows, err := h.reader.ListCostCalcDetailRows(ctx, CostCalcDetailFilter{
		IsActive:        cmd.ActiveOnly,
		Period:          cmd.Period,
		CalculationType: calcType,
		// Forwarded VERBATIM — empty stays empty, which the reader treats as "no filter".
		CalcStatus:      cmd.CalcStatus,
		IncludeRejected: cmd.IncludeRejected,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to read mb cost calc detail rows: %w", err)
	}

	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close cost-calc-detail Excel file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	if setupErr := setupCostCalcDetailSheet(f); setupErr != nil {
		return nil, "", setupErr
	}

	writer := &excelWriter{f: f, sheetName: costCalcDetailSheetName}
	for i := range rows {
		writeCostCalcDetailRow(writer, i+2, &rows[i])
	}
	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("Some cost-calc-detail Excel cell writes failed")
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, "", fmt.Errorf("failed to write cost-calc-detail excel to buffer: %w", err)
	}
	return buffer.Bytes(), "mb_cost_calc_detail_export.xlsx", nil
}

// setupCostCalcDetailSheet creates the sheet and writes the styled header row.
func setupCostCalcDetailSheet(f *excelize.File) error {
	index, err := f.NewSheet(costCalcDetailSheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)
	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	for col, header := range costCalcDetailHeaders {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, 1)
		if cellErr != nil {
			return fmt.Errorf("failed to get cell name: %w", cellErr)
		}
		if setErr := f.SetCellValue(costCalcDetailSheetName, cell, header); setErr != nil {
			return fmt.Errorf("failed to set header %s: %w", header, setErr)
		}
	}

	lastCol, err := excelize.CoordinatesToCellName(len(costCalcDetailHeaders), 1)
	if err != nil {
		return fmt.Errorf("failed to get last header cell name: %w", err)
	}
	style, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return fmt.Errorf("failed to create header style: %w", err)
	}
	if err := f.SetCellStyle(costCalcDetailSheetName, "A1", lastCol+"1", style); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}
	return nil
}

// writeCostCalcDetailRow writes one calc-dump row. Column order MUST track
// costCalcDetailHeaders.
func writeCostCalcDetailRow(w *excelWriter, row int, r *CostCalcDetailRow) {
	values := []any{
		r.MBCode,
		optStr(r.MBName),
		optFloat(r.TotalFix),
		optFloat(r.PctWaste),
		optFloat(r.PctQualityLoss),
		optFloat(r.PctEfficiency),
		optFloat(r.DevExpense),
		optFloat(r.Packing),
		optFloat(r.ProdPerDay),
		optFloat(r.ThroughputPerHr),
		optFloat(r.NoProcess),
		optFloat(r.MBNetProd),
		optFloat(r.MBWasteVal),
		optFloat(r.MBFixedTotal),
		optFloat(r.MBCostOthers),
		optFloat(r.MBRMCost),
		optFloat(r.MBConvCost),
		optFloat(r.MBTotalCost),
		optFloat(r.MBCostPerUnit),
		r.CalcVersion,
		r.CalcStatus,
		r.RowNo,
		r.RMType,
		r.RMRef,
		optStr(r.RMGroupName),
		optFloat(r.Komposisi),
		optFloat(r.RMRateActual),
		// Empty tier is a REAL value for PRODUCT-type rows (no cst_rm_cost row exists),
		// so it renders as a blank cell without any placeholder text.
		r.RMRateTier,
		optFloat(r.CostActual),
	}
	for col, v := range values {
		cell, err := excelize.CoordinatesToCellName(col+1, row)
		if err != nil {
			continue
		}
		w.setCellValue(cell, v)
	}
}
