// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ImportCommand represents the import MB Heads command.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string // "skip", "update", "error"
	CreatedBy       string
}

// ImportResult represents the import MB Heads result. Note: the proto response has no
// UpdatedCount field, so updated duplicate rows are folded into SuccessCount.
type ImportResult struct {
	SuccessCount int32
	SkippedCount int32
	FailedCount  int32
	Errors       []ImportError
}

// ImportError represents a single import error.
type ImportError struct {
	RowNumber int32
	Field     string
	Message   string
}

// ImportHandler handles the ImportMBHeads command.
type ImportHandler struct {
	repo mbhead.Repository
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(repo mbhead.Repository) *ImportHandler {
	return &ImportHandler{repo: repo}
}

// Handle executes the import MB Heads command.
func (h *ImportHandler) Handle(ctx context.Context, cmd ImportCommand) (result *ImportResult, err error) {
	result = &ImportResult{Errors: []ImportError{}}

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

// mbHeadRowData holds parsed row values, matching mbHeadTemplateHeaders column order.
type mbHeadRowData struct {
	mbCosting    string
	mgtName      string
	devCode      string
	shadeCode    string
	shadeName    string
	crossSection string
	lustureCode  string
	denier       string
	filament     string
	dozing       string
	isBoughtout  string
}

func parseMBHeadRow(row []string) mbHeadRowData {
	return mbHeadRowData{
		mbCosting:    getCell(row, 0),
		mgtName:      getCell(row, 1),
		devCode:      getCell(row, 2),
		shadeCode:    getCell(row, 3),
		shadeName:    getCell(row, 4),
		crossSection: getCell(row, 5),
		lustureCode:  getCell(row, 6),
		denier:       getCell(row, 7),
		filament:     getCell(row, 8),
		dozing:       getCell(row, 9),
		isBoughtout:  getCell(row, 10),
	}
}

// getCell safely gets a cell value from a row.
func getCell(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}

// processRow handles a single row import.
func (h *ImportHandler) processRow(ctx context.Context, row []string, rowNum int32, cmd ImportCommand, result *ImportResult) {
	data := parseMBHeadRow(row)

	if data.mbCosting == "" {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "mb_costing",
			Message:   "mb costing cannot be empty",
		})
		return
	}

	denier, filament, dozing, isBoughtout, parseErr := parseMBHeadNumericFields(data)
	if parseErr != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "numeric_fields",
			Message:   parseErr.Error(),
		})
		return
	}

	exists, err := h.repo.ExistsByMBCosting(ctx, data.mbCosting)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "mb_costing",
			Message:   fmt.Sprintf("failed to check duplicate: %v", err),
		})
		return
	}

	if exists {
		h.handleDuplicate(ctx, data, denier, filament, dozing, isBoughtout, rowNum, cmd, result)
		return
	}

	h.createMBHead(ctx, data, denier, filament, dozing, isBoughtout, rowNum, cmd.CreatedBy, result)
}

// parseMBHeadNumericFields parses the optional numeric/boolean fields from a row.
func parseMBHeadNumericFields(data mbHeadRowData) (denier *float64, filament *int, dozing *float64, isBoughtout bool, err error) {
	if data.denier != "" {
		v, parseErr := strconv.ParseFloat(data.denier, 64)
		if parseErr != nil {
			return nil, nil, nil, false, fmt.Errorf("invalid denier %q: %w", data.denier, parseErr)
		}
		denier = &v
	}
	if data.filament != "" {
		v, parseErr := strconv.Atoi(data.filament)
		if parseErr != nil {
			return nil, nil, nil, false, fmt.Errorf("invalid filament %q: %w", data.filament, parseErr)
		}
		filament = &v
	}
	if data.dozing != "" {
		v, parseErr := strconv.ParseFloat(data.dozing, 64)
		if parseErr != nil {
			return nil, nil, nil, false, fmt.Errorf("invalid dozing %q: %w", data.dozing, parseErr)
		}
		dozing = &v
	}
	if data.isBoughtout != "" {
		v, parseErr := strconv.ParseBool(data.isBoughtout)
		if parseErr != nil {
			return nil, nil, nil, false, fmt.Errorf("invalid is bought out %q: %w", data.isBoughtout, parseErr)
		}
		isBoughtout = v
	}
	return denier, filament, dozing, isBoughtout, nil
}

// handleDuplicate handles a duplicate mb_costing based on the specified action.
func (h *ImportHandler) handleDuplicate(
	ctx context.Context, data mbHeadRowData, denier *float64, filament *int, dozing *float64,
	isBoughtout bool, rowNum int32, cmd ImportCommand, result *ImportResult,
) {
	switch cmd.DuplicateAction {
	case "skip":
		result.SkippedCount++
	case "update":
		h.updateExisting(ctx, data, denier, filament, dozing, isBoughtout, rowNum, cmd.CreatedBy, result)
	case "error":
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "mb_costing",
			Message:   "duplicate mb costing already exists",
		})
	default:
		result.SkippedCount++
	}
}

// updateExisting updates an existing MB Head.
func (h *ImportHandler) updateExisting(
	ctx context.Context, data mbHeadRowData, denier *float64, filament *int, dozing *float64,
	isBoughtout bool, rowNum int32, updatedBy string, result *ImportResult,
) {
	existing, err := h.repo.GetByMBCosting(ctx, data.mbCosting)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "mb_costing",
			Message:   fmt.Sprintf("failed to get existing: %v", err),
		})
		return
	}

	update := mbhead.UpdateInput{
		MgtName:      strPtrOrNil(data.mgtName),
		DevCode:      &data.devCode,
		ShadeCode:    &data.shadeCode,
		ShadeName:    &data.shadeName,
		CrossSection: &data.crossSection,
		LustureCode:  &data.lustureCode,
		Denier:       denier,
		Filament:     filament,
		Dozing:       dozing,
		IsActive:     &isBoughtout,
	}
	// IsActive above is a placeholder overwrite bug guard: bought-out status is not is_active.
	update.IsActive = nil

	if err := existing.Update(update, updatedBy); err != nil {
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

	result.SuccessCount++
}

// createMBHead creates a new MB Head.
func (h *ImportHandler) createMBHead(
	ctx context.Context, data mbHeadRowData, denier *float64, filament *int, dozing *float64,
	isBoughtout bool, rowNum int32, createdBy string, result *ImportResult,
) {
	entity, err := mbhead.New(
		data.mbCosting, nil, strPtrOrNil(data.mgtName),
		denier, filament, dozing,
		nil, nil, nil, nil, nil, nil,
		createdBy, isBoughtout, data.devCode, data.shadeCode, data.shadeName,
		data.crossSection, data.lustureCode, nil,
	)
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

// strPtrOrNil returns nil for an empty string, else a pointer to the string.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
