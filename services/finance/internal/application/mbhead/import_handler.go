// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ImportCommand represents the import MB Heads command.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string // "skip", "update", "error"
	CreatedBy       string
}

// ImportResult represents the import MB Heads result. Note: the proto response has no
// UpdatedCount field, so updated duplicate rows are folded into SuccessCount.
type ImportResult struct {
	SuccessCount int32
	SkippedCount int32
	FailedCount  int32
	Errors       []ImportError
}

// ImportError represents a single import error.
type ImportError struct {
	RowNumber int32
	Field     string
	Message   string
}

// ImportHandler handles the ImportMBHeads command.
type ImportHandler struct {
	repo mbhead.Repository
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(repo mbhead.Repository) *ImportHandler {
	return &ImportHandler{repo: repo}
}

// Handle executes the import MB Heads command.
func (h *ImportHandler) Handle(ctx context.Context, cmd ImportCommand) (result *ImportResult, err error) {
	result = &ImportResult{Errors: []ImportError{}}

	rows, err := h.parseExcelFile(cmd.FileContent, cmd.FileName)
	if err != nil {
		return nil, err
	}

	if headerErr := validateMBHeadImportHeader(rows); headerErr != nil {
		return nil, headerErr
	}

	if len(rows) <= 1 {
		return result, nil
	}

	for i, row := range rows[1:] {
		rowNum := safeconv.IntToInt32(i + 2)
		h.processRow(ctx, row, rowNum, cmd, result)
	}

	return result, nil
}

// parseExcelFile opens and validates the Excel file, returning rows.
func (h *ImportHandler) parseExcelFile(content []byte, fileName string) ([][]string, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext != ".xlsx" && ext != ".xls" {
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}

	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to open excel file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close Excel file")
		}
	}()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found in file")
	}

	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("failed to get rows: %w", err)
	}

	return rows, nil
}

// mbHeadRowData holds parsed row values, matching mbHeadTemplateHeaders column order.
type mbHeadRowData struct {
	mbCosting    string
	mgtName      string
	devCode      string
	shadeCode    string
	shadeName    string
	crossSection string
	lustureCode  string
	denier       string
	filament     string
	dozing       string
	isBoughtout  string
}

func parseMBHeadRow(row []string) mbHeadRowData {
	return mbHeadRowData{
		mbCosting:    getCell(row, 0),
		mgtName:      getCell(row, 1),
		devCode:      getCell(row, 2),
		shadeCode:    getCell(row, 3),
		shadeName:    getCell(row, 4),
		crossSection: getCell(row, 5),
		lustureCode:  getCell(row, 6),
		denier:       getCell(row, 7),
		filament:     getCell(row, 8),
		dozing:       getCell(row, 9),
		isBoughtout:  getCell(row, 10),
	}
}

// getCell safely gets a cell value from a row.
func getCell(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}

// processRow handles a single row import.
func (h *ImportHandler) processRow(ctx context.Context, row []string, rowNum int32, cmd ImportCommand, result *ImportResult) {
	data := parseMBHeadRow(row)

	if data.mbCosting == "" {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "mb_costing",
			Message:   "mb costing cannot be empty",
		})
		return
	}

	denier, filament, dozing, isBoughtout, parseErr := parseMBHeadNumericFields(data)
	if parseErr != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "numeric_fields",
			Message:   parseErr.Error(),
		})
		return
	}

	exists, err := h.repo.ExistsByMBCosting(ctx, data.mbCosting)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "mb_costing",
			Message:   fmt.Sprintf("failed to check duplicate: %v", err),
		})
		return
	}

	if exists {
		h.handleDuplicate(ctx, data, denier, filament, dozing, isBoughtout, rowNum, cmd, result)
		return
	}

	h.createMBHead(ctx, data, denier, filament, dozing, isBoughtout, rowNum, cmd.CreatedBy, result)
}

// parseMBHeadNumericFields parses the optional numeric/boolean fields from a row.
func parseMBHeadNumericFields(data mbHeadRowData) (denier *float64, filament *int, dozing *float64, isBoughtout bool, err error) {
	if data.denier != "" {
		v, parseErr := strconv.ParseFloat(data.denier, 64)
		if parseErr != nil {
			return nil, nil, nil, false, fmt.Errorf("invalid denier %q: %w", data.denier, parseErr)
		}
		denier = &v
	}
	if data.filament != "" {
		v, parseErr := strconv.Atoi(data.filament)
		if parseErr != nil {
			return nil, nil, nil, false, fmt.Errorf("invalid filament %q: %w", data.filament, parseErr)
		}
		filament = &v
	}
	if data.dozing != "" {
		v, parseErr := strconv.ParseFloat(data.dozing, 64)
		if parseErr != nil {
			return nil, nil, nil, false, fmt.Errorf("invalid dozing %q: %w", data.dozing, parseErr)
		}
		dozing = &v
	}
	if data.isBoughtout != "" {
		v, parseErr := strconv.ParseBool(data.isBoughtout)
		if parseErr != nil {
			return nil, nil, nil, false, fmt.Errorf("invalid is bought out %q: %w", data.isBoughtout, parseErr)
		}
		isBoughtout = v
	}
	return denier, filament, dozing, isBoughtout, nil
}

