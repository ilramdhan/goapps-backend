// Package formula provides application layer handlers for Formula operations.
package formula

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"
)

// TemplateResult represents the download template result.
type TemplateResult struct {
	FileContent []byte
	FileName    string
}

// TemplateHandler handles the DownloadFormulaTemplate query.
type TemplateHandler struct{}

// NewTemplateHandler creates a new TemplateHandler.
func NewTemplateHandler() *TemplateHandler {
	return &TemplateHandler{}
}

// Handle generates the import template Excel file.
func (h *TemplateHandler) Handle() (result *TemplateResult, err error) {
	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close Excel template file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	sheetName := "Formula Import Template"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	if err := writeFormulaTemplateHeaders(f, sheetName); err != nil {
		return nil, err
	}

	writer := &formulaTemplateWriter{f: f, sheetName: sheetName}
	writeFormulaSampleData(writer)
	writeFormulaTemplateWidths(writer)

	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("Some Excel formatting operations failed")
	}

	writeFormulaInstructionsSheet(f)

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &TemplateResult{
		FileContent: buffer.Bytes(),
		FileName:    "formula_import_template.xlsx",
	}, nil
}

func writeFormulaTemplateHeaders(f *excelize.File, sheetName string) error {
	headers := []string{"Code", "Name", "Type", "Expression", "Result Param Code", "Input Param Codes", "Description"}
	for col, header := range headers {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, 1)
		if cellErr != nil {
			return fmt.Errorf("failed to get cell name: %w", cellErr)
		}
		if cellErr := f.SetCellValue(sheetName, cell, header); cellErr != nil {
			return fmt.Errorf("failed to set header %s: %w", header, cellErr)
		}
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return fmt.Errorf("failed to create header style: %w", err)
	}
	if err := f.SetCellStyle(sheetName, "A1", "G1", headerStyle); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}
	return nil
}

func writeFormulaSampleData(writer *formulaTemplateWriter) {
	sampleData := [][]string{
		{"COST_ELEC_STD", "Electricity Cost Standard", "CALCULATION", "ELEC_CONSUMPTION * ELEC_RATE", "COST_ELEC", "ELEC_CONSUMPTION,ELEC_RATE", "Standard electricity cost calculation"},
		{"COST_WATER", "Water Cost", "CALCULATION", "WATER_USAGE * WATER_RATE", "COST_WATER_OUT", "WATER_USAGE,WATER_RATE", "Water cost formula"},
		{"TAX_RATE", "Tax Rate", "CONSTANT", "0.11", "TAX_RATE_OUT", "", "Fixed tax rate value"},
		{"DENIER_QUERY", "Denier Lookup", "SQL_QUERY", "SELECT denier FROM yarn_specs WHERE lot_id = :LOT_ID", "DENIER_OUT", "LOT_ID", "SQL lookup for denier value"},
	}

	for i, row := range sampleData {
		rowNum := i + 2
		for j, val := range row {
			cell, cellErr := excelize.CoordinatesToCellName(j+1, rowNum)
			if cellErr != nil {
				writer.errs = append(writer.errs, fmt.Errorf("coordinates row %d col %d: %w", rowNum, j+1, cellErr))
				continue
			}
			writer.setCellValue(cell, val)
		}
	}
}

func writeFormulaTemplateWidths(writer *formulaTemplateWriter) {
	writer.setColWidth("A", "A", 20)
	writer.setColWidth("B", "B", 30)
	writer.setColWidth("C", "C", 15)
	writer.setColWidth("D", "D", 45)
	writer.setColWidth("E", "E", 20)
	writer.setColWidth("F", "F", 30)
	writer.setColWidth("G", "G", 35)
}

func writeFormulaInstructionsSheet(f *excelize.File) {
	notesSheet := "Instructions"
	if _, sheetErr := f.NewSheet(notesSheet); sheetErr != nil {
		log.Debug().Err(sheetErr).Msg("Could not create Instructions sheet")
		return
	}

	w := &formulaTemplateWriter{f: f, sheetName: notesSheet}
	w.setCellValue("A1", "Import Instructions")
	w.setCellValue("A3", "1. Code: Unique, uppercase letters/numbers/underscores, starts with letter (e.g., COST_ELEC_STD)")
	w.setCellValue("A4", "2. Name: Display name (required, max 200 chars)")
	w.setCellValue("A5", "3. Type: Must be one of: CALCULATION, SQL_QUERY, CONSTANT")
	w.setCellValue("A6", "4. Expression: Formula expression (required, max 5000 chars)")
	w.setCellValue("A7", "5. Result Param Code: Code of the output parameter (must exist in Parameter master)")
	w.setCellValue("A8", "6. Input Param Codes: Comma-separated codes of input parameters (must exist in Parameter master)")
	w.setCellValue("A9", "7. Description: Optional description (max 1000 chars)")
	w.setCellValue("A11", "Notes:")
	w.setCellValue("A12", "- Delete sample data rows before importing")
	w.setCellValue("A13", "- Save file as .xlsx format")
	w.setCellValue("A14", "- Parameter codes must exist in the Parameter master data")
	w.setCellValue("A15", "- Each result parameter can only be used by one formula")
	w.setCellValue("A16", "- Input parameters cannot include the result parameter (no circular reference)")

	if w.hasErrors() {
		log.Warn().Err(w.error()).Msg("Some Instructions sheet operations failed")
	}
}

// formulaTemplateWriter wraps excelize file with error collection.
type formulaTemplateWriter struct {
	f         *excelize.File
	sheetName string
	errs      []error
}

func (tw *formulaTemplateWriter) setCellValue(cell string, value interface{}) {
	if err := tw.f.SetCellValue(tw.sheetName, cell, value); err != nil {
		tw.errs = append(tw.errs, fmt.Errorf("cell %s: %w", cell, err))
	}
}

func (tw *formulaTemplateWriter) setColWidth(startCol, endCol string, width float64) {
	if err := tw.f.SetColWidth(tw.sheetName, startCol, endCol, width); err != nil {
		tw.errs = append(tw.errs, fmt.Errorf("column %s-%s: %w", startCol, endCol, err))
	}
}

func (tw *formulaTemplateWriter) hasErrors() bool { return len(tw.errs) > 0 }

func (tw *formulaTemplateWriter) error() error {
	if len(tw.errs) == 0 {
		return nil
	}
	return errors.Join(tw.errs...)
}
