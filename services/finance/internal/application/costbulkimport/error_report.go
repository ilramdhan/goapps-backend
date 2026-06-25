package costbulkimport

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

// unknownParamPrefix is the message prefix written by processCPP, processCAP,
// and preflightParamSheet when a param_code is not found in the parameter master.
// Used to de-duplicate these errors into the dedicated "missing_param_codes" sheet.
const unknownParamPrefix = "unknown param code: "

// missProductPrefix is the sentinel prefix used by crossCheckProductMap to encode
// a missing product ID along with its affected-row count in the format:
//
//	"miss_product:<legacy_id>:<row_count>"
//
// These synthetic errors are extracted into a "missing_product_ids" sheet and
// suppressed from the per-sheet error tabs.
const missProductPrefix = "miss_product:"

// SheetResult summarizes processing outcomes for one Excel sheet.
type SheetResult struct {
	SheetName string
	TotalRows int
	Inserted  int
	Updated   int
	Skipped   int
	Errors    []SheetError
}

// GenerateErrorReport builds a multi-sheet Excel error report in memory.
//
// Sheets produced:
//   - "summary"             — per-sheet totals (all error types counted)
//   - "<sheet>_errors"      — row-level errors, excluding unknown-param-code rows
//   - "missing_param_codes" — unique unknown param codes with skipped-row counts
//
// Unknown param code errors are separated into the dedicated sheet so the
// operator sees a clean, de-duplicated list rather than potentially millions
// of individual row entries for the same handful of missing codes.
func GenerateErrorReport(results []SheetResult) ([]byte, error) {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			_ = err
		}
	}()

	const summarySheet = "summary"
	if setErr := f.SetSheetName("Sheet1", summarySheet); setErr != nil {
		return nil, fmt.Errorf("set summary sheet: %w", setErr)
	}
	if err := writeSummarySheet(f, summarySheet, results); err != nil {
		return nil, err
	}

	// Per-sheet error tabs — unknown-param-code and missing-product rows go to dedicated sheets.
	for _, r := range results {
		filtered := filterOutSummaryErrors(r.Errors)
		if len(filtered) == 0 {
			continue
		}
		copy := r
		copy.Errors = filtered
		if err := writeErrorSheet(f, copy); err != nil {
			return nil, err
		}
	}

	// Dedicated sheet for unique unknown param codes.
	if codes := collectUnknownParamCodes(results); len(codes) > 0 {
		if err := writeMissingParamCodesSheet(f, codes); err != nil {
			return nil, err
		}
	}

	// Dedicated sheet for product IDs not found in the database (params-only import).
	if products := collectMissingProductIDs(results); len(products) > 0 {
		if err := writeMissingProductIDsSheet(f, products); err != nil {
			return nil, err
		}
	}

	var buf bytes.Buffer
	if writeErr := f.Write(&buf); writeErr != nil {
		return nil, fmt.Errorf("write error report: %w", writeErr)
	}
	return buf.Bytes(), nil
}

func writeSummarySheet(f *excelize.File, sheetName string, results []SheetResult) error {
	headers := []string{"sheet_name", "total_rows", "inserted", "updated", "skipped", "errors"}
	for col, h := range headers {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, 1)
		if cellErr != nil {
			return fmt.Errorf("coordinates to cell name: %w", cellErr)
		}
		if err := f.SetCellValue(sheetName, cell, h); err != nil {
			return fmt.Errorf("set cell %s: %w", cell, err)
		}
	}
	for row, r := range results {
		rowIdx := row + 2
		vals := []any{r.SheetName, r.TotalRows, r.Inserted, r.Updated, r.Skipped, len(r.Errors)}
		for col, v := range vals {
			cell, cellErr := excelize.CoordinatesToCellName(col+1, rowIdx)
			if cellErr != nil {
				return fmt.Errorf("coordinates to cell name: %w", cellErr)
			}
			if err := f.SetCellValue(sheetName, cell, v); err != nil {
				return fmt.Errorf("set cell %s: %w", cell, err)
			}
		}
	}
	return nil
}

func writeErrorSheet(f *excelize.File, r SheetResult) error {
	errSheetName := r.SheetName + "_errors"
	if _, createErr := f.NewSheet(errSheetName); createErr != nil {
		return fmt.Errorf("create sheet %q: %w", errSheetName, createErr)
	}
	errHeaders := []string{"row_number", "field", "message"}
	for col, h := range errHeaders {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, 1)
		if cellErr != nil {
			return fmt.Errorf("coordinates to cell name: %w", cellErr)
		}
		if err := f.SetCellValue(errSheetName, cell, h); err != nil {
			return fmt.Errorf("set cell %s: %w", cell, err)
		}
	}
	for rowIdx, e := range r.Errors {
		vals := []any{e.RowNumber, e.Field, e.Message}
		for col, v := range vals {
			cell, cellErr := excelize.CoordinatesToCellName(col+1, rowIdx+2)
			if cellErr != nil {
				return fmt.Errorf("coordinates to cell name: %w", cellErr)
			}
			if err := f.SetCellValue(errSheetName, cell, v); err != nil {
				return fmt.Errorf("set cell %s: %w", cell, err)
			}
		}
	}
	return nil
}

