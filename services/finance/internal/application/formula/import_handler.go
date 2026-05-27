// Package formula provides application layer handlers for Formula operations.
package formula

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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/formula"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ImportCommand represents the import Formulas command.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string
	CreatedBy       string
}

// ImportResult represents the import Formulas result.
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

// ImportHandler handles the ImportFormulas command.
type ImportHandler struct {
	repo formula.Repository
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(repo formula.Repository) *ImportHandler {
	return &ImportHandler{repo: repo}
}

// Handle executes the import Formulas command.
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

type formulaRowData struct {
	code            string
	name            string
	formulaType     string
	expression      string
	resultParamCode string
	inputParamCodes string
	description     string
}

func parseFormulaRow(row []string) formulaRowData {
	return formulaRowData{
		code:            getFormulaCell(row, 0),
		name:            getFormulaCell(row, 1),
		formulaType:     getFormulaCell(row, 2),
		expression:      getFormulaCell(row, 3),
		resultParamCode: getFormulaCell(row, 4),
		inputParamCodes: getFormulaCell(row, 5),
		description:     getFormulaCell(row, 6),
	}
}

func (h *ImportHandler) processRow(ctx context.Context, row []string, rowNum int32, cmd ImportCommand, result *ImportResult) {
	data := parseFormulaRow(row)

	code, err := formula.NewCode(data.code)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "formula_code", Message: err.Error()})
		return
	}

	if data.name == "" {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "formula_name", Message: "name cannot be empty"})
		return
	}

	_, err = formula.NewType(data.formulaType)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "formula_type", Message: err.Error()})
		return
	}

	if data.expression == "" {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "expression", Message: "expression cannot be empty"})
		return
	}

	// Resolve result param code
	resultParamID, err := h.repo.ResolveParamCode(ctx, data.resultParamCode)
	if err != nil {
		if errors.Is(err, formula.ErrInputParamNotFound) {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "result_param_code", Message: fmt.Sprintf("result param '%s' not found", data.resultParamCode)})
			return
		}
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "result_param_code", Message: err.Error()})
		return
	}

	// Resolve input param codes
	inputParamIDs, err := h.resolveInputParamCodes(ctx, data.inputParamCodes, rowNum, result)
	if err != nil {
		return // error already added to result
	}

	// Check duplicate
	exists, err := h.repo.ExistsByCode(ctx, code)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "formula_code", Message: fmt.Sprintf("failed to check duplicate: %v", err)})
		return
	}

	if exists {
		h.handleDuplicate(ctx, code, data, *resultParamID, inputParamIDs, rowNum, cmd, result)
		return
	}

	// Check result param not used by another formula
	used, usedErr := h.repo.ResultParamUsedByOther(ctx, *resultParamID, uuid.Nil)
	if usedErr != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "result_param_code", Message: fmt.Sprintf("failed to check result param: %v", usedErr)})
		return
	}
	if used {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "result_param_code", Message: "result parameter already used by another formula"})
		return
	}

	h.createFormula(ctx, code, data, *resultParamID, inputParamIDs, rowNum, cmd.CreatedBy, result)
}

func (h *ImportHandler) resolveInputParamCodes(ctx context.Context, codesStr string, rowNum int32, result *ImportResult) ([]uuid.UUID, error) {
	if codesStr == "" {
		return nil, nil
	}

	codes := strings.Split(codesStr, ",")
	ids := make([]uuid.UUID, 0, len(codes))

	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		id, err := h.repo.ResolveParamCode(ctx, code)
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "input_param_codes",
				Message:   fmt.Sprintf("input param '%s' not found", code),
			})
			return nil, err
		}
		ids = append(ids, *id)
	}

	return ids, nil
}

func (h *ImportHandler) handleDuplicate(ctx context.Context, code formula.Code, data formulaRowData, resultParamID uuid.UUID, inputParamIDs []uuid.UUID, rowNum int32, cmd ImportCommand, result *ImportResult) {
	switch cmd.DuplicateAction {
	case "skip":
		result.SkippedCount++
	case "update":
		h.updateExisting(ctx, code, data, resultParamID, inputParamIDs, rowNum, cmd.CreatedBy, result)
	case "error":
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "formula_code", Message: "duplicate code already exists"})
	default:
		result.SkippedCount++
	}
}

func (h *ImportHandler) updateExisting(ctx context.Context, code formula.Code, data formulaRowData, resultParamID uuid.UUID, inputParamIDs []uuid.UUID, rowNum int32, updatedBy string, result *ImportResult) {
	existing, err := h.repo.GetByCode(ctx, code)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "formula_code", Message: fmt.Sprintf("failed to get existing: %v", err)})
		return
	}

	ft, err := formula.NewType(data.formulaType)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "formula_type", Message: err.Error()})
		return
	}

	// Check result param not used by another formula (excluding this one)
	used, usedErr := h.repo.ResultParamUsedByOther(ctx, resultParamID, existing.ID())
	if usedErr != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "result_param_code", Message: fmt.Sprintf("failed to check result param: %v", usedErr)})
		return
	}
	if used {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "result_param_code", Message: "result parameter already used by another formula"})
		return
	}

	rpID := resultParamID
	if err := existing.Update(&data.name, &ft, &data.expression, &rpID, inputParamIDs, &data.description, nil, updatedBy); err != nil {
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

func (h *ImportHandler) createFormula(ctx context.Context, code formula.Code, data formulaRowData, resultParamID uuid.UUID, inputParamIDs []uuid.UUID, rowNum int32, createdBy string, result *ImportResult) {
	ft, err := formula.NewType(data.formulaType)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "formula_type", Message: err.Error()})
		return
	}

	entity, err := formula.NewFormula(code, data.name, ft, data.expression, resultParamID, inputParamIDs, data.description, createdBy)
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

func getFormulaCell(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}