// handleDuplicate handles a duplicate mb_costing based on the specified action.
func (h *ImportHandler) handleDuplicate(
	ctx context.Context, data mbHeadRowData, denier *float64, filament *int, dozing *float64,
	isBoughtout bool, rowNum int32, cmd ImportCommand, result *ImportResult,
) {
	switch cmd.DuplicateAction {
	case "skip":
		result.SkippedCount++
	case "update":
		h.updateExisting(ctx, data, denier, filament, dozing, isBoughtout, rowNum, cmd.CreatedBy, result)
	case "error":
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "mb_costing",
			Message:   "duplicate mb costing already exists",
		})
	default:
		result.SkippedCount++
	}
}

// updateExisting updates an existing MB Head.
func (h *ImportHandler) updateExisting(
	ctx context.Context, data mbHeadRowData, denier *float64, filament *int, dozing *float64,
	isBoughtout bool, rowNum int32, updatedBy string, result *ImportResult,
) {
	existing, err := h.repo.GetByMBCosting(ctx, data.mbCosting)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "mb_costing",
			Message:   fmt.Sprintf("failed to get existing: %v", err),
		})
		return
	}

	update := mbhead.UpdateInput{
		MgtName:      strPtrOrNil(data.mgtName),
		DevCode:      &data.devCode,
		ShadeCode:    &data.shadeCode,
		ShadeName:    &data.shadeName,
		CrossSection: &data.crossSection,
		LustureCode:  &data.lustureCode,
		Denier:       denier,
		Filament:     filament,
		Dozing:       dozing,
		IsActive:     &isBoughtout,
	}
	// IsActive above is a placeholder overwrite bug guard: bought-out status is not is_active.
	update.IsActive = nil

	if err := existing.Update(update, updatedBy); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "update",
			Message:   err.Error(),
		})
		return
	}

	if err := h.repo.Update(ctx, existing); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "update",
			Message:   fmt.Sprintf("failed to update: %v", err),
		})
		return
	}

	result.SuccessCount++
}

// createMBHead creates a new MB Head.
func (h *ImportHandler) createMBHead(
	ctx context.Context, data mbHeadRowData, denier *float64, filament *int, dozing *float64,
	isBoughtout bool, rowNum int32, createdBy string, result *ImportResult,
) {
	// ⚠ Minimal mechanical port to NewParams (K-5). No new Excel columns are wired
	// here on purpose: the MB Head template gains VS Number / No of Process / Shade
	// 2-3 only in P4 (plan §407), so the template is not rewritten twice.
	entity, err := mbhead.New(mbhead.NewParams{
		MBCosting:    data.mbCosting,
		MgtName:      strPtrOrNil(data.mgtName),
		Denier:       denier,
		Filament:     filament,
		Dozing:       dozing,
		CreatedBy:    createdBy,
		IsBoughtout:  isBoughtout,
		DevCode:      data.devCode,
		ShadeCode:    data.shadeCode,
		ShadeName:    data.shadeName,
		CrossSection: data.crossSection,
		LustureCode:  data.lustureCode,
	})
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "create",
			Message:   err.Error(),
		})
		return
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "create",
			Message:   fmt.Sprintf("failed to create: %v", err),
		})
		return
	}

	result.SuccessCount++
}

// strPtrOrNil returns nil for an empty string, else a pointer to the string.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ─────────────────────────────────────────────────────────────────────────────
// Import header validation — plan §11 item 108, OPTION 1 (user decision).
//
// parseMBHeadRow reads FIXED indices 0..10 and never consults any header list.
// Before this guard the importer skipped rows[0] blindly, so ANY file whose
// first row happened to be a header was accepted and its columns were read
// positionally — a file with shuffled or foreign columns imported silently
// wrong values. The most common instance: an EXPORTED MB Head file fed straight
// back in, whose leading "No" column made mb_costing receive a row counter.
//
// ⚠ ACCEPTED CONSEQUENCE (explicit user decision, 2026-08-23): this REJECTS old
// files that previously "imported successfully". That is the point — those
// imports were silently wrong. No bypass flag is provided on purpose.
// ─────────────────────────────────────────────────────────────────────────────

