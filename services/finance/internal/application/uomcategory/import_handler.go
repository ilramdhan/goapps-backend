// Package uomcategory provides application layer handlers for UOM Category operations.
package uomcategory

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uomcategory"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ImportCommand represents the import UOM Categories command.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string // "skip", "update", "error"
	CreatedBy       string
}

// ImportResult represents the import UOM Categories result.
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

// ImportHandler handles the ImportUOMCategories command.
type ImportHandler struct {
	repo uomcategory.Repository
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(repo uomcategory.Repository) *ImportHandler {
	return &ImportHandler{repo: repo}
}

// Handle executes the import UOM Categories command.
func (h *ImportHandler) Handle(ctx context.Context, cmd ImportCommand) (result *ImportResult, err error) {
	result = &ImportResult{
		Errors: []ImportError{},
	}

	// Validate file and get rows
	rows, err := h.parseExcelFile(cmd.FileContent, cmd.FileName)
	if err != nil {
		return nil, err
	}

	// Skip header row
	if len(rows) <= 1 {
		return result, nil // No data rows
	}

	// Process each row
	for i, row := range rows[1:] {
		rowNum := safeconv.IntToInt32(i + 2) // 1-indexed, skip header
		h.processRow(ctx, row, rowNum, cmd, result)
	}

	return result, nil
}

// parseExcelFile opens and validates the Excel file, returning rows.
func (h *ImportHandler) parseExcelFile(content []byte, fileName string) ([][]string, error) {
	// Validate file extension
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext != ".xlsx" && ext != ".xls" {
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}

	// Open Excel file
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to open excel file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close Excel file")
		}
	}()

	// Get first sheet
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found in file")
	}

	// Get all rows
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("failed to get rows: %w", err)
	}

	return rows, nil
}

// importRowData holds parsed row values.
type importRowData struct {
	code        string
	name        string
	description string
}

// parseImportRow extracts cell values from a row.
func parseImportRow(row []string) importRowData {
	return importRowData{
		code:        getCellValue(row, 0),
		name:        getCellValue(row, 1),
		description: getCellValue(row, 2),
	}
}

// processRow handles a single row import.
func (h *ImportHandler) processRow(ctx context.Context, row []string, rowNum int32, cmd ImportCommand, result *ImportResult) {
	data := parseImportRow(row)

	// Validate fields
	code, err := h.validateImportRow(data, rowNum, result)
	if err != nil {
		return // Error already recorded in result
	}

	// Check for duplicates
	exists, err := h.repo.ExistsByCode(ctx, code)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "category_code",
			Message:   fmt.Sprintf("failed to check duplicate: %v", err),
		})
		return
	}

	if exists {
		h.handleDuplicate(ctx, code, data, rowNum, cmd, result)
		return
	}

	// Create new category
	h.createCategory(ctx, code, data, rowNum, cmd.CreatedBy, result)
}

// validateImportRow validates the row data and returns domain objects.
func (h *ImportHandler) validateImportRow(data importRowData, rowNum int32, result *ImportResult) (uomcategory.Code, error) {
	// Validate code
	code, err := uomcategory.NewCode(data.code)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "category_code",
			Message:   err.Error(),
		})
		return uomcategory.Code{}, err
	}

	// Validate name
	if data.name == "" {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "category_name",
			Message:   "name cannot be empty",
		})
		return uomcategory.Code{}, fmt.Errorf("name cannot be empty")
	}

	return code, nil
}

// handleDuplicate handles a duplicate code based on the specified action.
func (h *ImportHandler) handleDuplicate(ctx context.Context, code uomcategory.Code, data importRowData, rowNum int32, cmd ImportCommand, result *ImportResult) {
	switch cmd.DuplicateAction {
	case "skip":
		result.SkippedCount++
	case "update":
		h.updateExisting(ctx, code, data, rowNum, cmd.CreatedBy, result)
	case "error":
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "category_code",
			Message:   "duplicate code already exists",
		})
	default:
		// Unknown action, treat as skip
		result.SkippedCount++
	}
}

// updateExisting updates an existing UOM Category.
func (h *ImportHandler) updateExisting(ctx context.Context, code uomcategory.Code, data importRowData, rowNum int32, updatedBy string, result *ImportResult) {
	existing, err := h.repo.GetByCode(ctx, code)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "category_code",
			Message:   fmt.Sprintf("failed to get existing: %v", err),
		})
		return
	}

	if err := existing.Update(&data.name, &data.description, nil, updatedBy); err != nil {
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

// createCategory creates a new UOM Category.
func (h *ImportHandler) createCategory(ctx context.Context, code uomcategory.Code, data importRowData, rowNum int32, createdBy string, result *ImportResult) {
	entity, err := uomcategory.NewCategory(code, data.name, data.description, createdBy)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "create",
			Message:   err.Error(),
		})
		return
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

// getCellValue safely gets a cell value from a row.
func getCellValue(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}