// writeMissingParamCodesSheet writes a dedicated summary sheet listing every
// unique unknown param code and how many rows were skipped because of it.
func writeMissingParamCodesSheet(f *excelize.File, codes map[string]int) error {
	const sheetName = "missing_param_codes"
	if _, createErr := f.NewSheet(sheetName); createErr != nil {
		return fmt.Errorf("create sheet %q: %w", sheetName, createErr)
	}
	headers := []string{"param_code", "skipped_rows", "action_required"}
	for col, h := range headers {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, 1)
		if cellErr != nil {
			return fmt.Errorf("coordinates to cell name: %w", cellErr)
		}
		if err := f.SetCellValue(sheetName, cell, h); err != nil {
			return fmt.Errorf("set header %s: %w", cell, err)
		}
	}
	sorted := make([]string, 0, len(codes))
	for c := range codes {
		sorted = append(sorted, c)
	}
	sort.Strings(sorted)
	for rowIdx, code := range sorted {
		rowNum := rowIdx + 2
		vals := []any{code, codes[code], "Create or map in Finance > Master > Parameter, then re-import"}
		for col, v := range vals {
			cell, cellErr := excelize.CoordinatesToCellName(col+1, rowNum)
			if cellErr != nil {
				return fmt.Errorf("coordinates to cell name: %w", cellErr)
			}
			if err := f.SetCellValue(sheetName, cell, v); err != nil {
				return fmt.Errorf("set cell %s: %w", cell, err)
			}
		}
	}
	return nil
}

func collectUnknownParamCodes(results []SheetResult) map[string]int {
	codes := make(map[string]int)
	for _, r := range results {
		for _, e := range r.Errors {
			if code, ok := strings.CutPrefix(e.Message, unknownParamPrefix); ok {
				codes[code]++
			}
		}
	}
	return codes
}

// filterOutSummaryErrors removes errors that are surfaced in dedicated summary sheets
// (missing_param_codes and missing_product_ids) so the per-sheet error tabs stay lean.
func filterOutSummaryErrors(errs []SheetError) []SheetError {
	result := make([]SheetError, 0, len(errs))
	for _, e := range errs {
		if strings.HasPrefix(e.Message, unknownParamPrefix) {
			continue
		}
		if strings.HasPrefix(e.Message, missProductPrefix) {
			continue
		}
		result = append(result, e)
	}
	return result
}

// collectMissingProductIDs scans all sheet errors for miss_product: sentinel errors
// and returns a map of legacyID → affected_row_count.
// Sentinel format: "miss_product:<id>:<count>"
func collectMissingProductIDs(results []SheetResult) map[string]int {
	products := make(map[string]int)
	for _, r := range results {
		for _, e := range r.Errors {
			payload, ok := strings.CutPrefix(e.Message, missProductPrefix)
			if !ok {
				continue
			}
			sepIdx := strings.LastIndex(payload, ":")
			if sepIdx < 0 {
				products[payload]++
				continue
			}
			id := payload[:sepIdx]
			var cnt int
			if _, scanErr := fmt.Sscanf(payload[sepIdx+1:], "%d", &cnt); scanErr != nil || cnt < 1 {
				cnt = 1
			}
			products[id] += cnt
		}
	}
	return products
}

// writeMissingProductIDsSheet writes a dedicated sheet listing every product
// Oracle ID that was not found in the database, with row-skip counts.
func writeMissingProductIDsSheet(f *excelize.File, products map[string]int) error {
	const sheetName = "missing_product_ids"
	if _, createErr := f.NewSheet(sheetName); createErr != nil {
		return fmt.Errorf("create sheet %q: %w", sheetName, createErr)
	}
	headers := []string{"legacy_oracle_sys_id", "skipped_rows", "action_required"}
	for col, h := range headers {
		cell, cellErr := excelize.CoordinatesToCellName(col+1, 1)
		if cellErr != nil {
			return fmt.Errorf("coordinates to cell name: %w", cellErr)
		}
		if err := f.SetCellValue(sheetName, cell, h); err != nil {
			return fmt.Errorf("set header %s: %w", cell, err)
		}
	}

	sorted := make([]string, 0, len(products))
	for id := range products {
		sorted = append(sorted, id)
	}
	sort.Strings(sorted)

	for rowIdx, id := range sorted {
		rowNum := rowIdx + 2
		vals := []any{id, products[id], "Import this product in product_master first, then re-import params"}
		for col, v := range vals {
			cell, cellErr := excelize.CoordinatesToCellName(col+1, rowNum)
			if cellErr != nil {
				return fmt.Errorf("coordinates to cell name: %w", cellErr)
			}
			if err := f.SetCellValue(sheetName, cell, v); err != nil {
				return fmt.Errorf("set cell %s: %w", cell, err)
			}
		}
	}
	return nil
}
