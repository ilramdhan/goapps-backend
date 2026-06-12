// Package costproductmaster contains application use cases for CostProductMaster.
package costproductmaster

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

// TemplateHandler handles the DownloadCostProductMasterTemplate query.
type TemplateHandler struct{}

// NewTemplateHandler creates a new TemplateHandler.
func NewTemplateHandler() *TemplateHandler {
	return &TemplateHandler{}
}

// cpmExcelWriter wraps excelize with error collection for non-critical operations.
type cpmExcelWriter struct {
	f         *excelize.File
	sheetName string
	errs      []error
}

// setCellValue sets a cell value and collects any error.
func (ew *cpmExcelWriter) setCellValue(cell string, value any) {
	if err := ew.f.SetCellValue(ew.sheetName, cell, value); err != nil {
		ew.errs = append(ew.errs, fmt.Errorf("cell %s: %w", cell, err))
	}
}

// setColWidth sets column width and collects any error.
func (ew *cpmExcelWriter) setColWidth(startCol, endCol string, width float64) {
	if err := ew.f.SetColWidth(ew.sheetName, startCol, endCol, width); err != nil {
		ew.errs = append(ew.errs, fmt.Errorf("column %s-%s: %w", startCol, endCol, err))
	}
}

// hasErrors returns true if any errors were collected.
func (ew *cpmExcelWriter) hasErrors() bool {
	return len(ew.errs) > 0
}

// error returns combined errors or nil.
func (ew *cpmExcelWriter) error() error {
	if len(ew.errs) == 0 {
		return nil
	}
	return errors.Join(ew.errs...)
}

// Handle generates the import template Excel file.
func (h *TemplateHandler) Handle() (result *TemplateResult, err error) {
	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("cpm template: failed to close excel file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	sheetName := "cost_product_master"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("cpm template: could not delete Sheet1")
	}

	if err := writeCPMTemplateHeaders(f, sheetName); err != nil {
		return nil, err
	}

	writer := &cpmExcelWriter{f: f, sheetName: sheetName}
	writeCPMTemplateSampleData(writer)
	writeCPMTemplateColWidths(writer)

	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("cpm template: some excel formatting operations failed")
	}

	writeCPMInstructionsSheet(f)

	buffer, bufErr := f.WriteToBuffer()
	if bufErr != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", bufErr)
	}

	return &TemplateResult{
		FileContent: buffer.Bytes(),
		FileName:    "cost_product_master_import_template.xlsx",
	}, nil
}

func writeCPMTemplateHeaders(f *excelize.File, sheetName string) error {
	headers := []string{
		"product_code", "product_type_code", "product_name",
		"grade_code", "shade_code", "shade_name",
		"erp_item_code", "erp_grade_code1", "erp_grade_code2",
		"flex_01", "flex_02", "flex_03", "description", "is_active",
	}
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
	if err := f.SetCellStyle(sheetName, "A1", "N1", headerStyle); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}
	return nil
}

func writeCPMTemplateSampleData(writer *cpmExcelWriter) {
	sampleData := [][]any{
		{"", "POY", "Polyester Yarn 75D", "A", "RED", "Red Shade", "", "", "", "", "", "", "Sample product", true},
		{"", "PTY", "Textured Yarn 100D", "B", "BLUE", "Blue Shade", "ERP-001", "G1", "", "", "", "", "", true},
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

func writeCPMTemplateColWidths(writer *cpmExcelWriter) {
	writer.setColWidth("A", "A", 18)
	writer.setColWidth("B", "B", 20)
	writer.setColWidth("C", "C", 30)
	writer.setColWidth("D", "D", 12)
	writer.setColWidth("E", "E", 12)
	writer.setColWidth("F", "F", 20)
	writer.setColWidth("G", "G", 18)
	writer.setColWidth("H", "H", 16)
	writer.setColWidth("I", "I", 16)
	writer.setColWidth("J", "J", 12)
	writer.setColWidth("K", "K", 12)
	writer.setColWidth("L", "L", 12)
	writer.setColWidth("M", "M", 25)
	writer.setColWidth("N", "N", 10)
}

func writeCPMInstructionsSheet(f *excelize.File) {
	notesSheet := "Instructions"
	if _, sheetErr := f.NewSheet(notesSheet); sheetErr != nil {
		log.Debug().Err(sheetErr).Msg("cpm template: could not create Instructions sheet")
		return
	}

	notesWriter := &cpmExcelWriter{f: f, sheetName: notesSheet}
	notesWriter.setCellValue("A1", "Import Instructions — Cost Product Master")
	notesWriter.setCellValue("A3", "1. product_code: Leave blank to auto-generate. If provided, the row is treated as upsert (existing product updated).")
	notesWriter.setCellValue("A4", "2. product_type_code: Required. Must match an existing product type code (e.g., POY, PTY).")
	notesWriter.setCellValue("A5", "3. product_name: Required. Max 500 characters.")
	notesWriter.setCellValue("A6", "4. grade_code: Optional. Defaults to 'AX' when empty. Max 20 characters.")
	notesWriter.setCellValue("A7", "5. shade_code / shade_name: Optional shade identifiers.")
	notesWriter.setCellValue("A8", "6. erp_item_code / erp_grade_code1 / erp_grade_code2: Optional ERP linkage fields.")
	notesWriter.setCellValue("A9", "7. flex_01 / flex_02 / flex_03: Optional legacy compound keys from ERP system.")
	notesWriter.setCellValue("A10", "8. description: Optional free-text description.")
	notesWriter.setCellValue("A11", "9. is_active: true/false, yes/no, 1/0. Defaults to true when empty.")
	notesWriter.setCellValue("A13", "Notes:")
	notesWriter.setCellValue("A14", "- Delete sample data rows before importing")
	notesWriter.setCellValue("A15", "- Save file as .xlsx format")
	notesWriter.setCellValue("A16", "- Rows with an existing product_code are upserted (name, shade, grade, description updated)")

	if notesWriter.hasErrors() {
		log.Warn().Err(notesWriter.error()).Msg("cpm template: some Instructions sheet operations failed")
	}
}
