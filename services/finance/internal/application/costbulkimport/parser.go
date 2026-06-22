package costbulkimport

import (
	"fmt"
	"slices"
	"strings"

	"github.com/xuri/excelize/v2"
)

// SheetError records a single row-level parse or validation error.
type SheetError struct {
	RowNumber int32
	Field     string
	Message   string
}

// ParseSheet reads a named sheet from the Excel file and returns parsed rows.
// Each row is a map of header→value. Rows where ALL cells are empty are skipped.
// Returns an error if the sheet is missing or required headers are absent.
func ParseSheet(f *excelize.File, sheetName string, requiredHeaders []string) ([]map[string]string, error) {
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("sheet %q not found or unreadable: %w", sheetName, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("sheet %q is empty", sheetName)
	}

	headers := make([]string, len(rows[0]))
	for i, h := range rows[0] {
		headers[i] = strings.TrimSpace(h)
	}

	for _, required := range requiredHeaders {
		if !slices.Contains(headers, required) {
			return nil, fmt.Errorf("sheet %q missing required header %q", sheetName, required)
		}
	}

	result := make([]map[string]string, 0, len(rows)-1)
	for _, row := range rows[1:] {
		allEmpty := true
		rowMap := make(map[string]string, len(headers))
		for i, h := range headers {
			if i < len(row) {
				val := strings.TrimSpace(row[i])
				rowMap[h] = val
				if val != "" {
					allEmpty = false
				}
			} else {
				rowMap[h] = ""
			}
		}
		if !allEmpty {
			result = append(result, rowMap)
		}
	}
	return result, nil
}
