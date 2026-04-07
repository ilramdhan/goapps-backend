// Package parameter provides application layer handlers for Parameter operations.
package parameter

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// ExportQuery represents the export Parameters query.
type ExportQuery struct {
	DataType      *string
	ParamCategory *string
	IsActive      *bool
}

// ExportResult represents the export Parameters result.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler handles the ExportParameters query.
type ExportHandler struct {
	repo parameter.Repository
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(repo parameter.Repository) *ExportHandler {
	return &ExportHandler{repo: repo}
}

// excelWriter wraps excelize file with error collection for non-critical operations.
type excelWriter struct {
	f         *excelize.File
	sheetName string
	errs      []error
}

// setCellValue sets a cell value and collects any error.
func (ew *excelWriter) setCellValue(cell string, value interface{}) {
	if err := ew.f.SetCellValue(ew.sheetName, cell, value); err != nil {
		ew.errs = append(ew.errs, fmt.Errorf("cell %s: %w", cell, err))
	}
}

// setColWidth sets column width and collects any error.
func (ew *excelWriter) setColWidth(startCol, endCol string, width float64) {
	if err := ew.f.SetColWidth(ew.sheetName, startCol, endCol, width); err != nil {
		ew.errs = append(ew.errs, fmt.Errorf("column %s-%s: %w", startCol, endCol, err))
	}
}

// hasErrors returns true if any errors were collected.
func (ew *excelWriter) hasErrors() bool {
	return len(ew.errs) > 0
}

// error returns combined errors or nil.
func (ew *excelWriter) error() error {
	if len(ew.errs) == 0 {
		return nil
	}
	return errors.Join(ew.errs...)
}

// buildParamExportFilter creates an export filter from the query.
func buildParamExportFilter(query ExportQuery) (parameter.ExportFilter, error) {
	filter := parameter.ExportFilter{}

	if query.DataType != nil {
		dt, err := parameter.NewDataType(*query.DataType)
		if err != nil {
			return filter, err
		}
		filter.DataType = &dt
	}

	if query.ParamCategory != nil {
		cat, err := parameter.NewParamCategory(*query.ParamCategory)
		if err != nil {
			return filter, err
		}
		filter.ParamCategory = &cat
	}

	filter.IsActive = query.IsActive

	return filter, nil
}

// setupParamExcelSheet creates and configures the export sheet.
func setupParamExcelSheet(f *excelize.File, sheetName string) error {
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	// Delete default sheet (non-critical)
	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	// Set headers
	headers := []string{
		"No", "Code", "Name", "Short Name", "Data Type", "Category",
		"UOM Code", "Default Value", "Min Value", "Max Value",
		"Active", "Created At", "Created By",
	}
	for col, header := range headers {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return fmt.Errorf("failed to get cell name: %w", err)
		}
		if err := f.SetCellValue(sheetName, cell, header); err != nil {
			return fmt.Errorf("failed to set header %s: %w", header, err)
		}
	}

	// Style headers
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return fmt.Errorf("failed to create header style: %w", err)
	}
	if err := f.SetCellStyle(sheetName, "A1", "M1", headerStyle); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}

	return nil
}

// Handle executes the export Parameters query.
func (h *ExportHandler) Handle(ctx context.Context, query ExportQuery) (result *ExportResult, err error) {
	// Build filter
	filter, err := buildParamExportFilter(query)
	if err != nil {
		return nil, err
	}

	// Get all Parameters
	params, err := h.repo.ListAll(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameters for export: %w", err)
	}

	// Create Excel file
	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close Excel file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	sheetName := "Parameters"
	if err := setupParamExcelSheet(f, sheetName); err != nil {
		return nil, err
	}

	// Create writer for data rows (non-critical errors are collected)
	writer := &excelWriter{f: f, sheetName: sheetName}

	// Write data rows
	for i, p := range params {
		row := i + 2
		writer.setCellValue(fmt.Sprintf("A%d", row), i+1)
		writer.setCellValue(fmt.Sprintf("B%d", row), p.Code().String())
		writer.setCellValue(fmt.Sprintf("C%d", row), p.Name())
		writer.setCellValue(fmt.Sprintf("D%d", row), p.ShortName())
		writer.setCellValue(fmt.Sprintf("E%d", row), p.DataType().String())
		writer.setCellValue(fmt.Sprintf("F%d", row), p.ParamCategory().String())
		writer.setCellValue(fmt.Sprintf("G%d", row), p.UOMCode())

		defVal := ""
		if p.DefaultValue() != nil {
			defVal = *p.DefaultValue()
		}
		writer.setCellValue(fmt.Sprintf("H%d", row), defVal)

		minVal := ""
		if p.MinValue() != nil {
			minVal = *p.MinValue()
		}
		writer.setCellValue(fmt.Sprintf("I%d", row), minVal)

		maxVal := ""
		if p.MaxValue() != nil {
			maxVal = *p.MaxValue()
		}
		writer.setCellValue(fmt.Sprintf("J%d", row), maxVal)

		writer.setCellValue(fmt.Sprintf("K%d", row), p.IsActive())
		writer.setCellValue(fmt.Sprintf("L%d", row), p.CreatedAt().Format("2006-01-02 15:04:05"))
		writer.setCellValue(fmt.Sprintf("M%d", row), p.CreatedBy())
	}

	// Set column widths
	writer.setColWidth("A", "A", 5)
	writer.setColWidth("B", "B", 15)
	writer.setColWidth("C", "C", 25)
	writer.setColWidth("D", "D", 15)
	writer.setColWidth("E", "E", 12)
	writer.setColWidth("F", "F", 15)
	writer.setColWidth("G", "G", 12)
	writer.setColWidth("H", "H", 15)
	writer.setColWidth("I", "I", 12)
	writer.setColWidth("J", "J", 12)
	writer.setColWidth("K", "K", 10)
	writer.setColWidth("L", "L", 20)
	writer.setColWidth("M", "M", 20)

	// Log any non-critical errors but continue
	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("Some Excel formatting operations failed")
	}

	// Write to buffer
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &ExportResult{
		FileContent: buffer.Bytes(),
		FileName:    "parameter_export.xlsx",
	}, nil
}
