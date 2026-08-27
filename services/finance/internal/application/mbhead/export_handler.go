// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// ExportQuery represents the export MB Heads query.
type ExportQuery struct {
	IsActive *bool
	// IncludeRejected forwards verbatim to mbhead.ExportFilter.IncludeRejected — false
	// (the zero value) EXCLUDES REJECTED heads from the export. See that field's doc
	// comment for the pending proto follow-up.
	IncludeRejected bool
}

// ExportResult represents the export MB Heads result.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler handles the ExportMBHeads query.
type ExportHandler struct {
	repo mbhead.Repository
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(repo mbhead.Repository) *ExportHandler {
	return &ExportHandler{repo: repo}
}

// excelWriter wraps excelize file with error collection for non-critical operations.
type excelWriter struct {
	f         *excelize.File
	sheetName string
	errs      []error
}

// setCellValue sets a cell value and collects any error.
func (ew *excelWriter) setCellValue(cell string, value any) {
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

// mbHeadExportHeaders lists the export/template column headers in column order.
var mbHeadExportHeaders = []string{
	"No", "MB Costing", "Mgt Name", "Dev Code", "Shade Code", "Shade Name",
	"Cross Section", "Lusture Code", "Denier", "Filament", "Dozing",
	"Is Bought Out", "Active", "Created At", "Created By",
}

// setupExcelSheet creates and configures the export sheet.
func setupExcelSheet(f *excelize.File, sheetName string) error {
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	// Delete default sheet (non-critical)
	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	for col, header := range mbHeadExportHeaders {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return fmt.Errorf("failed to get cell name: %w", err)
		}
		if err := f.SetCellValue(sheetName, cell, header); err != nil {
			return fmt.Errorf("failed to set header %s: %w", header, err)
		}
	}

	lastCol, err := excelize.CoordinatesToCellName(len(mbHeadExportHeaders), 1)
	if err != nil {
		return fmt.Errorf("failed to get last header cell name: %w", err)
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return fmt.Errorf("failed to create header style: %w", err)
	}
	if err := f.SetCellStyle(sheetName, "A1", lastCol+"1", headerStyle); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}

	return nil
}

func optFloat(v *float64) any {
	if v == nil {
		return ""
	}
	return *v
}

func optInt(v *int) any {
	if v == nil {
		return ""
	}
	return *v
}

func optStr(v *string) any {
	if v == nil {
		return ""
	}
	return *v
}

// writeMBHeadRow writes a single MB Head entity's fields into the given Excel row.
func writeMBHeadRow(writer *excelWriter, row int, idx int, e *mbhead.Entity) {
	writer.setCellValue(fmt.Sprintf("A%d", row), idx+1)
	writer.setCellValue(fmt.Sprintf("B%d", row), e.MBCosting())
	writer.setCellValue(fmt.Sprintf("C%d", row), optStr(e.MgtName()))
	writer.setCellValue(fmt.Sprintf("D%d", row), e.DevCode())
	writer.setCellValue(fmt.Sprintf("E%d", row), e.ShadeCode())
	writer.setCellValue(fmt.Sprintf("F%d", row), e.ShadeName())
	writer.setCellValue(fmt.Sprintf("G%d", row), e.CrossSection())
	writer.setCellValue(fmt.Sprintf("H%d", row), e.LustureCode())
	writer.setCellValue(fmt.Sprintf("I%d", row), optFloat(e.Denier()))
	writer.setCellValue(fmt.Sprintf("J%d", row), optInt(e.Filament()))
	writer.setCellValue(fmt.Sprintf("K%d", row), optFloat(e.Dozing()))
	writer.setCellValue(fmt.Sprintf("L%d", row), e.IsBoughtout())
	writer.setCellValue(fmt.Sprintf("M%d", row), e.IsActive())
	writer.setCellValue(fmt.Sprintf("N%d", row), e.CreatedAt().Format("2006-01-02 15:04:05"))
	writer.setCellValue(fmt.Sprintf("O%d", row), e.CreatedBy())
}

// Handle executes the export MB Heads query.
func (h *ExportHandler) Handle(ctx context.Context, query ExportQuery) (result *ExportResult, err error) {
	heads, err := h.repo.ListAll(ctx, mbhead.ExportFilter{
		IsActive:        query.IsActive,
		IncludeRejected: query.IncludeRejected,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get mb heads for export: %w", err)
	}

	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close Excel file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	sheetName := "MB Heads"
	if err := setupExcelSheet(f, sheetName); err != nil {
		return nil, err
	}

	writer := &excelWriter{f: f, sheetName: sheetName}

	for i, e := range heads {
		writeMBHeadRow(writer, i+2, i, e)
	}

	writer.setColWidth("A", "A", 5)
	writer.setColWidth("B", "B", 20)
	writer.setColWidth("C", "C", 25)
	writer.setColWidth("D", "F", 15)
	writer.setColWidth("G", "H", 15)
	writer.setColWidth("I", "K", 10)
	writer.setColWidth("L", "M", 10)
	writer.setColWidth("N", "N", 20)
	writer.setColWidth("O", "O", 20)

	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("Some Excel formatting operations failed")
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &ExportResult{
		FileContent: buffer.Bytes(),
		FileName:    "mb_head_export.xlsx",
	}, nil
}
