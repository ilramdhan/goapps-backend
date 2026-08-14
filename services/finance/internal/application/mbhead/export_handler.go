// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// ExportQuery represents the export MB Heads query.
type ExportQuery struct {
	IsActive *bool
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

// setupExcelSheet creates and configures the export sheet.
func setupExcelSheet(f *excelize.File, sheetName string, headers []string) error {
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	// Delete default sheet (non-critical)
	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
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

	lastCol, err := excelize.CoordinatesToCellName(len(headers), 1)
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

// mbHeadColumnWidth sizes a column from its header text: wide enough to read the header, with
// a floor so short headers still fit their values. Derived rather than hardcoded per letter so
// the widths cannot drift out of step with the shared column table.
func mbHeadColumnWidth(header string) float64 {
	const minWidth = 12.0
	w := float64(len(header)) + 4
	if w < minWidth {
		return minWidth
	}
	return w
}

// setMBHeadColumnWidths sizes every column of cols by index.
func setMBHeadColumnWidths(writer *excelWriter, cols []mbHeadImportColumn) {
	for i, col := range cols {
		name, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			writer.errs = append(writer.errs, fmt.Errorf("column %d name: %w", i+1, err))
			continue
		}
		writer.setColWidth(name, name, mbHeadColumnWidth(col.header))
	}
}

// writeMBHeadRow writes one export row. Cells are addressed by column index off the shared
// column table, so adding a column to mbHeadImportColumns needs no change here.
func writeMBHeadRow(writer *excelWriter, cols []mbHeadImportColumn, rowNum int, row mbHeadExportRow) {
	for i, col := range cols {
		cell, err := excelize.CoordinatesToCellName(i+1, rowNum)
		if err != nil {
			writer.errs = append(writer.errs, fmt.Errorf("column %d: %w", i+1, err))
			continue
		}
		writer.setCellValue(cell, col.export(row))
	}
}

// Handle executes the export MB Heads query.
func (h *ExportHandler) Handle(ctx context.Context, query ExportQuery) (result *ExportResult, err error) {
	heads, err := h.repo.ListAll(ctx, mbhead.ExportFilter{IsActive: query.IsActive})
	if err != nil {
		return nil, fmt.Errorf("failed to get mb heads for export: %w", err)
	}

	// One batched query rather than a ListShades per head: the export is unpaginated over the
	// whole table, so a per-row lookup would be an N+1 across thousands of rows.
	ids := make([]uuid.UUID, 0, len(heads))
	for _, e := range heads {
		ids = append(ids, e.ID())
	}
	shadesByHead, err := h.repo.ListShadesByHeads(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get mb head shades for export: %w", err)
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

	cols := mbHeadExportColumns()
	sheetName := "MB Heads"
	if err := setupExcelSheet(f, sheetName, mbHeadColumnHeaders(cols)); err != nil {
		return nil, err
	}

	writer := &excelWriter{f: f, sheetName: sheetName}

	for i, e := range heads {
		writeMBHeadRow(writer, cols, i+2, mbHeadExportRow{entity: e, shades: shadesByHead[e.ID()]})
	}

	setMBHeadColumnWidths(writer, cols)

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
