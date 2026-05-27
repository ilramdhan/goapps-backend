// Package excel provides reusable Excel export, import, and template utilities
// for all backend services. It wraps xuri/excelize/v2 with a column-definition
// DSL that eliminates per-entity boilerplate.
package excel

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"
)

// safeRowNumber converts an Excel row index (int) to int32 with bounds clamping.
// Excel's maximum row count is ~1M, well under int32 max, so in practice no clamping
// is needed — but gosec G115 requires the explicit guard.
func safeRowNumber(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < 0 {
		return 0
	}
	return int32(v) //nolint:gosec // bounds checked above
}

// Column defines a single column in an export/template sheet.
type Column struct {
	Header string  // Header text shown in row 1.
	Width  float64 // Column width in characters. 0 uses excelize default.
}

// ExportRow is one row of cell values, ordered to match the Column slice.
type ExportRow []interface{}

// headerStyle is the shared blue-header style for all sheets.
var headerStyleDef = &excelize.Style{
	Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
	Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
	Alignment: &excelize.Alignment{Horizontal: "center"},
}

// Export writes rows to an Excel file and returns the bytes.
func Export(sheetName string, columns []Column, rows []ExportRow) ([]byte, error) {
	f := excelize.NewFile()
	defer closeFile(f)

	if err := createSheet(f, sheetName, columns); err != nil {
		return nil, err
	}

	w := &writer{f: f, sheet: sheetName}
	for i, row := range rows {
		excelRow := i + 2
		for col, val := range row {
			cell, cErr := excelize.CoordinatesToCellName(col+1, excelRow)
			if cErr != nil {
				return nil, fmt.Errorf("cell coord (%d,%d): %w", col+1, excelRow, cErr)
			}
			w.set(cell, val)
		}
	}
	if w.hasErrors() {
		log.Warn().Err(w.error()).Msg("non-critical Excel write errors during export")
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	return buf.Bytes(), nil
}

// SampleRow is a slice of string values used in import templates.
type SampleRow = []string

// Instruction is a single line on the Instructions sheet.
type Instruction struct {
	Cell string // e.g. "A3".
	Text string
}

// Template generates an import template with headers, sample data, and an instructions sheet.
func Template(sheetName string, columns []Column, samples []SampleRow, instructions []Instruction) ([]byte, error) {
	f := excelize.NewFile()
	defer closeFile(f)

	if err := createSheet(f, sheetName, columns); err != nil {
		return nil, err
	}

	w := &writer{f: f, sheet: sheetName}
	for i, sample := range samples {
		row := i + 2
		for col, val := range sample {
			cell, cErr := excelize.CoordinatesToCellName(col+1, row)
			if cErr != nil {
				return nil, fmt.Errorf("cell coord: %w", cErr)
			}
			w.set(cell, val)
		}
	}

	if len(instructions) > 0 {
		instrSheet := "Instructions"
		if _, err := f.NewSheet(instrSheet); err != nil {
			log.Debug().Err(err).Msg("could not create Instructions sheet")
		} else {
			iw := &writer{f: f, sheet: instrSheet}
			for _, inst := range instructions {
				iw.set(inst.Cell, inst.Text)
			}
			if iw.hasErrors() {
				log.Warn().Err(iw.error()).Msg("non-critical errors writing instructions sheet")
			}
		}
	}

	if w.hasErrors() {
		log.Warn().Err(w.error()).Msg("non-critical Excel write errors during template")
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	return buf.Bytes(), nil
}

// ParsedRow is a row from an imported file — map of column index to trimmed string value.
type ParsedRow struct {
	RowNumber int32
	Cells     []string
}

// ImportError records one validation failure during import.
type ImportError struct {
	RowNumber int32
	Field     string
	Message   string
}

// ParseFile opens an .xlsx file and returns all data rows (skipping the header).
func ParseFile(content []byte, fileName string) ([]ParsedRow, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext != ".xlsx" && ext != ".xls" {
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}

	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("open excel: %w", err)
	}
	defer closeFile(f)

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found")
	}

	rawRows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("get rows: %w", err)
	}

	if len(rawRows) <= 1 {
		return nil, nil
	}

	parsed := make([]ParsedRow, 0, len(rawRows)-1)
	for i, row := range rawRows[1:] {
		trimmed := make([]string, len(row))
		for j, cell := range row {
			trimmed[j] = strings.TrimSpace(cell)
		}
		parsed = append(parsed, ParsedRow{RowNumber: safeRowNumber(i + 2), Cells: trimmed})
	}
	return parsed, nil
}

// Cell safely gets a cell value from the parsed row.
func (r ParsedRow) Cell(index int) string {
	if index < len(r.Cells) {
		return r.Cells[index]
	}
	return ""
}

// createSheet sets up a named sheet with styled headers and column widths.
func createSheet(f *excelize.File, name string, columns []Column) error {
	idx, err := f.NewSheet(name)
	if err != nil {
		return fmt.Errorf("create sheet: %w", err)
	}
	f.SetActiveSheet(idx)
	if delErr := f.DeleteSheet("Sheet1"); delErr != nil {
		log.Debug().Err(delErr).Msg("could not delete default Sheet1")
	}

	for i, col := range columns {
		cell, cErr := excelize.CoordinatesToCellName(i+1, 1)
		if cErr != nil {
			return fmt.Errorf("header coord: %w", cErr)
		}
		if err := f.SetCellValue(name, cell, col.Header); err != nil {
			return fmt.Errorf("set header %q: %w", col.Header, err)
		}
		if col.Width > 0 {
			colLetter, _ := excelize.ColumnNumberToName(i + 1)
			if wErr := f.SetColWidth(name, colLetter, colLetter, col.Width); wErr != nil {
				log.Debug().Err(wErr).Str("col", colLetter).Msg("set col width")
			}
		}
	}

	style, err := f.NewStyle(headerStyleDef)
	if err != nil {
		return fmt.Errorf("create header style: %w", err)
	}
	lastCol, _ := excelize.ColumnNumberToName(len(columns))
	if err := f.SetCellStyle(name, "A1", lastCol+"1", style); err != nil {
		return fmt.Errorf("set header style: %w", err)
	}
	return nil
}

// writer collects non-critical cell-write errors.
type writer struct {
	f     *excelize.File
	sheet string
	errs  []error
}

func (w *writer) set(cell string, val interface{}) {
	if err := w.f.SetCellValue(w.sheet, cell, val); err != nil {
		w.errs = append(w.errs, fmt.Errorf("cell %s: %w", cell, err))
	}
}

func (w *writer) hasErrors() bool { return len(w.errs) > 0 }

func (w *writer) error() error { return errors.Join(w.errs...) }

func closeFile(f *excelize.File) {
	if err := f.Close(); err != nil {
		log.Warn().Err(err).Msg("failed to close Excel file")
	}
}
