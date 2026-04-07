// Package parameter provides application layer handlers for Parameter operations.
package parameter

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

// TemplateHandler handles the DownloadParameterTemplate query.
type TemplateHandler struct{}

// NewTemplateHandler creates a new TemplateHandler.
func NewTemplateHandler() *TemplateHandler {
	return &TemplateHandler{}
}

// templateExcelWriter wraps excelize file with error collection for template generation.
type templateExcelWriter struct {
	f         *excelize.File
	sheetName string
	errs      []error
}

// setCellValue sets a cell value and collects any error.
func (tw *templateExcelWriter) setCellValue(cell string, value interface{}) {
	if err := tw.f.SetCellValue(tw.sheetName, cell, value); err != nil {
		tw.errs = append(tw.errs, fmt.Errorf("cell %s: %w", cell, err))
	}
}

// setColWidth sets column width and collects any error.
func (tw *templateExcelWriter) setColWidth(startCol, endCol string, width float64) {
	if err := tw.f.SetColWidth(tw.sheetName, startCol, endCol, width); err != nil {
		tw.errs = append(tw.errs, fmt.Errorf("column %s-%s: %w", startCol, endCol, err))
	}
}

// hasErrors returns true if any errors were collected.
func (tw *templateExcelWriter) hasErrors() bool {
	return len(tw.errs) > 0
}

// error returns combined errors or nil.
func (tw *templateExcelWriter) error() error {
	if len(tw.errs) == 0 {
		return nil
	}
	return errors.Join(tw.errs...)
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

	sheetName := "Parameter Import Template"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	if err := writeTemplateHeaders(f, sheetName); err != nil {
		return nil, err
	}

	writer := &templateExcelWriter{f: f, sheetName: sheetName}
	writeSampleData(writer)
	writeColumnWidths(writer)

	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("Some Excel formatting operations failed")
	}

	writeInstructionsSheet(f)

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &TemplateResult{
		FileContent: buffer.Bytes(),
		FileName:    "parameter_import_template.xlsx",
	}, nil
}

func writeTemplateHeaders(f *excelize.File, sheetName string) error {
	headers := []string{"Code", "Name", "Short Name", "Data Type", "Category", "UOM Code", "Default Value", "Min Value", "Max Value"}
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
	if err := f.SetCellStyle(sheetName, "A1", "I1", headerStyle); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}
	return nil
}

func writeSampleData(writer *templateExcelWriter) {
	sampleData := [][]string{
		{"SPEED", "Speed", "Spd", "NUMBER", "INPUT", "RPM", "100", "0", "9999"},
		{"DENIER", "Denier", "Den", "NUMBER", "INPUT", "", "75", "10", "300"},
		{"ELEC_RATE", "Electricity Rate", "Elec", "NUMBER", "RATE", "KWH", "0.5", "0", "100"},
		{"IS_PREMIUM", "Is Premium", "", "BOOLEAN", "CALCULATED", "", "false", "", ""},
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

func writeColumnWidths(writer *templateExcelWriter) {
	writer.setColWidth("A", "A", 15)
	writer.setColWidth("B", "B", 25)
	writer.setColWidth("C", "C", 15)
	writer.setColWidth("D", "D", 12)
	writer.setColWidth("E", "E", 15)
	writer.setColWidth("F", "F", 12)
	writer.setColWidth("G", "G", 15)
	writer.setColWidth("H", "H", 12)
	writer.setColWidth("I", "I", 12)
}

func writeInstructionsSheet(f *excelize.File) {
	notesSheet := "Instructions"
	if _, sheetErr := f.NewSheet(notesSheet); sheetErr != nil {
		log.Debug().Err(sheetErr).Msg("Could not create Instructions sheet")
		return
	}

	notesWriter := &templateExcelWriter{f: f, sheetName: notesSheet}
	notesWriter.setCellValue("A1", "Import Instructions")
	notesWriter.setCellValue("A3", "1. Code: Unique, uppercase letters/numbers/underscores, starts with letter (e.g., SPEED, ELEC_RATE)")
	notesWriter.setCellValue("A4", "2. Name: Display name (required, max 200 chars)")
	notesWriter.setCellValue("A5", "3. Short Name: Optional short name (max 50 chars)")
	notesWriter.setCellValue("A6", "4. Data Type: Must be one of: NUMBER, TEXT, BOOLEAN")
	notesWriter.setCellValue("A7", "5. Category: Must be one of: INPUT, RATE, CALCULATED")
	notesWriter.setCellValue("A8", "6. UOM Code: Optional, must match an existing UOM code (e.g., KG, RPM)")
	notesWriter.setCellValue("A9", "7. Default Value: Optional default value (decimal for NUMBER, text for TEXT, true/false for BOOLEAN)")
	notesWriter.setCellValue("A10", "8. Min Value: Optional minimum (decimal)")
	notesWriter.setCellValue("A11", "9. Max Value: Optional maximum (decimal)")
	notesWriter.setCellValue("A13", "Notes:")
	notesWriter.setCellValue("A14", "- Delete sample data rows before importing")
	notesWriter.setCellValue("A15", "- Save file as .xlsx format")
	notesWriter.setCellValue("A16", "- UOM Code must exist in the UOM master data")

	if notesWriter.hasErrors() {
		log.Warn().Err(notesWriter.error()).Msg("Some Instructions sheet operations failed")
	}
}
