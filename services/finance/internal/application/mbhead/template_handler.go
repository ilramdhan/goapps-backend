// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"
)

// TemplateResult represents the download template result.
type TemplateResult struct {
	FileContent []byte
	FileName    string
}

// TemplateHandler handles the DownloadMBHeadTemplate query.
type TemplateHandler struct{}

// NewTemplateHandler creates a new TemplateHandler.
func NewTemplateHandler() *TemplateHandler {
	return &TemplateHandler{}
}

// writeMBHeadTemplateHeaders writes and styles the header row, returning the last header column.
func writeMBHeadTemplateHeaders(f *excelize.File, sheetName string, headers []string) (string, error) {
	lastCol, err := excelize.CoordinatesToCellName(len(headers), 1)
	if err != nil {
		return "", fmt.Errorf("failed to get last header cell name: %w", err)
	}

	for col, header := range headers {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			return "", fmt.Errorf("failed to get cell name: %w", err)
		}
		if err := f.SetCellValue(sheetName, cell, header); err != nil {
			return "", fmt.Errorf("failed to set header %s: %w", header, err)
		}
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create header style: %w", err)
	}
	if err := f.SetCellStyle(sheetName, "A1", lastCol+"1", headerStyle); err != nil {
		return "", fmt.Errorf("failed to set header style: %w", err)
	}

	return lastCol, nil
}

// writeMBHeadTemplateSampleRows writes the two illustrative rows carried by the shared column
// table, so the samples cannot fall out of step with the headers above them.
func writeMBHeadTemplateSampleRows(writer *excelWriter, cols []mbHeadImportColumn) {
	for sample := range len(cols[0].samples) {
		rowNum := sample + 2
		for i, col := range cols {
			cell, cellErr := excelize.CoordinatesToCellName(i+1, rowNum)
			if cellErr != nil {
				writer.errs = append(writer.errs, fmt.Errorf("row %d col %d: %w", rowNum, i+1, cellErr))
				continue
			}
			writer.setCellValue(cell, col.samples[sample])
		}
	}
}

// Handle builds the MB Head import template Excel file.
func (h *TemplateHandler) Handle() (result *TemplateResult, err error) {
	f := excelize.NewFile()
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close Excel file")
			if err == nil {
				err = fmt.Errorf("failed to close file: %w", closeErr)
			}
		}
	}()

	sheetName := "MB Head Import Template"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	if deleteErr := f.DeleteSheet("Sheet1"); deleteErr != nil {
		log.Debug().Err(deleteErr).Msg("Could not delete default Sheet1")
	}

	// The template carries exactly the import's canonical columns — no audit columns, because
	// they are read-only provenance the import ignores.
	cols := mbHeadImportColumns
	if _, err := writeMBHeadTemplateHeaders(f, sheetName, mbHeadColumnHeaders(cols)); err != nil {
		return nil, err
	}

	writer := &excelWriter{f: f, sheetName: sheetName}
	writeMBHeadTemplateSampleRows(writer, cols)
	setMBHeadColumnWidths(writer, cols)

	if writer.hasErrors() {
		log.Warn().Errs("errors", writer.errs).Msg("Some Excel formatting operations failed")
	}

	if instrErr := addMBHeadInstructionsSheet(f); instrErr != nil {
		log.Debug().Err(instrErr).Msg("Could not add instructions sheet")
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return &TemplateResult{
		FileContent: buffer.Bytes(),
		FileName:    "mb_head_import_template.xlsx",
	}, nil
}

// addMBHeadInstructionsSheet adds a non-critical instructions sheet to the template. The
// required-column list is generated from the shared column table so it cannot go stale.
func addMBHeadInstructionsSheet(f *excelize.File) error {
	sheetName := "Instructions"
	if _, err := f.NewSheet(sheetName); err != nil {
		return fmt.Errorf("failed to create instructions sheet: %w", err)
	}

	var required []string
	for _, col := range mbHeadImportColumns {
		if col.required {
			required = append(required, col.header)
		}
	}

	instructions := []string{
		"MB Head Import Instructions",
		"",
		"1. Columns are matched by header NAME, not by position — you may reorder them freely.",
		"   Extra columns are ignored, so an exported file can be edited and imported back as-is.",
		"",
		"2. These columns are REQUIRED on every row and must not be left blank:",
		"   " + strings.Join(required, ", "),
		"",
		"3. MB Costing, Development No and VS Number must each be unique across all MB recipes,",
		"   and unique within the file you are importing.",
		"",
		"4. No of Process must be one of the active option codes configured in the MB parameter",
		"   master (NO_OF_PROCESS). Ask an administrator for the current list — it is maintained",
		"   in the master data, not fixed in the file format.",
		"",
		"5. POY Denier and POY Filament must be greater than 0. LDR % must be between 0 and 100.",
		"",
		"6. Dozing is numeric and optional. Is Bought Out must be TRUE or FALSE (blank means FALSE).",
		"",
		"7. Shade Code 2/3 and Shade Name 2/3 are optional additional shades. Supply both the code",
		"   and the name for a shade, and keep each code distinct from the others and from the main",
		"   Shade Code. Leaving them blank clears any existing additional shades on update.",
		"",
		"8. Duplicate MB Costing values are handled per the duplicate action selected on import",
		"   (skip, update, or error).",
		"",
		"9. Tick 'dry run' on import to validate the whole file and see the errors without writing",
		"   anything to the database.",
	}

	for i, line := range instructions {
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			return fmt.Errorf("failed to get instructions cell name: %w", err)
		}
		if err := f.SetCellValue(sheetName, cell, line); err != nil {
			return fmt.Errorf("failed to set instructions cell: %w", err)
		}
	}

	if err := f.SetColWidth(sheetName, "A", "A", 90); err != nil {
		return fmt.Errorf("failed to set instructions column width: %w", err)
	}

	return nil
}
