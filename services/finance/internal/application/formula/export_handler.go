// Package formula provides application layer handlers for Formula operations.
package formula

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/formula"
)

// ExportQuery represents the export Formulas query.
type ExportQuery struct {
	FormulaType *string
	IsActive    *bool
}

// ExportResult represents the export Formulas result.
type ExportResult struct {
	FileContent []byte
	FileName    string
}

// ExportHandler handles the ExportFormulas query.
type ExportHandler struct {
	repo formula.Repository
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(repo formula.Repository) *ExportHandler {
	return &ExportHandler{repo: repo}
}

// Handle executes the export Formulas query.
func (h *ExportHandler) Handle(ctx context.Context, query ExportQuery) (result *ExportResult, err error) {
	filter, err := buildFormulaExportFilter(query)
	if err != nil {
		return nil, err
	}

	// Get all formulas (without input params)
	formulas, err := h.repo.ListAll(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get formulas for export: %w", err)
	}

	// For each formula, load full data with input params
	fullFormulas := make([]*formula.Formula, 0, len(formulas))
	for _, f := range formulas {
		full, getErr := h.repo.GetByID(ctx, f.ID())
		if getErr != nil {
			log.Warn().Err(getErr).Str("id", f.ID().String()).Msg("Failed to load formula details for export")
			fullFormulas = append(fullFormulas, f)
			continue
		}
		fullFormulas = append(fullFormulas, full)
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

	sheetName := "Formulas"
	if err := setupFormulaExcelSheet(f, sheetName); err != nil {
		return nil, err
	}

	writer := &formulaExcelWriter{f: f, sheetName: sheetName}
	writeFormulaDataRows(writer, fullFormulas)
	writeFormulaColumnWidths(writer)

	if writer.hasErrors() {
		log.Warn().Err(writer.error()).Msg("Some Excel formatting operations failed")
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &ExportResult{
		FileContent: buffer.Bytes(),
		FileName:    "formula_export.xlsx",
	}, nil
}

func buildFormulaExportFilter(query ExportQuery) (formula.ExportFilter, error) {
	filter := formula.ExportFilter{}

	if query.FormulaType != nil {
		ft, err := formula.NewType(*query.FormulaType)
		if err != nil {
			return filter, err
		}
		filter.FormulaType = &ft
	}

	filter.IsActive = query.IsActive
	return filter, nil
}

func setupFormulaExcelSheet(f *excelize.File, sheetName string) error {
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	headers := []string{
		"No", "Code", "Name", "Type", "Expression", "Result Param Code",
		"Input Param Codes", "Description", "Version", "Active",
		"Created At", "Created By",
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
	if err := f.SetCellStyle(sheetName, "A1", "L1", headerStyle); err != nil {
		return fmt.Errorf("failed to set header style: %w", err)
	}

	return nil
}

func writeFormulaDataRows(writer *formulaExcelWriter, formulas []*formula.Formula) {
	for i, fm := range formulas {
		row := i + 2
		writer.setCellValue(fmt.Sprintf("A%d", row), i+1)
		writer.setCellValue(fmt.Sprintf("B%d", row), fm.Code().String())
		writer.setCellValue(fmt.Sprintf("C%d", row), fm.Name())
		writer.setCellValue(fmt.Sprintf("D%d", row), fm.FormulaType().String())
		writer.setCellValue(fmt.Sprintf("E%d", row), fm.Expression())
		writer.setCellValue(fmt.Sprintf("F%d", row), fm.ResultParamCode())

		// Build comma-separated input param codes
		paramCodes := ""
		for j, p := range fm.InputParams() {
			if j > 0 {
				paramCodes += ","
			}
			paramCodes += p.ParamCode()
		}
		writer.setCellValue(fmt.Sprintf("G%d", row), paramCodes)

		writer.setCellValue(fmt.Sprintf("H%d", row), fm.Description())
		writer.setCellValue(fmt.Sprintf("I%d", row), fm.Version())
		writer.setCellValue(fmt.Sprintf("J%d", row), fm.IsActive())
		writer.setCellValue(fmt.Sprintf("K%d", row), fm.CreatedAt().Format("2006-01-02 15:04:05"))
		writer.setCellValue(fmt.Sprintf("L%d", row), fm.CreatedBy())
	}
}

func writeFormulaColumnWidths(writer *formulaExcelWriter) {
	writer.setColWidth("A", "A", 5)
	writer.setColWidth("B", "B", 20)
	writer.setColWidth("C", "C", 25)
	writer.setColWidth("D", "D", 15)
	writer.setColWidth("E", "E", 40)
	writer.setColWidth("F", "F", 20)
	writer.setColWidth("G", "G", 30)
	writer.setColWidth("H", "H", 25)
	writer.setColWidth("I", "I", 10)
	writer.setColWidth("J", "J", 10)
	writer.setColWidth("K", "K", 20)
	writer.setColWidth("L", "L", 20)
}

// formulaExcelWriter wraps excelize file with error collection.
type formulaExcelWriter struct {
	f         *excelize.File
	sheetName string
	errs      []error
}

func (ew *formulaExcelWriter) setCellValue(cell string, value interface{}) {
	if err := ew.f.SetCellValue(ew.sheetName, cell, value); err != nil {
		ew.errs = append(ew.errs, fmt.Errorf("cell %s: %w", cell, err))
	}
}

func (ew *formulaExcelWriter) setColWidth(startCol, endCol string, width float64) {
	if err := ew.f.SetColWidth(ew.sheetName, startCol, endCol, width); err != nil {
		ew.errs = append(ew.errs, fmt.Errorf("column %s-%s: %w", startCol, endCol, err))
	}
}

func (ew *formulaExcelWriter) hasErrors() bool { return len(ew.errs) > 0 }

func (ew *formulaExcelWriter) error() error {
	if len(ew.errs) == 0 {
		return nil
	}
	return errors.Join(ew.errs...)
}
