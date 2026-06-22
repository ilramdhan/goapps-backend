package costbulkimport

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// SheetResult summarizes processing outcomes for one Excel sheet.
type SheetResult struct {
	SheetName string
	TotalRows int
	Inserted  int
	Updated   int
	Skipped   int
	Errors    []SheetError
}

// GenerateErrorReport builds a multi-sheet Excel error report in memory and returns the bytes.
// Sheets: summary (per-sheet totals) + one error sheet per input sheet (with row errors).
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
	for _, r := range results {
		if len(r.Errors) == 0 {
			continue
		}
		if err := writeErrorSheet(f, r); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	if writeErr := f.Write(&buf); writeErr != nil {
		return nil, fmt.Errorf("write error report: %w", writeErr)
	}
	return buf.Bytes(), nil
}

// writeSummarySheet populates the summary sheet with per-sheet totals.
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

// writeErrorSheet creates and populates a per-sheet error tab.
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
