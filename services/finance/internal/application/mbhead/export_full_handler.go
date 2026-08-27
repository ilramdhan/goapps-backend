package mbhead

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"
)

// DefaultExportCostType is the cost type the full-recipe export falls back to when the
// caller sends none. Pinning ONE cost type is deliberate: without it the row count would
// be n_composition × n_cost_type and the workbook would explode for a large filter.
const DefaultExportCostType = "ACTUAL"

// CheckStatusCalcNotComputedLabel is what a NULL mbh_check_status_calc renders as.
//
// 🔴 A NULL derived check status means "the application has never computed one for this
// head" (P10). It is a real, meaningful state — 207 legacy heads sit in it — so it is
// rendered EXPLICITLY here, exactly as the UI renders it, and ⛔ never as a blank cell
// that would be indistinguishable from a value the export failed to read.
const CheckStatusCalcNotComputedLabel = "Belum dihitung"

// recipeFullSheetName is the single sheet the full-recipe workbook carries.
const recipeFullSheetName = "MB Recipe Full"

// ExportFullCommand parameterizes the denormalized full-recipe export (P12, items C1 + C2).
type ExportFullCommand struct {
	// ActiveOnly nil means no active filter at all — absence stays absence (D13).
	ActiveOnly *bool
	// Period is YYYYMM, or empty for "latest active period per head".
	Period string
	// CostType is empty for DefaultExportCostType.
	CostType string
	// CheckStatusCalc filters on the DERIVED check status (mbh_check_status_calc, P10).
	//
	// Empty means ALL ROWS, NULL-status heads included — so an unset filter reproduces
	// the pre-filter export exactly. ⛔ Unlike CostType this is never defaulted: empty
	// is a legitimate choice, not a missing one.
	//
	// The six accepted values are the six allowed by chk_mbh_check_status_calc
	// (migration 000487). ⚠ DeriveCheckStatus produces only THREE of them today —
	// CheckStatusBoughtout, CheckStatusApproved, CheckStatusWaiting. Filtering by
	// "Current", "Outdated" or "Rejected" is VALID but yields ZERO ROWS until the
	// corresponding user gates are decided. ⛔ That is not a bug.
	//
	// ⛔ There is deliberately NO sentinel meaning "only the NULL heads" — that
	// semantic has not been decided by the user.
	CheckStatusCalc string
	// IncludeRejected forwards verbatim to RecipeFullFilter.IncludeRejected — false
	// (the zero value) EXCLUDES REJECTED heads from the export. See that field's doc
	// comment for the pending proto follow-up.
	IncludeRejected bool
}

// ExportFullHandler handles the ExportMBRecipeFull query.
//
// ⛔ READ-ONLY: it holds only a reader port and issues no writes. cst_mb_cost and
// cst_product_cost are never touched by this path.
type ExportFullHandler struct {
	reader RecipeFullReader
}

// NewExportFullHandler creates a new ExportFullHandler.
func NewExportFullHandler(reader RecipeFullReader) *ExportFullHandler {
	return &ExportFullHandler{reader: reader}
}

// recipeFullHeaders lists the full-export column headers in column order.
//
// ⚠ DELIBERATELY SEPARATE from mbHeadExportHeaders (export_handler.go), which doubles as
// the IMPORT TEMPLATE header. This export is a read-only report and carries derived,
// non-importable columns (Check Status Calc, MB Cost, traceability); merging the two
// lists would make those columns look importable. ⛔ Do not unify them.
var recipeFullHeaders = []string{
	// Recipe block
	"No", "MB Costing", "MB Name", "Code", "Dev No", "VS Number", "No of Process",
	"Shade Code", "Shade Name", "Shade 2 Code", "Shade 2 Name", "Shade 3 Code", "Shade 3 Name",
	"Denier", "Filament", "Cross Section", "Lusture", "LDR %", "Dozing",
	"Status", "Check Status (Legacy)", "Check Status (Calc)", "Final Product",
	"Boughtout", "Entry Status",
	// Composition block
	"Seq No", "Source Type", "RM Group Code", "MB Ref Costing", "Composition %", "Is Carrier",
	// MB Cost block (C2)
	"Cost Period", "Cost Type", "Cost Value", "Cost Pushed At",
	// Traceability block
	"Cost Product Sys ID", "Cost Product Code", "Cost Generated At",
}

