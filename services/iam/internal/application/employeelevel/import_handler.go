// Package employeelevel provides application layer handlers for Employee Level operations.
package employeelevel

import (
	"context"
	"fmt"
	"strconv"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeelevel"
	"github.com/mutugading/goapps-backend/services/shared/excel"
)

// ImportCommand represents the import employee levels command.
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

// ImportHandler handles the import employee levels command.
type ImportHandler struct {
	repo employeelevel.Repository
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(repo employeelevel.Repository) *ImportHandler {
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
	code, name, grade, typ, seq, wf, ok := parseImportRow(row, result)
	if !ok {
		return
	}

	exists, err := h.repo.ExistsByCode(ctx, code.String())
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{
			RowNumber: row.RowNumber, Field: "code",
			Message: fmt.Sprintf("check duplicate: %v", err),
		})
		return
	}

	if exists {
		h.handleDuplicate(ctx, code, name, grade, typ, seq, wf, row.RowNumber, cmd, result)
		return
	}

	entity, err := employeelevel.NewEmployeeLevel(code, name, grade, typ, seq, wf, cmd.CreatedBy)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{
			RowNumber: row.RowNumber, Field: "create", Message: err.Error(),
		})
		return
	}
	if err := h.repo.Create(ctx, entity); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{
			RowNumber: row.RowNumber, Field: "create",
			Message: fmt.Sprintf("failed to create: %v", err),
		})
		return
	}
	result.SuccessCount++
}

func (h *ImportHandler) handleDuplicate(
	ctx context.Context, code employeelevel.Code,
	name string, grade int32, typ employeelevel.Type, seq int32, wf employeelevel.Workflow,
	rowNum int32, cmd ImportCommand, result *ImportResult,
) {
	switch cmd.DuplicateAction {
	case "skip":
		result.SkippedCount++
	case "update":
		existing, err := h.repo.GetByCode(ctx, code.String())
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, excel.ImportError{
				RowNumber: rowNum, Field: "code",
				Message: fmt.Sprintf("failed to get existing: %v", err),
			})
			return
		}
		if err := existing.Update(&name, &grade, &typ, &seq, &wf, nil, cmd.CreatedBy); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, excel.ImportError{
				RowNumber: rowNum, Field: "update", Message: err.Error(),
			})
			return
		}
		if err := h.repo.Update(ctx, existing); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, excel.ImportError{
				RowNumber: rowNum, Field: "update",
				Message: fmt.Sprintf("failed to update: %v", err),
			})
			return
		}
		result.UpdatedCount++
	case "error":
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{
			RowNumber: rowNum, Field: "code", Message: "duplicate code already exists",
		})
	default:
		result.SkippedCount++
	}
}

// parseImportRow extracts and validates a single row.
// Columns: Code(0) | Name(1) | Grade(2) | Type(3) | Sequence(4) | Workflow(5).
func parseImportRow(row excel.ParsedRow, result *ImportResult) (
	employeelevel.Code, string, int32, employeelevel.Type, int32, employeelevel.Workflow, bool,
) {
	rn := row.RowNumber

	code, err := employeelevel.NewCode(row.Cell(0))
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{RowNumber: rn, Field: "code", Message: err.Error()})
		return employeelevel.Code{}, "", 0, 0, 0, 0, false
	}

	name := row.Cell(1)
	if name == "" {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{RowNumber: rn, Field: "name", Message: "name is required"})
		return employeelevel.Code{}, "", 0, 0, 0, 0, false
	}

	grade, err := strconv.ParseInt(row.Cell(2), 10, 32)
	if err != nil || grade < 0 || grade > 99 {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{RowNumber: rn, Field: "grade", Message: "grade must be 0-99"})
		return employeelevel.Code{}, "", 0, 0, 0, 0, false
	}

	typ := employeelevel.ParseType(row.Cell(3))
	if !typ.IsValid() {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{RowNumber: rn, Field: "type", Message: "invalid type (EXECUTIVE, NON_EXECUTIVE, OPERATOR, OTHER)"})
		return employeelevel.Code{}, "", 0, 0, 0, 0, false
	}

	seq, err := strconv.ParseInt(row.Cell(4), 10, 32)
	if err != nil || seq < 0 || seq > 999 {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{RowNumber: rn, Field: "sequence", Message: "sequence must be 0-999"})
		return employeelevel.Code{}, "", 0, 0, 0, 0, false
	}

	wf := employeelevel.ParseWorkflow(row.Cell(5))
	if !wf.IsValid() {
		result.FailedCount++
		result.Errors = append(result.Errors, excel.ImportError{RowNumber: rn, Field: "workflow", Message: "invalid workflow (DRAFT, RELEASED, SUPER_USER)"})
		return employeelevel.Code{}, "", 0, 0, 0, 0, false
	}

	// grade and seq are bounds-checked above (0-99 and 0-999, well within int32 range).
	return code, name, int32(grade), typ, int32(seq), wf, true //nolint:gosec // bounds checked above
}
