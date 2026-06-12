// Package costproductapplicableparam contains application use cases for
// Cost Product Applicable Param (CAPP_) operations.
package costproductapplicableparam

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

// ExportResult represents the export CAPP result.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler handles the export CAPP query.
type ExportHandler struct {
	repo cpp.Repository
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(repo cpp.Repository) *ExportHandler {
	return &ExportHandler{repo: repo}
}

// Handle executes the export CAPP query.
func (h *ExportHandler) Handle(ctx context.Context) (result *ExportResult, err error) {
	rows, listErr := h.repo.ListAllApplicable(ctx)
	if listErr != nil {
		return nil, fmt.Errorf("list applicable params for export: %w", listErr)
	}

	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("capp export: failed to close excel file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	sheetName := "capp"
	if setupErr := setupCAPPExcelSheet(f, sheetName); setupErr != nil {
		return nil, setupErr
	}

	writer := &cappExcelWriter{f: f, sheetName: sheetName}
	writeCAPPDataRows(writer, rows)
	writeCAPPColWidths(writer)

	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("capp export: some excel formatting operations failed")
	}

	buffer, bufErr := f.WriteToBuffer()
	if bufErr != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", bufErr)
	}

	return &ExportResult{
		FileContent: buffer.Bytes(),
		FileName:    "cost_product_applicable_param_export.xlsx",
	}, nil
}

func setupCAPPExcelSheet(f *excelize.File, sheetName string) error {
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("capp export: could not delete Sheet1")
	}

	headers := []string{"No", "cpm_product_code", "param_code", "capp_is_required", "capp_display_order"}
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

func writeCAPPDataRows(writer *cappExcelWriter, rows []cpp.CAPPRow) {
	for i, r := range rows {
		row := i + 2
		writer.setCellValue(fmt.Sprintf("A%d", row), i+1)
		writer.setCellValue(fmt.Sprintf("B%d", row), r.ProductCode)
		writer.setCellValue(fmt.Sprintf("C%d", row), r.ParamCode)
		writer.setCellValue(fmt.Sprintf("D%d", row), r.IsRequired)

		dispOrder := ""
		if r.DisplayOrder != nil {
			dispOrder = fmt.Sprintf("%d", *r.DisplayOrder)
		}
		writer.setCellValue(fmt.Sprintf("E%d", row), dispOrder)
	}
}

func writeCAPPColWidths(writer *cappExcelWriter) {
	writer.setColWidth("A", "A", 5)
	writer.setColWidth("B", "B", 20)
	writer.setColWidth("C", "C", 20)
	writer.setColWidth("D", "D", 15)
	writer.setColWidth("E", "E", 15)
}

// cappExcelWriter wraps excelize with error collection for non-critical operations.
type cappExcelWriter struct {
	f         *excelize.File
	sheetName string
	errs      []error
}

// setCellValue sets a cell value and collects any error.
func (ew *cappExcelWriter) setCellValue(cell string, value any) {
	if err := ew.f.SetCellValue(ew.sheetName, cell, value); err != nil {
		ew.errs = append(ew.errs, fmt.Errorf("cell %s: %w", cell, err))
	}
}

// setColWidth sets column width and collects any error.
func (ew *cappExcelWriter) setColWidth(startCol, endCol string, width float64) {
	if err := ew.f.SetColWidth(ew.sheetName, startCol, endCol, width); err != nil {
		ew.errs = append(ew.errs, fmt.Errorf("column %s-%s: %w", startCol, endCol, err))
	}
}

// hasErrors returns true if any errors were collected.
func (ew *cappExcelWriter) hasErrors() bool {
	return len(ew.errs) > 0
}

// error returns combined errors or nil.
func (ew *cappExcelWriter) error() error {
	if len(ew.errs) == 0 {
		return nil
	}
	return errors.Join(ew.errs...)
}
