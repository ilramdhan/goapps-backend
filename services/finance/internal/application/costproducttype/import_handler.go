// Package costproducttype contains application use cases for CostProductType.
package costproducttype

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ImportCommand represents the import CostProductTypes command.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string // "skip", "update", "error"
}

// ImportResult represents the import CostProductTypes result.
type ImportResult struct {
	SuccessCount int32
	SkippedCount int32
	UpdatedCount int32
	FailedCount  int32
	Errors       []ImportError
}

// ImportError represents a single import error.
type ImportError struct {
	RowNumber int32
	Field     string
	Message   string
}

// ImportHandler handles the ImportCostProductTypes command.
type ImportHandler struct {
	repo domain.Repository
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(repo domain.Repository) *ImportHandler {
	return &ImportHandler{repo: repo}
}

// Handle executes the import CostProductTypes command.
func (h *ImportHandler) Handle(ctx context.Context, cmd ImportCommand) (result *ImportResult, err error) {
	result = &ImportResult{
		Errors: []ImportError{},
	}

	rows, err := h.parseExcelFile(cmd.FileContent, cmd.FileName)
	if err != nil {
		return nil, err
	}

	if len(rows) <= 1 {
		return result, nil
	}

	for i, row := range rows[1:] {
		rowNum := safeconv.IntToInt32(i + 2)
		h.processRow(ctx, row, rowNum, cmd, result)
	}

	return result, nil
}

// parseExcelFile opens and validates the Excel file, returning rows.
func (h *ImportHandler) parseExcelFile(content []byte, fileName string) ([][]string, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext != ".xlsx" && ext != ".xls" {
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}

	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to open excel file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close Excel file")
		}
	}()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found in file")
	}

	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("failed to get rows: %w", err)
	}

	return rows, nil
}

// cptRowData holds parsed row values.
type cptRowData struct {
	typeCode string
	typeName string
	isActive string
}

// parseCPTRow extracts cell values from a row.
func parseCPTRow(row []string) cptRowData {
	return cptRowData{
		typeCode: getCPTCell(row, 0),
		typeName: getCPTCell(row, 1),
		isActive: getCPTCell(row, 2),
	}
}

// processRow handles a single row import.
func (h *ImportHandler) processRow(ctx context.Context, row []string, rowNum int32, cmd ImportCommand, result *ImportResult) {
	data := parseCPTRow(row)

	if data.typeCode == "" {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "type_code",
			Message:   "type_code cannot be empty",
		})
		return
	}

	if data.typeName == "" {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "type_name",
			Message:   "type_name cannot be empty",
		})
		return
	}

	existing, err := h.repo.GetByCode(ctx, data.typeCode)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "type_code",
			Message:   fmt.Sprintf("failed to check duplicate: %v", err),
		})
		return
	}

	if existing != nil {
		h.handleDuplicate(ctx, existing, data, rowNum, cmd, result)
		return
	}

	h.createType(ctx, data, rowNum, result)
}

// handleDuplicate handles a duplicate code based on the specified action.
func (h *ImportHandler) handleDuplicate(ctx context.Context, existing *domain.CostProductType, data cptRowData, rowNum int32, cmd ImportCommand, result *ImportResult) {
	switch cmd.DuplicateAction {
	case "skip":
		result.SkippedCount++
	case "update":
		h.updateExisting(ctx, existing, data, rowNum, result)
	case "error":
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "type_code",
			Message:   "duplicate code already exists",
		})
	default:
		result.SkippedCount++
	}
}

// updateExisting updates an existing CostProductType.
func (h *ImportHandler) updateExisting(ctx context.Context, existing *domain.CostProductType, data cptRowData, rowNum int32, result *ImportResult) {
	isActive := parseIsActive(data.isActive, existing.IsActive())

	if err := existing.Update(data.typeName, isActive, ""); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "update",
			Message:   err.Error(),
		})
		return
	}

	if err := h.repo.Update(ctx, existing); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "update",
			Message:   fmt.Sprintf("failed to update: %v", err),
		})
		return
	}

	result.UpdatedCount++
}

// createType creates a new CostProductType.
func (h *ImportHandler) createType(ctx context.Context, data cptRowData, rowNum int32, result *ImportResult) {
	entity, err := domain.New(data.typeCode, data.typeName, "")
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "create",
			Message:   err.Error(),
		})
		return
	}

	if data.isActive != "" {
		isActive := parseIsActive(data.isActive, true)
		if err := entity.Update(data.typeName, isActive, ""); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "is_active",
				Message:   err.Error(),
			})
			return
		}
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "create",
			Message:   fmt.Sprintf("failed to create: %v", err),
		})
		return
	}

	result.SuccessCount++
}

// parseIsActive parses an optional is_active cell value; returns defaultVal if empty.
func parseIsActive(raw string, defaultVal bool) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "true", "yes", "1", "active":
		return true
	case "false", "no", "0", "inactive":
		return false
	default:
		return defaultVal
	}
}

// getCPTCell safely gets a cell value from a row.
func getCPTCell(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}
