// Package employeegroup provides application layer handlers for Employee Group operations.
package employeegroup

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeegroup"
	"github.com/mutugading/goapps-backend/services/shared/excel"
)

const (
	codeField   = "code"
	createField = "create"
	updateField = "update"
)

// ImportCommand represents the import employee groups command.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string // "skip", "update", "error".
	CreatedBy       string
}

// ImportResult summarizes import outcomes.
type ImportResult struct {
	SuccessCount int32
	SkippedCount int32
	UpdatedCount int32
	FailedCount  int32
	Errors       []excel.ImportError
}

// ImportHandler handles the import employee groups command.
type ImportHandler struct {
	repo employeegroup.Repository
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(repo employeegroup.Repository) *ImportHandler {
	return &ImportHandler{repo: repo}
}

// Handle executes the import command.
func (h *ImportHandler) Handle(ctx context.Context, cmd ImportCommand) (*ImportResult, error) {
	result := &ImportResult{Errors: []excel.ImportError{}}

	rows, err := excel.ParseFile(cmd.FileContent, cmd.FileName)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return result, nil
	}

	for _, row := range rows {
		h.processRow(ctx, row, cmd, result)
	}
	return result, nil
}

func (h *ImportHandler) processRow(ctx context.Context, row excel.ParsedRow, cmd ImportCommand, result *ImportResult) {
	code, name, ok := parseImportRow(row, result)
	if !ok {
		return
	}

	exists, err := h.repo.ExistsByCode(ctx, code.String())
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{
			RowNumber: row.RowNumber, Field: codeField,
			Message: fmt.Sprintf("check duplicate: %v", err),
		})
		return
	}

	if exists {
		h.handleDuplicate(ctx, code, name, row.RowNumber, cmd, result)
		return
	}

	entity, err := employeegroup.NewEmployeeGroup(code, name, cmd.CreatedBy)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{
			RowNumber: row.RowNumber, Field: createField, Message: err.Error(),
		})
		return
	}
	if err := h.repo.Create(ctx, entity); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{
			RowNumber: row.RowNumber, Field: createField,
			Message: fmt.Sprintf("failed to create: %v", err),
		})
		return
	}
	result.SuccessCount++
}

func (h *ImportHandler) handleDuplicate(
	ctx context.Context, code employeegroup.Code,
	name string, rowNum int32, cmd ImportCommand, result *ImportResult,
) {
	switch cmd.DuplicateAction {
	case "skip":
		result.SkippedCount++
	case "update":
		existing, err := h.repo.GetByCode(ctx, code.String())
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, excel.ImportError{
				RowNumber: rowNum, Field: codeField,
				Message: fmt.Sprintf("failed to get existing: %v", err),
			})
			return
		}
		if err := existing.Update(&name, nil, cmd.CreatedBy); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, excel.ImportError{
				RowNumber: rowNum, Field: updateField, Message: err.Error(),
			})
			return
		}
		if err := h.repo.Update(ctx, existing); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, excel.ImportError{
				RowNumber: rowNum, Field: updateField,
				Message: fmt.Sprintf("failed to update: %v", err),
			})
			return
		}
		result.UpdatedCount++
	case "error":
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{
			RowNumber: rowNum, Field: codeField, Message: "duplicate code already exists",
		})
	default:
		result.SkippedCount++
	}
}

// parseImportRow extracts and validates a single row.
// Columns: Code(0) | Name(1).
func parseImportRow(row excel.ParsedRow, result *ImportResult) (employeegroup.Code, string, bool) {
	rn := row.RowNumber

	code, err := employeegroup.NewCode(row.Cell(0))
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{RowNumber: rn, Field: codeField, Message: err.Error()})
		return employeegroup.Code{}, "", false
	}

	name := row.Cell(1)
	if name == "" {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{RowNumber: rn, Field: "name", Message: "name is required"})
		return employeegroup.Code{}, "", false
	}

	return code, name, true
}