// headerErrPrefix prefixes every header rejection. It contains "invalid" on
// purpose: domainErrorToBaseResponse maps "invalid" to HTTP 400 rather than 500.
const headerErrPrefix = "invalid header row: "

// headerWhitespaceRun matches any run of whitespace inside a header cell.
var headerWhitespaceRun = regexp.MustCompile(`\s+`)

// normalizeHeaderCell makes header comparison tolerant of exactly two
// differences and no others: surrounding / repeated whitespace, and letter case.
//
// WHY THIS EXACT STRICTNESS (decision, 2026-08-23): Excel and human editors
// insert stray spaces and re-case text constantly, so rejecting "Denier " or
// "DENIER" would be a false rejection — users would learn to distrust the check
// and ask for it to be turned off. Everything else stays strict: a renamed,
// reordered, missing, or extra column is precisely the silent corruption this
// validation exists to catch, so no fuzzy/partial matching is done.
func normalizeHeaderCell(s string) string {
	return strings.ToLower(headerWhitespaceRun.ReplaceAllString(strings.TrimSpace(s), " "))
}

// trimTrailingEmptyCells drops trailing blank cells from a header row.
// excelize may return styled-but-empty trailing cells; a blank trailing cell
// carries no data, so it is not treated as an "extra column". A trailing cell
// with actual text IS an extra column and is rejected below.
func trimTrailingEmptyCells(row []string) []string {
	end := len(row)
	for end > 0 && strings.TrimSpace(row[end-1]) == "" {
		end--
	}
	return row[:end]
}

// headersMatch reports whether got equals want after normalization.
func headersMatch(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if normalizeHeaderCell(got[i]) != normalizeHeaderCell(want[i]) {
			return false
		}
	}
	return true
}

// looksLikeExportFile reports whether the header row came from the MB Head
// EXPORT (mbHeadExportHeaders) rather than the import template. Detected
// specially because it is by far the most common and most confusing rejection:
// the user just exported and is importing the same file back.
func looksLikeExportFile(got []string) bool {
	if headersMatch(got, mbHeadExportHeaders) {
		return true
	}
	// A partially edited export still betrays itself by the leading "No"
	// counter column, which the import template never has.
	return len(got) > 0 && normalizeHeaderCell(got[0]) == normalizeHeaderCell(mbHeadExportHeaders[0])
}

// describeHeaderMismatch reports, per position, what was found vs what was
// expected. ⚠ A bare "invalid header" is useless to the person hitting this;
// the point of the message is that they can fix the file without support.
func describeHeaderMismatch(got, want []string) string {
	maxLen := max(len(got), len(want))
	diffs := make([]string, 0, maxLen)
	for i := range maxLen {
		switch {
		case i >= len(got):
			diffs = append(diffs, fmt.Sprintf("column %d is missing, expected %q", i+1, want[i]))
		case i >= len(want):
			diffs = append(diffs, fmt.Sprintf("column %d is an unexpected extra column %q", i+1, got[i]))
		case normalizeHeaderCell(got[i]) != normalizeHeaderCell(want[i]):
			diffs = append(diffs, fmt.Sprintf("column %d is %q, expected %q", i+1, got[i], want[i]))
		}
	}
	return fmt.Sprintf("%s. The import template has exactly %d columns, in this order: %s. Download the import template and copy your data into it.",
		strings.Join(diffs, "; "), len(want), strings.Join(want, ", "))
}

// validateMBHeadImportHeader rejects the WHOLE file when rows[0] does not match
// mbHeadTemplateHeaders. No row is processed on failure — a partial import of a
// misaligned file is worse than no import at all.
func validateMBHeadImportHeader(rows [][]string) error {
	if len(rows) == 0 {
		return fmt.Errorf("%sthe file contains no rows at all. Download the MB Head import template and fill it in", headerErrPrefix)
	}

	got := trimTrailingEmptyCells(rows[0])
	if headersMatch(got, mbHeadTemplateHeaders) {
		return nil
	}

	if looksLikeExportFile(got) {
		return fmt.Errorf("%sthis is an EXPORTED MB Head file, not the import template. "+
			"The export adds a leading %q column and trailing %q / %q / %q columns, so every value would land in the wrong field "+
			"(%q would receive the row number). Download the import template first and copy your data into it. "+
			"The template has exactly %d columns, in this order: %s",
			headerErrPrefix,
			mbHeadExportHeaders[0], "Active", "Created At", "Created By",
			mbHeadTemplateHeaders[0],
			len(mbHeadTemplateHeaders), strings.Join(mbHeadTemplateHeaders, ", "))
	}

	return fmt.Errorf("%s%s", headerErrPrefix, describeHeaderMismatch(got, mbHeadTemplateHeaders))
}
