// Package costproductparameter wires CPP_ use cases.
package costproductparameter

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

// TemplateHandler handles the DownloadCPPTemplate query.
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
			log.Warn().Err(closeErr).Msg("cpp template: failed to close excel file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	sheetName := "cpp"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("cpp template: could not delete Sheet1")
	}

	if err := writeCPPTemplateHeaders(f, sheetName); err != nil {
		return nil, err
	}

	writer := &cppExcelWriter{f: f, sheetName: sheetName}
	writeCPPTemplateSampleData(writer)
	writeCPPTemplateColWidths(writer)

	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("cpp template: some excel formatting operations failed")
	}

	writeCPPInstructionsSheet(f)

	buffer, bufErr := f.WriteToBuffer()
	if bufErr != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", bufErr)
	}

	return &TemplateResult{
		FileContent: buffer.Bytes(),
		FileName:    "cost_product_parameter_import_template.xlsx",
	}, nil
}

func writeCPPTemplateHeaders(f *excelize.File, sheetName string) error {
	headers := []string{"product_code", "param_code", "value_numeric", "value_text", "value_flag"}
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
	if err := f.SetCellStyle(sheetName, "A1", "E1", headerStyle); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}
	return nil
}

func writeCPPTemplateSampleData(writer *cppExcelWriter) {
	// Each row must have exactly one of value_numeric / value_text / value_flag populated.
	sampleData := [][]any{
		{"POY-0001", "SPEED", "1500", "", ""},
		{"POY-0001", "DENIER", "75.5", "", ""},
		{"POY-0001", "GRADE_LABEL", "", "A-grade", ""},
		{"POY-0001", "IS_PREMIUM", "", "", "false"},
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

func writeCPPTemplateColWidths(writer *cppExcelWriter) {
	writer.setColWidth("A", "A", 20)
	writer.setColWidth("B", "B", 20)
	writer.setColWidth("C", "C", 18)
	writer.setColWidth("D", "D", 18)
	writer.setColWidth("E", "E", 12)
}

func writeCPPInstructionsSheet(f *excelize.File) {
	notesSheet := "Instructions"
	if _, sheetErr := f.NewSheet(notesSheet); sheetErr != nil {
		log.Debug().Err(sheetErr).Msg("cpp template: could not create Instructions sheet")
		return
	}

	notesWriter := &cppExcelWriter{f: f, sheetName: notesSheet}
	notesWriter.setCellValue("A1", "Import Instructions — Cost Product Parameter (CPP)")
	notesWriter.setCellValue("A3", "1. product_code: Required. Must match an existing product master code.")
	notesWriter.setCellValue("A4", "2. param_code: Required. Must match an existing active parameter code. Param must also be in the product's applicable list (CAPP).")
	notesWriter.setCellValue("A5", "3. value_numeric: Populate for NUMBER parameters. Leave blank for TEXT/BOOLEAN rows.")
	notesWriter.setCellValue("A6", "4. value_text: Populate for TEXT parameters. Leave blank for NUMBER/BOOLEAN rows.")
	notesWriter.setCellValue("A7", "5. value_flag: Populate for BOOLEAN parameters (true/false, yes/no, 1/0). Leave blank for NUMBER/TEXT rows.")
	notesWriter.setCellValue("A9", "Rules:")
	notesWriter.setCellValue("A10", "- Exactly ONE value column must be populated per row — the import handler uses the parameter's data_type to determine which column to read")
	notesWriter.setCellValue("A11", "- Rows are upserted: if the product+param pair already exists the value is overwritten")
	notesWriter.setCellValue("A12", "- Period-dependent parameters cannot be stored in CPP; upload those via the fill form instead")
	notesWriter.setCellValue("A14", "Notes:")
	notesWriter.setCellValue("A15", "- Delete sample data rows before importing")
	notesWriter.setCellValue("A16", "- Save file as .xlsx format")

	if notesWriter.hasErrors() {
		log.Warn().Err(notesWriter.error()).Msg("cpp template: some Instructions sheet operations failed")
	}
}
