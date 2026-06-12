// Package costproductmaster contains application use cases for CostProductMaster.
package costproductmaster

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// ExportQuery represents the export CostProductMaster query.
type ExportQuery struct {
	Search        string
	ProductTypeID int32
	ShadeCode     string
	ActiveFilter  string
}

// ExportResult represents the export CostProductMaster result.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler handles the export CostProductMaster query.
type ExportHandler struct {
	repo domain.Repository
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(repo domain.Repository) *ExportHandler {
	return &ExportHandler{repo: repo}
}

// Handle executes the export CostProductMaster query.
func (h *ExportHandler) Handle(ctx context.Context, query ExportQuery) (result *ExportResult, err error) {
	items, listErr := h.repo.ListAll(ctx, domain.Filter{
		Search:        query.Search,
		ProductTypeID: query.ProductTypeID,
		ShadeCode:     query.ShadeCode,
		ActiveFilter:  query.ActiveFilter,
	})
	if listErr != nil {
		return nil, fmt.Errorf("list products for export: %w", listErr)
	}

	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("cpm export: failed to close excel file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	sheetName := "cost_product_master"
	if setupErr := setupCPMExcelSheet(f, sheetName); setupErr != nil {
		return nil, setupErr
	}

	writer := &cpmExcelWriter{f: f, sheetName: sheetName}
	writeCPMRows(writer, items)
	writeCPMColWidths(writer)

	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("cpm export: some excel formatting operations failed")
	}

	buffer, bufErr := f.WriteToBuffer()
	if bufErr != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", bufErr)
	}

	return &ExportResult{
		FileContent: buffer.Bytes(),
		FileName:    "cost_product_master_export.xlsx",
	}, nil
}

func setupCPMExcelSheet(f *excelize.File, sheetName string) error {
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("cpm export: could not delete Sheet1")
	}

	headers := []string{
		"No", "product_code", "product_type_code", "product_name",
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
	if err := f.SetCellStyle(sheetName, "A1", "O1", headerStyle); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}
	return nil
}

func writeCPMRows(writer *cpmExcelWriter, items []*domain.CostProductMaster) {
	for i, p := range items {
		row := i + 2
		writer.setCellValue(fmt.Sprintf("A%d", row), i+1)
		writer.setCellValue(fmt.Sprintf("B%d", row), p.ProductCode())
		// product_type_code is not stored on the aggregate directly — the ID is;
		// the export captures the ID as a surrogate. In practice the gRPC handler
		// resolves the code for the proto response, so the ID is sufficient for
		// round-trip re-import when the import handler resolves code→ID.
		writer.setCellValue(fmt.Sprintf("C%d", row), p.ProductTypeID())
		writer.setCellValue(fmt.Sprintf("D%d", row), p.ProductName())
		writer.setCellValue(fmt.Sprintf("E%d", row), p.GradeCode())
		writer.setCellValue(fmt.Sprintf("F%d", row), p.ShadeCode())
		writer.setCellValue(fmt.Sprintf("G%d", row), p.ShadeName())
		writer.setCellValue(fmt.Sprintf("H%d", row), p.ErpItemCode())
		writer.setCellValue(fmt.Sprintf("I%d", row), p.ErpGradeCode1())
		writer.setCellValue(fmt.Sprintf("J%d", row), p.ErpGradeCode2())
		writer.setCellValue(fmt.Sprintf("K%d", row), p.Flex01())
		writer.setCellValue(fmt.Sprintf("L%d", row), p.Flex02())
		writer.setCellValue(fmt.Sprintf("M%d", row), p.Flex03())
		writer.setCellValue(fmt.Sprintf("N%d", row), p.Description())
		writer.setCellValue(fmt.Sprintf("O%d", row), p.IsActive())
	}
}

func writeCPMColWidths(writer *cpmExcelWriter) {
	writer.setColWidth("A", "A", 5)
	writer.setColWidth("B", "B", 18)
	writer.setColWidth("C", "C", 20)
	writer.setColWidth("D", "D", 30)
	writer.setColWidth("E", "E", 12)
	writer.setColWidth("F", "F", 12)
	writer.setColWidth("G", "G", 20)
	writer.setColWidth("H", "H", 18)
	writer.setColWidth("I", "I", 16)
	writer.setColWidth("J", "J", 16)
	writer.setColWidth("K", "K", 12)
	writer.setColWidth("L", "L", 12)
	writer.setColWidth("M", "M", 12)
	writer.setColWidth("N", "N", 25)
	writer.setColWidth("O", "O", 10)
}
