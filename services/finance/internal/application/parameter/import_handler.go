// Package parameter provides application layer handlers for Parameter operations.
package parameter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ImportCommand represents the import Parameters command.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string // "skip", "update", "error"
	CreatedBy       string
}

// ImportResult represents the import Parameters result.
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

// ImportHandler handles the ImportParameters command.
type ImportHandler struct {
	repo parameter.Repository
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(repo parameter.Repository) *ImportHandler {
	return &ImportHandler{repo: repo}
}

// Handle executes the import Parameters command.
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

// paramRowData holds parsed row values.
type paramRowData struct {
	code          string
	name          string
	shortName     string
	dataType      string
	paramCategory string
	uomCode       string
	defaultValue  string
	minValue      string
	maxValue      string
}

// parseParamRow extracts cell values from a row.
func parseParamRow(row []string) paramRowData {
	return paramRowData{
		code:          getParamCell(row, 0),
		name:          getParamCell(row, 1),
		shortName:     getParamCell(row, 2),
		dataType:      getParamCell(row, 3),
		paramCategory: getParamCell(row, 4),
		uomCode:       getParamCell(row, 5),
		defaultValue:  getParamCell(row, 6),
		minValue:      getParamCell(row, 7),
		maxValue:      getParamCell(row, 8),
	}
}

// processRow handles a single row import.
func (h *ImportHandler) processRow(ctx context.Context, row []string, rowNum int32, cmd ImportCommand, result *ImportResult) {
	data := parseParamRow(row)

	code, dataType, paramCategory, err := h.validateRowData(data, rowNum, result)
	if err != nil {
		return
	}

	exists, err := h.repo.ExistsByCode(ctx, code)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "param_code",
			Message:   fmt.Sprintf("failed to check duplicate: %v", err),
		})
		return
	}

	if exists {
		h.handleDuplicate(ctx, code, data, rowNum, cmd, result)
		return
	}

	h.createParameter(ctx, code, dataType, paramCategory, data, rowNum, cmd.CreatedBy, result)
}

// validateRowData validates the row data and returns domain objects.
func (h *ImportHandler) validateRowData(data paramRowData, rowNum int32, result *ImportResult) (parameter.Code, parameter.DataType, parameter.ParamCategory, error) {
	code, err := parameter.NewCode(data.code)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "param_code", Message: err.Error()})
		return parameter.Code{}, "", "", err
	}

	if data.name == "" {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "param_name", Message: "name cannot be empty"})
		return parameter.Code{}, "", "", fmt.Errorf("name cannot be empty")
	}

	dataType, err := parameter.NewDataType(data.dataType)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "data_type", Message: err.Error()})
		return parameter.Code{}, "", "", err
	}

	paramCategory, err := parameter.NewParamCategory(data.paramCategory)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "param_category", Message: err.Error()})
		return parameter.Code{}, "", "", err
	}

	return code, dataType, paramCategory, nil
}

// handleDuplicate handles a duplicate code based on the specified action.
func (h *ImportHandler) handleDuplicate(ctx context.Context, code parameter.Code, data paramRowData, rowNum int32, cmd ImportCommand, result *ImportResult) {
	switch cmd.DuplicateAction {
	case "skip":
		result.SkippedCount++
	case "update":
		h.updateExisting(ctx, code, data, rowNum, cmd.CreatedBy, result)
	case "error":
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "param_code", Message: "duplicate code already exists"})
	default:
		result.SkippedCount++
	}
}

// updateExisting updates an existing Parameter.
func (h *ImportHandler) updateExisting(ctx context.Context, code parameter.Code, data paramRowData, rowNum int32, updatedBy string, result *ImportResult) {
	existing, err := h.repo.GetByCode(ctx, code)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "param_code", Message: fmt.Sprintf("failed to get existing: %v", err)})
		return
	}

	dataType, err := parameter.NewDataType(data.dataType)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "data_type", Message: err.Error()})
		return
	}

	paramCategory, err := parameter.NewParamCategory(data.paramCategory)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "param_category", Message: err.Error()})
		return
	}

	// Prepare optional fields with double-pointer pattern
	var defaultValue, minValue, maxValue **string
	if data.defaultValue != "" {
		dv := data.defaultValue
		dvp := &dv
		defaultValue = &dvp
	}
	if data.minValue != "" {
		mv := data.minValue
		mvp := &mv
		minValue = &mvp
	}
	if data.maxValue != "" {
		xv := data.maxValue
		xvp := &xv
		maxValue = &xvp
	}

	// Resolve UOM code to UUID if provided
	var uomIDPtr **uuid.UUID
	if data.uomCode != "" {
		resolvedID, resolveErr := h.repo.ResolveUOMCode(ctx, data.uomCode)
		if errors.Is(resolveErr, parameter.ErrUOMNotFound) {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "uom_code", Message: fmt.Sprintf("UOM code '%s' not found", data.uomCode)})
			return
		}
		if resolveErr != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "uom_code", Message: fmt.Sprintf("failed to resolve UOM code: %v", resolveErr)})
			return
		}
		uomIDPtr = &resolvedID
	}

	if err := existing.Update(&data.name, &data.shortName, &dataType, &paramCategory, uomIDPtr, defaultValue, minValue, maxValue, nil, updatedBy); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "update", Message: err.Error()})
		return
	}

	if err := h.repo.Update(ctx, existing); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "update", Message: fmt.Sprintf("failed to update: %v", err)})
		return
	}

	result.UpdatedCount++
}

// createParameter creates a new Parameter.
func (h *ImportHandler) createParameter(
	ctx context.Context,
	code parameter.Code,
	dataType parameter.DataType,
	paramCategory parameter.ParamCategory,
	data paramRowData,
	rowNum int32,
	createdBy string,
	result *ImportResult,
) {
	var defaultValue, minValue, maxValue *string
	if data.defaultValue != "" {
		defaultValue = &data.defaultValue
	}
	if data.minValue != "" {
		minValue = &data.minValue
	}
	if data.maxValue != "" {
		maxValue = &data.maxValue
	}

	// Resolve UOM code to UUID if provided
	var uomID *uuid.UUID
	if data.uomCode != "" {
		resolvedID, resolveErr := h.repo.ResolveUOMCode(ctx, data.uomCode)
		if errors.Is(resolveErr, parameter.ErrUOMNotFound) {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "uom_code", Message: fmt.Sprintf("UOM code '%s' not found", data.uomCode)})
			return
		}
		if resolveErr != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "uom_code", Message: fmt.Sprintf("failed to resolve UOM code: %v", resolveErr)})
			return
		}
		uomID = resolvedID
	}

	entity, err := parameter.NewParameter(
		code, data.name, data.shortName,
		dataType, paramCategory, uomID,
		defaultValue, minValue, maxValue,
		createdBy,
	)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "create", Message: err.Error()})
		return
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "create", Message: fmt.Sprintf("failed to create: %v", err)})
		return
	}

	result.SuccessCount++
}

// getParamCell safely gets a cell value from a row.
func getParamCell(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}
