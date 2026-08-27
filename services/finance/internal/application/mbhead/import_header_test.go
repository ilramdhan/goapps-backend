// Package mbhead — header-row validation on MB Head import (plan §11 item 108,
// OPTION 1: reject files whose header row does not match mbHeadTemplateHeaders).
package mbhead

import (
	"context"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// buildHeaderTestXLSX creates an in-memory xlsx from the given rows.
func buildHeaderTestXLSX(t *testing.T, rows [][]string) []byte {
	t.Helper()

	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			t.Logf("close xlsx: %v", err)
		}
	}()

	sheet := f.GetSheetName(f.GetActiveSheetIndex())
	for r, row := range rows {
		for c, val := range row {
			cell, err := excelize.CoordinatesToCellName(c+1, r+1)
			if err != nil {
				t.Fatalf("cell coordinates (%d,%d): %v", c+1, r+1, err)
			}
			if err := f.SetCellValue(sheet, cell, val); err != nil {
				t.Fatalf("set cell %s: %v", cell, err)
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	return buf.Bytes()
}

func copyHeaders(src []string) []string {
	out := make([]string, len(src))
	copy(out, src)
	return out
}

func TestValidateMBHeadImportHeader_TemplateHeaderPasses(t *testing.T) {
	t.Parallel()

	if err := validateMBHeadImportHeader([][]string{copyHeaders(mbHeadTemplateHeaders)}); err != nil {
		t.Fatalf("template header must be accepted, got: %v", err)
	}
}

func TestValidateMBHeadImportHeader_ToleratesWhitespaceAndCase(t *testing.T) {
	t.Parallel()

	sloppy := copyHeaders(mbHeadTemplateHeaders)
	sloppy[7] = "  denier "
	sloppy[0] = "MB   COSTING"

	if err := validateMBHeadImportHeader([][]string{sloppy}); err != nil {
		t.Fatalf("whitespace/case differences must be tolerated, got: %v", err)
	}
}

func TestValidateMBHeadImportHeader_ExportFileIsRejectedWithExportSpecificMessage(t *testing.T) {
	t.Parallel()

	err := validateMBHeadImportHeader([][]string{copyHeaders(mbHeadExportHeaders)})
	if err == nil {
		t.Fatal("an EXPORTED file must be rejected")
	}
	msg := err.Error()
	for _, want := range []string{"EXPORT", "import template", `"No"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("export rejection message must mention %q; got: %s", want, msg)
		}
	}
}

func TestValidateMBHeadImportHeader_SwappedColumnIsRejectedWithPosition(t *testing.T) {
	t.Parallel()

	swapped := copyHeaders(mbHeadTemplateHeaders)
	swapped[3], swapped[4] = swapped[4], swapped[3]

	err := validateMBHeadImportHeader([][]string{swapped})
	if err == nil {
		t.Fatal("a swapped column must be rejected")
	}
	msg := err.Error()
	for _, want := range []string{"column 4", "column 5", "Shade Code", "Shade Name"} {
		if !strings.Contains(msg, want) {
			t.Errorf("mismatch message must mention %q; got: %s", want, msg)
		}
	}
}

func TestValidateMBHeadImportHeader_ExtraTextColumnIsRejected(t *testing.T) {
	t.Parallel()

	extra := append(copyHeaders(mbHeadTemplateHeaders), "Notes")
	err := validateMBHeadImportHeader([][]string{extra})
	if err == nil {
		t.Fatal("an extra trailing column must be rejected")
	}
	if !strings.Contains(err.Error(), "extra column") {
		t.Errorf("message must call out the extra column; got: %s", err)
	}
}

func TestValidateMBHeadImportHeader_TrailingBlankCellsAreIgnored(t *testing.T) {
	t.Parallel()

	padded := append(copyHeaders(mbHeadTemplateHeaders), "", "   ")
	if err := validateMBHeadImportHeader([][]string{padded}); err != nil {
		t.Fatalf("trailing blank cells carry no data and must be ignored, got: %v", err)
	}
}

func TestValidateMBHeadImportHeader_EmptyFileDoesNotPanic(t *testing.T) {
	t.Parallel()

	err := validateMBHeadImportHeader([][]string{})
	if err == nil {
		t.Fatal("a file with no rows must be rejected, not silently accepted")
	}
	if !strings.Contains(err.Error(), "no rows") {
		t.Errorf("message must explain the file is empty; got: %s", err)
	}
}

func TestImportHandler_RejectsWholeFileOnBadHeader(t *testing.T) {
	t.Parallel()

	content := buildHeaderTestXLSX(t, [][]string{
		copyHeaders(mbHeadExportHeaders),
		{"1", "MBC-0001", "Black", "DEV-001", "SH-BLK", "Black", "ROUND", "LC-01", "150", "48", "1.2", "FALSE", "TRUE", "", ""},
	})

	// A nil repo proves no row was processed: touching the repo would panic.
	h := NewImportHandler(nil)
	result, err := h.Handle(context.Background(), ImportCommand{
		FileContent: content,
		FileName:    "mb_head_export.xlsx",
		CreatedBy:   "tester",
	})
	if err == nil {
		t.Fatal("importing an exported file must fail the whole file")
	}
	if result != nil {
		t.Errorf("no partial result may be returned on header rejection, got: %+v", result)
	}
	if !strings.Contains(err.Error(), "EXPORT") {
		t.Errorf("error must identify the export-vs-template mistake; got: %s", err)
	}
}

func TestImportHandler_AcceptsTemplateHeaderAndProcessesRows(t *testing.T) {
	t.Parallel()

	content := buildHeaderTestXLSX(t, [][]string{
		copyHeaders(mbHeadTemplateHeaders),
		{"", "Black MB Batch", "", "", "", "", "", "", "", "", ""},
	})

	// Blank mb_costing fails before any repo call, so a nil repo is safe here
	// while still proving the header gate let the data row through. The row
	// carries a non-blank cell because excelize drops fully-empty trailing rows.
	h := NewImportHandler(nil)
	result, err := h.Handle(context.Background(), ImportCommand{
		FileContent: content,
		FileName:    "mb_head_import_template.xlsx",
		CreatedBy:   "tester",
	})
	if err != nil {
		t.Fatalf("a correct template header must be accepted, got: %v", err)
	}
	if result.FailedCount != 1 {
		t.Fatalf("the data row must have been processed, got result: %+v", result)
	}
}

func TestImportHandler_EmptySheetDoesNotPanic(t *testing.T) {
	t.Parallel()

	content := buildHeaderTestXLSX(t, [][]string{})

	h := NewImportHandler(nil)
	if _, err := h.Handle(context.Background(), ImportCommand{
		FileContent: content,
		FileName:    "empty.xlsx",
		CreatedBy:   "tester",
	}); err == nil {
		t.Fatal("an empty sheet must be rejected, not silently accepted")
	}
}
