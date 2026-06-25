package costbulkimport

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

// SheetError records a single row-level parse or validation error.
type SheetError struct {
	RowNumber int32
	Field     string
	Message   string
}

// ParseSheet reads sheet(s) from the Excel file that match baseName and returns
// all data rows merged in order.
//
// Matching rules (in priority order):
//  1. Exact match — sheet named exactly baseName is used alone.
//  2. Contains match — all sheets whose name contains baseName as a
//     case-insensitive substring are merged alphabetically.
//     This transparently handles:
//     - Number-prefixed sheets  ("1_product_master" → baseName "product_master")
//     - Split part sheets       ("product_parameters_p1", "_p2" → merged)
//
// Required headers are validated against the first matching sheet only.
// Subsequent sheets must have compatible columns (their header row is skipped).
func ParseSheet(f *excelize.File, baseName string, requiredHeaders []string) ([]map[string]string, error) {
	matched := findMatchingSheets(f, baseName)
	if len(matched) == 0 {
		return nil, fmt.Errorf("no sheet found matching %q — available: %v", baseName, f.GetSheetList())
	}
	return mergeSheetRows(f, baseName, matched, requiredHeaders)
}

// ParseSheetOptional is like ParseSheet but returns nil, nil when no matching sheet
// exists instead of an error. Use for sheets that are optional in a bulk import file
// (e.g. product_parameters when the user only wants to import product master + routing).
func ParseSheetOptional(f *excelize.File, baseName string, requiredHeaders []string) ([]map[string]string, error) {
	matched := findMatchingSheets(f, baseName)
	if len(matched) == 0 {
		return nil, nil // sheet absent — caller should treat as zero rows
	}
	return mergeSheetRows(f, baseName, matched, requiredHeaders)
}

// findMatchingSheets returns sheets from f whose name matches baseName.
// Exact match wins exclusively; otherwise all contains-matches sorted alpha.
func findMatchingSheets(f *excelize.File, baseName string) []string {
	all := f.GetSheetList()
	lower := strings.ToLower(baseName)

	for _, s := range all {
		if s == baseName {
			return []string{s}
		}
	}

	var found []string
	for _, s := range all {
		if strings.Contains(strings.ToLower(s), lower) {
			found = append(found, s)
		}
	}
	sort.Strings(found)
	return found
}

// mergeSheetRows reads and concatenates data rows from all given sheets.
// Headers are taken from the first sheet; subsequent header rows are skipped.
func mergeSheetRows(f *excelize.File, baseName string, sheets []string, requiredHeaders []string) ([]map[string]string, error) {
	var headers []string
	var result []map[string]string

	for i, sheet := range sheets {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return nil, fmt.Errorf("sheet %q not found or unreadable: %w", sheet, err)
		}
		if len(rows) == 0 {
			continue
		}

		sheetHeaders := make([]string, len(rows[0]))
		for j, h := range rows[0] {
			sheetHeaders[j] = strings.TrimSpace(h)
		}

		if i == 0 {
			headers = sheetHeaders
			for _, req := range requiredHeaders {
				if !slices.Contains(headers, req) {
					return nil, fmt.Errorf("sheet matching %q (found %q) missing required header %q", baseName, sheet, req)
				}
			}
		}

		for _, row := range rows[1:] {
			allEmpty := true
			rowMap := make(map[string]string, len(headers))
			for j, h := range headers {
				if j < len(row) {
					val := strings.TrimSpace(row[j])
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
	}
	return result, nil
}
