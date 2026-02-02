// Package uom provides application layer handlers for UOM operations.
package uom

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

// TemplateHandler handles the DownloadTemplate query.
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
	// Create Excel file
	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close Excel template file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	sheetName := "UOM Import Template"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	// Delete default sheet (non-critical)
	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	// Set headers
	headers := []string{"Code", "Name", "Category", "Description"}
	for col, header := range headers {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return nil, fmt.Errorf("failed to get cell name: %w", err)
		}
		if err := f.SetCellValue(sheetName, cell, header); err != nil {
			return nil, fmt.Errorf("failed to set header %s: %w", header, err)
		}
	}

	// Style headers
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create header style: %w", err)
	}
	if err := f.SetCellStyle(sheetName, "A1", "D1", headerStyle); err != nil {
		return nil, fmt.Errorf("failed to set header style: %w", err)
	}

	// Create writer for data rows (non-critical errors are collected)
	writer := &templateExcelWriter{f: f, sheetName: sheetName}

	// Add sample data
	sampleData := [][]string{
		{"KG", "Kilogram", "WEIGHT", "Weight in kilograms"},
		{"MTR", "Meter", "LENGTH", "Length in meters"},
		{"LTR", "Liter", "VOLUME", "Volume in liters"},
		{"PCS", "Pieces", "QUANTITY", "Count in pieces"},
	}

	for i, row := range sampleData {
		rowNum := i + 2
		writer.setCellValue(fmt.Sprintf("A%d", rowNum), row[0])
		writer.setCellValue(fmt.Sprintf("B%d", rowNum), row[1])
		writer.setCellValue(fmt.Sprintf("C%d", rowNum), row[2])
		writer.setCellValue(fmt.Sprintf("D%d", rowNum), row[3])
	}

	// Set column widths
	writer.setColWidth("A", "A", 15)
	writer.setColWidth("B", "B", 25)
	writer.setColWidth("C", "C", 15)
	writer.setColWidth("D", "D", 40)

	// Add validation note sheet
	notesSheet := "Instructions"
	if _, sheetErr := f.NewSheet(notesSheet); sheetErr != nil {
		log.Debug().Err(sheetErr).Msg("Could not create Instructions sheet")
	} else {
		notesWriter := &templateExcelWriter{f: f, sheetName: notesSheet}
		notesWriter.setCellValue("A1", "Import Instructions")
		notesWriter.setCellValue("A3", "1. Code: Unique, uppercase letters/numbers/underscores (e.g., KG, MTR_SQ)")
		notesWriter.setCellValue("A4", "2. Name: Display name (required)")
		notesWriter.setCellValue("A5", "3. Category: Must be one of: WEIGHT, LENGTH, VOLUME, QUANTITY")
		notesWriter.setCellValue("A6", "4. Description: Optional description")
		notesWriter.setCellValue("A8", "Notes:")
		notesWriter.setCellValue("A9", "- Delete sample data rows before importing")
		notesWriter.setCellValue("A10", "- Save file as .xlsx format")

		if notesWriter.hasErrors() {
			log.Warn().Err(notesWriter.error()).Msg("Some Instructions sheet operations failed")
		}
	}

	// Log any non-critical errors but continue
	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("Some Excel formatting operations failed")
	}

	// Write to buffer
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &TemplateResult{
		FileContent: buffer.Bytes(),
		FileName:    "uom_import_template.xlsx",
	}, nil
}
