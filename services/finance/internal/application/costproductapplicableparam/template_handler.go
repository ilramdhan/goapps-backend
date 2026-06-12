// Package costproductapplicableparam contains application use cases for
// Cost Product Applicable Param (CAPP_) operations.
package costproductapplicableparam

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"
)

// TemplateResult represents the download template result.
type TemplateResult struct {
	FileContent []byte
	FileName    string
}

// TemplateHandler handles the DownloadCAPPTemplate query.
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
			log.Warn().Err(closeErr).Msg("capp template: failed to close excel file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	sheetName := "capp"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("capp template: could not delete Sheet1")
	}

	if err := writeCAPPTemplateHeaders(f, sheetName); err != nil {
		return nil, err
	}

	writer := &cappExcelWriter{f: f, sheetName: sheetName}
	writeCAPPTemplateSampleData(writer)
	writeCAPPTemplateColWidths(writer)

	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("capp template: some excel formatting operations failed")
	}

	writeCAPPInstructionsSheet(f)

	buffer, bufErr := f.WriteToBuffer()
	if bufErr != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", bufErr)
	}

	return &TemplateResult{
		FileContent: buffer.Bytes(),
		FileName:    "cost_product_applicable_param_import_template.xlsx",
	}, nil
}

func writeCAPPTemplateHeaders(f *excelize.File, sheetName string) error {
	headers := []string{"product_code", "param_code", "is_required", "display_order"}
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
	if err := f.SetCellStyle(sheetName, "A1", "D1", headerStyle); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}
	return nil
}

func writeCAPPTemplateSampleData(writer *cappExcelWriter) {
	sampleData := [][]any{
		{"POY-0001", "SPEED", true, 1},
		{"POY-0001", "DENIER", true, 2},
		{"PTY-0001", "ELEC_RATE", false, ""},
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

func writeCAPPTemplateColWidths(writer *cappExcelWriter) {
	writer.setColWidth("A", "A", 20)
	writer.setColWidth("B", "B", 20)
	writer.setColWidth("C", "C", 12)
	writer.setColWidth("D", "D", 15)
}

func writeCAPPInstructionsSheet(f *excelize.File) {
	notesSheet := "Instructions"
	if _, sheetErr := f.NewSheet(notesSheet); sheetErr != nil {
		log.Debug().Err(sheetErr).Msg("capp template: could not create Instructions sheet")
		return
	}

	notesWriter := &cappExcelWriter{f: f, sheetName: notesSheet}
	notesWriter.setCellValue("A1", "Import Instructions — Cost Product Applicable Param (CAPP)")
	notesWriter.setCellValue("A3", "1. product_code: Required. Must match an existing product master code.")
	notesWriter.setCellValue("A4", "2. param_code: Required. Must match an existing active parameter code.")
	notesWriter.setCellValue("A5", "3. is_required: Optional. true/false, yes/no, 1/0. Defaults to false when empty.")
	notesWriter.setCellValue("A6", "4. display_order: Optional. Positive integer; leave blank to inherit from parameter default.")
	notesWriter.setCellValue("A8", "Notes:")
	notesWriter.setCellValue("A9", "- Rows are upserted: if the product+param pair already exists the is_required/display_order are updated")
	notesWriter.setCellValue("A10", "- Delete sample data rows before importing")
	notesWriter.setCellValue("A11", "- Save file as .xlsx format")

	if notesWriter.hasErrors() {
		log.Warn().Err(notesWriter.error()).Msg("capp template: some Instructions sheet operations failed")
	}
}