// Handle executes the full-recipe export and returns the workbook bytes and file name.
func (h *ExportFullHandler) Handle(ctx context.Context, cmd ExportFullCommand) (content []byte, fileName string, err error) {
	costType := cmd.CostType
	if costType == "" {
		costType = DefaultExportCostType
	}

	rows, err := h.reader.ListRecipeFullRows(ctx, RecipeFullFilter{
		IsActive: cmd.ActiveOnly,
		Period:   cmd.Period,
		CostType: costType,
		// Forwarded VERBATIM — empty stays empty, which the reader treats as "no filter".
		CheckStatusCalc: cmd.CheckStatusCalc,
		IncludeRejected: cmd.IncludeRejected,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to read mb recipe full rows: %w", err)
	}

	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close full-recipe Excel file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	if setupErr := setupRecipeFullSheet(f); setupErr != nil {
		return nil, "", setupErr
	}

	writer := &excelWriter{f: f, sheetName: recipeFullSheetName}
	for i := range rows {
		writeRecipeFullRow(writer, i+2, i+1, &rows[i])
	}
	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("Some full-recipe Excel cell writes failed")
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, "", fmt.Errorf("failed to write full-recipe excel to buffer: %w", err)
	}
	return buffer.Bytes(), "mb_recipe_full_export.xlsx", nil
}

// setupRecipeFullSheet creates the sheet and writes the styled header row.
func setupRecipeFullSheet(f *excelize.File) error {
	index, err := f.NewSheet(recipeFullSheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)
	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	for col, header := range recipeFullHeaders {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, 1)
		if cellErr != nil {
			return fmt.Errorf("failed to get cell name: %w", cellErr)
		}
		if setErr := f.SetCellValue(recipeFullSheetName, cell, header); setErr != nil {
			return fmt.Errorf("failed to set header %s: %w", header, setErr)
		}
	}

	lastCol, err := excelize.CoordinatesToCellName(len(recipeFullHeaders), 1)
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
	if err := f.SetCellStyle(recipeFullSheetName, "A1", lastCol+"1", style); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}
	return nil
}

// optBool renders a nullable boolean: nil stays an empty cell, never a coerced false (D13).
func optBool(v *bool) any {
	if v == nil {
		return ""
	}
	return *v
}

// optInt32 renders a nullable int32: nil stays an empty cell, never a coerced 0 (D13).
func optInt32(v *int32) any {
	if v == nil {
		return ""
	}
	return *v
}

// optSysID renders mbh_cost_product_id. 0 means "no cost product generated" — that is the
// column's own sentinel (it arrives as 0 from a NULL), so it is shown as blank rather than
// as a misleading product id of zero.
func optSysID(v int64) any {
	if v == 0 {
		return ""
	}
	return v
}

// checkStatusCalcCell renders the derived check status, making the NULL state explicit.
func checkStatusCalcCell(v *string) any {
	if v == nil {
		return CheckStatusCalcNotComputedLabel
	}
	return *v
}

// writeRecipeFullRow writes one denormalized row. Column order MUST track recipeFullHeaders.
func writeRecipeFullRow(w *excelWriter, row, idx int, r *RecipeFullRow) {
	values := []any{
		idx,
		r.MBCosting,
		optStr(r.MgtName),
		optStr(r.Code),
		r.DevCode,
		optStr(r.VSNumber),
		optStr(r.NoOfProcess),
		r.ShadeCode,
		r.ShadeName,
		optStr(r.Shade2Code),
		optStr(r.Shade2Name),
		optStr(r.Shade3Code),
		optStr(r.Shade3Name),
		optFloat(r.Denier),
		optInt(r.Filament),
		r.CrossSection,
		r.LustureCode,
		optFloat(r.LdrPct),
		optFloat(r.Dozing),
		optStr(r.Status),
		optStr(r.CheckStatusLegacy),
		checkStatusCalcCell(r.CheckStatusCalc),
		optStr(r.FinalProduct),
		r.IsBoughtout,
		r.EntryStatus,
		optInt32(r.CompSeqNo),
		optStr(r.CompSourceType),
		optStr(r.CompRMGroupCode),
		optStr(r.CompMBRefCosting),
		optStr(r.CompPct),
		optBool(r.CompIsCarrier),
		optStr(r.CostPeriod),
		optStr(r.CostType),
		optStr(r.CostValue),
		optStr(r.CostPushedAt),
		optSysID(r.CostProductSysID),
		optStr(r.CostProductCode),
		optStr(r.CostGeneratedAt),
	}
	for col, v := range values {
		cell, err := excelize.CoordinatesToCellName(col+1, row)
		if err != nil {
			continue
		}
		w.setCellValue(cell, v)
	}
}
