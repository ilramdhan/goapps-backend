// Package uom provides application layer handlers for UOM operations.
package uom

import (
	"context"
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

// ExportQuery represents the export UOMs query.
type ExportQuery struct {
	Category *string
	IsActive *bool
}

// ExportResult represents the export UOMs result.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler handles the ExportUOMs query.
type ExportHandler struct {
	repo uom.Repository
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(repo uom.Repository) *ExportHandler {
	return &ExportHandler{repo: repo}
}

// Handle executes the export UOMs query.
func (h *ExportHandler) Handle(ctx context.Context, query ExportQuery) (*ExportResult, error) {
	// Build filter
	filter := uom.ExportFilter{}

	if query.Category != nil {
		cat, err := uom.NewCategory(*query.Category)
		if err != nil {
			return nil, err
		}
		filter.Category = &cat
	}
	filter.IsActive = query.IsActive

	// Get all UOMs
	uoms, err := h.repo.ListAll(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get uoms for export: %w", err)
	}

	// Create Excel file
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	sheetName := "UOMs"
	index, _ := f.NewSheet(sheetName)
	f.SetActiveSheet(index)
	_ = f.DeleteSheet("Sheet1")

	// Set headers
	headers := []string{"No", "Code", "Name", "Category", "Description", "Active", "Created At", "Created By"}
	for col, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = f.SetCellValue(sheetName, cell, header)
	}

	// Style headers
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	_ = f.SetCellStyle(sheetName, "A1", "H1", headerStyle)

	// Write data
	for i, u := range uoms {
		row := i + 2
		_ = f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), i+1)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), u.Code().String())
		_ = f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), u.Name())
		_ = f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), u.Category().String())
		_ = f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), u.Description())
		_ = f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), u.IsActive())
		_ = f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), u.CreatedAt().Format("2006-01-02 15:04:05"))
		_ = f.SetCellValue(sheetName, fmt.Sprintf("H%d", row), u.CreatedBy())
	}

	// Set column widths
	_ = f.SetColWidth(sheetName, "A", "A", 5)
	_ = f.SetColWidth(sheetName, "B", "B", 15)
	_ = f.SetColWidth(sheetName, "C", "C", 25)
	_ = f.SetColWidth(sheetName, "D", "D", 15)
	_ = f.SetColWidth(sheetName, "E", "E", 40)
	_ = f.SetColWidth(sheetName, "F", "F", 10)
	_ = f.SetColWidth(sheetName, "G", "G", 20)
	_ = f.SetColWidth(sheetName, "H", "H", 20)

	// Write to buffer
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &ExportResult{
		FileContent: buffer.Bytes(),
		FileName:    "uom_export.xlsx",
	}, nil
}
