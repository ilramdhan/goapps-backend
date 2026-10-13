package worker

// costsheet_alldata_stream.go is the streaming counterpart of
// WriteAllDataSheet (costsheet_alldata.go, which is code generated and must
// not be hand-edited). It writes the same 149 columns with the same value
// rules, but through excelize's StreamWriter so a large flat-only export never
// materializes the whole sheet in memory.

import (
	"fmt"
	"strconv"

	"github.com/xuri/excelize/v2"
)

// allDataStream writes "all data"-shaped rows to one sheet.
type allDataStream struct {
	sheet string
	sw    *excelize.StreamWriter
	// row is the last Excel row written (1 = header).
	row int
}

// newAllDataStream opens a stream on an existing sheet and writes the frozen
// header row. Panes must be set before any row is written.
func newAllDataStream(book *excelize.File, sheet string) (*allDataStream, error) {
	sw, err := book.NewStreamWriter(sheet)
	if err != nil {
		return nil, fmt.Errorf("open %q stream: %w", sheet, err)
	}
	if err := sw.SetPanes(&excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return nil, fmt.Errorf("freeze %q header: %w", sheet, err)
	}
	header := make([]any, len(allDataColumns))
	for i, col := range allDataColumns {
		header[i] = col.Header
	}
	if err := sw.SetRow("A1", header); err != nil {
		return nil, fmt.Errorf("write %q header: %w", sheet, err)
	}
	return &allDataStream{sheet: sheet, sw: sw, row: 1}, nil
}

// full reports whether the sheet has no room for another row.
func (s *allDataStream) full() bool { return s.row >= excelize.TotalRows }

// writeStage appends one stage's row; no is the printed sequence number.
func (s *allDataStream) writeStage(no int, stage Stage) error {
	s.row++
	cell, err := excelize.CoordinatesToCellName(1, s.row)
	if err != nil {
		return fmt.Errorf("all data coordinate row=%d: %w", s.row, err)
	}
	if err := s.sw.SetRow(cell, allDataRowValues(no, stage)); err != nil {
		return fmt.Errorf("write %q row %d: %w", s.sheet, s.row, err)
	}
	return nil
}

// flush finalizes the sheet.
func (s *allDataStream) flush() error {
	if err := s.sw.Flush(); err != nil {
		return fmt.Errorf("flush %q: %w", s.sheet, err)
	}
	return nil
}

// allDataRowValues mirrors writeAllDataRow's value rules for a stream row:
// absent values stay empty cells (nil), numeric values are real numbers at
// full precision, everything else is text.
func allDataRowValues(no int, stage Stage) []any {
	values := make([]any, len(allDataColumns))
	for i, col := range allDataColumns {
		raw := allDataCellValue(col, no, stage)
		if raw == "" {
			continue
		}
		if number, err := strconv.ParseFloat(raw, 64); err == nil {
			values[i] = number
			continue
		}
		values[i] = raw
	}
	return values
}
