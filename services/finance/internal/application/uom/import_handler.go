// Package uom provides application layer handlers for UOM operations.
package uom

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

// ImportCommand represents the import UOMs command.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string // "skip", "update", "error"
	CreatedBy       string
}

// ImportResult represents the import UOMs result.
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

// ImportHandler handles the ImportUOMs command.
type ImportHandler struct {
	repo uom.Repository
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(repo uom.Repository) *ImportHandler {
	return &ImportHandler{repo: repo}
}

// Handle executes the import UOMs command.
func (h *ImportHandler) Handle(ctx context.Context, cmd ImportCommand) (*ImportResult, error) {
	result := &ImportResult{
		Errors: []ImportError{},
	}

	// Open Excel file
	f, err := excelize.OpenReader(bytes.NewReader(cmd.FileContent))
	if err != nil {
		return nil, fmt.Errorf("failed to open excel file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Get sheet name based on file extension
	ext := strings.ToLower(filepath.Ext(cmd.FileName))
	if ext != ".xlsx" && ext != ".xls" {
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}

	// Get first sheet
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found in file")
	}
	sheetName := sheets[0]

	// Get all rows
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to get rows: %w", err)
	}

	// Skip header row
	if len(rows) <= 1 {
		return result, nil // No data rows
	}

	// Process each row
	for i, row := range rows[1:] {
		rowNum := int32(i + 2) // 1-indexed, skip header

		// Parse row data
		uomCode := getCell(row, 0)
		uomName := getCell(row, 1)
		uomCategory := getCell(row, 2)
		description := getCell(row, 3)

		// Validate code
		code, err := uom.NewCode(uomCode)
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "uom_code",
				Message:   err.Error(),
			})
			continue
		}

		// Validate category
		category, err := uom.NewCategory(uomCategory)
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "uom_category",
				Message:   err.Error(),
			})
			continue
		}

		// Validate name
		if uomName == "" {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "uom_name",
				Message:   "name cannot be empty",
			})
			continue
		}

		// Check for duplicates
		exists, err := h.repo.ExistsByCode(ctx, code)
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "uom_code",
				Message:   fmt.Sprintf("failed to check duplicate: %v", err),
			})
			continue
		}

		if exists {
			switch cmd.DuplicateAction {
			case "skip":
				result.SkippedCount++
				continue
			case "update":
				// Get existing and update
				existing, err := h.repo.GetByCode(ctx, code)
				if err != nil {
					result.FailedCount++
					result.Errors = append(result.Errors, ImportError{
						RowNumber: rowNum,
						Field:     "uom_code",
						Message:   fmt.Sprintf("failed to get existing: %v", err),
					})
					continue
				}

				if err := existing.Update(&uomName, &category, &description, nil, cmd.CreatedBy); err != nil {
					result.FailedCount++
					result.Errors = append(result.Errors, ImportError{
						RowNumber: rowNum,
						Field:     "update",
						Message:   err.Error(),
					})
					continue
				}

				if err := h.repo.Update(ctx, existing); err != nil {
					result.FailedCount++
					result.Errors = append(result.Errors, ImportError{
						RowNumber: rowNum,
						Field:     "update",
						Message:   fmt.Sprintf("failed to update: %v", err),
					})
					continue
				}

				result.UpdatedCount++
				continue
			case "error":
				result.FailedCount++
				result.Errors = append(result.Errors, ImportError{
					RowNumber: rowNum,
					Field:     "uom_code",
					Message:   "duplicate code already exists",
				})
				continue
			}
		}

		// Create new UOM
		entity, err := uom.NewUOM(code, uomName, category, description, cmd.CreatedBy)
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "create",
				Message:   err.Error(),
			})
			continue
		}

		if err := h.repo.Create(ctx, entity); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "create",
				Message:   fmt.Sprintf("failed to create: %v", err),
			})
			continue
		}

		result.SuccessCount++
	}

	return result, nil
}

// getCell safely gets a cell value from a row.
func getCell(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}
