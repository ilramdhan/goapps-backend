// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// ImportCommand represents the import MB Heads command.
type ImportCommand struct {
	FileContent     []byte
	FileName        string
	DuplicateAction string // "skip", "update", "error"
	CreatedBy       string
	// DryRun, when true, runs the full validation pipeline (required fields, in-file and DB
	// uniqueness, no-of-process membership, shade rules) exactly as a real import would, but
	// performs no repository writes (Create/Update/ReplaceShades). Counts and Errors are
	// populated as if the import had run for real.
	DryRun bool
}

// maxImportErrors caps the number of per-row errors returned to the caller; FailedCount and
// ErrorSummary stay accurate for every row even once the individual list is capped (mirrors
// internal/application/bi/upload/parse_handler.go's capErrors).
const maxImportErrors = 200

// ImportResult represents the import MB Heads result. SuccessCount counts newly created rows
// only; UpdatedCount counts existing rows updated via duplicate_action=update, so the two no
// longer collapse into one number (fixes I5).
type ImportResult struct {
	SuccessCount int32
	UpdatedCount int32
	SkippedCount int32
	FailedCount  int32
	Errors       []ImportError
	// ErrorSummary groups error messages by field, counting occurrences beyond the individual
	// Errors slice's cap so the caller can still see the shape of a large failure.
	ErrorSummary []ImportErrorSummary
}

// ImportErrorSummary groups repeated per-field error messages with their occurrence count.
type ImportErrorSummary struct {
	Field   string
	Message string
	Count   int32
}

// finalize caps Errors at maxImportErrors and builds ErrorSummary from the full (uncapped) set of
// errors recorded during the run. Call once after all rows have been processed.
func (r *ImportResult) finalize() {
	r.ErrorSummary = summarizeImportErrors(r.Errors)
	if len(r.Errors) > maxImportErrors {
		r.Errors = r.Errors[:maxImportErrors]
	}
}

// summarizeImportErrors groups errors by (field, message), returning one summary entry per
// distinct combination with its total occurrence count, sorted by count descending then field.
func summarizeImportErrors(errs []ImportError) []ImportErrorSummary {
	type key struct{ field, message string }
	counts := make(map[key]int32, len(errs))
	order := make([]key, 0)
	for _, e := range errs {
		k := key{field: e.Field, message: e.Message}
		if _, seen := counts[k]; !seen {
			order = append(order, k)
		}
		counts[k]++
	}
	summary := make([]ImportErrorSummary, 0, len(order))
	for _, k := range order {
		summary = append(summary, ImportErrorSummary{Field: k.field, Message: k.message, Count: counts[k]})
	}
	sort.SliceStable(summary, func(i, j int) bool {
		if summary[i].Count != summary[j].Count {
			return summary[i].Count > summary[j].Count
		}
		return summary[i].Field < summary[j].Field
	})
	return summary
}

// ImportError represents a single import error.
type ImportError struct {
	RowNumber int32
	Field     string
	Message   string
}

// ImportHandler handles the ImportMBHeads command.
type ImportHandler struct {
	repo        mbhead.Repository
	noOfProcess noOfProcessValidator
}

// NewImportHandler creates a new ImportHandler. paramRepo backs the no-of-process membership
// check against the live mst_mb_param_option set (spec section 2.3), the same validator used by
// create and update so import cannot drift from those rules (spec section 5.3).
func NewImportHandler(repo mbhead.Repository, paramRepo mbparam.Repository) *ImportHandler {
	return &ImportHandler{repo: repo, noOfProcess: noOfProcessValidator{paramRepo: paramRepo}}
}

// Handle executes the import MB Heads command.
func (h *ImportHandler) Handle(ctx context.Context, cmd ImportCommand) (result *ImportResult, err error) {
	result = &ImportResult{Errors: []ImportError{}}

	rows, err := h.parseExcelFile(cmd.FileContent, cmd.FileName)
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return result, nil
	}

	cols, err := resolveMBHeadColumns(rows[0])
	if err != nil {
		return nil, err
	}

	if len(rows) == 1 {
		return result, nil
	}

	seen := newMBHeadSeenTracker()
	for i, row := range rows[1:] {
		rowNum := safeconv.IntToInt32(i + 2)
		h.processRow(ctx, row, rowNum, cols, cmd, result, seen)
	}

	result.finalize()
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

// The header-name column mapping (mbHeadRowData, mbHeadImportColumn, mbHeadImportColumns,
// mbHeadColumnIndex, normalizeHeader, resolveMBHeadColumns, parseMBHeadRow and getCell) lives
// in import_columns.go, where the same table is shared with the export and template handlers so
// the three cannot drift out of column-set agreement.

// mbHeadSeenTracker records dev codes and vs numbers already claimed earlier in the same file, so
// a second row claiming one is rejected before it ever reaches the database (spec section 5.3).
type mbHeadSeenTracker struct {
	devCodes  map[string]int32
	vsNumbers map[string]int32
}

func newMBHeadSeenTracker() *mbHeadSeenTracker {
	return &mbHeadSeenTracker{devCodes: map[string]int32{}, vsNumbers: map[string]int32{}}
}

// checkAndTrack returns a row-level duplicate error the first time a dev code or vs number is
// seen again later in the same file, else records both as claimed by rowNum.
func (t *mbHeadSeenTracker) checkAndTrack(devCode, vsNumber string, rowNum int32) *ImportError {
	if firstRow, dup := t.devCodes[devCode]; dup {
		return &ImportError{
			Field:   "mbh_dev_code",
			Message: fmt.Sprintf("development no %q duplicates row %d in this file", devCode, firstRow),
		}
	}
	if firstRow, dup := t.vsNumbers[vsNumber]; dup {
		return &ImportError{
			Field:   "mbh_vs_number",
			Message: fmt.Sprintf("vs number %q duplicates row %d in this file", vsNumber, firstRow),
		}
	}
	t.devCodes[devCode] = rowNum
	t.vsNumbers[vsNumber] = rowNum
	return nil
}

// requiredMBHeadField pairs one of the 11 required fields (spec section 2.1) with the domain
// sentinel raised when it is empty, so the row-level message matches the domain layer exactly.
type requiredMBHeadField struct {
	value string
	field string
	err   error
}

// requiredMBHeadFields lists the 11 required fields in spec section 2.1 order.
func requiredMBHeadFields(data mbHeadRowData) []requiredMBHeadField {
	return []requiredMBHeadField{
		{data.mgtName, "mbh_mgt_name", mbhead.ErrEmptyMgtName},
		{data.devCode, "mbh_dev_code", mbhead.ErrEmptyDevCode},
		{data.vsNumber, "mbh_vs_number", mbhead.ErrEmptyVsNumber},
		{data.noOfProcess, "mbh_no_of_process", mbhead.ErrEmptyNoOfProcess},
		{data.shadeCode, "mbh_shade_code", mbhead.ErrEmptyShadeCode},
		{data.shadeName, "mbh_shade_name", mbhead.ErrEmptyShadeName},
		{data.denier, "mbh_denier", mbhead.ErrInvalidDenier},
		{data.filament, "mbh_filament", mbhead.ErrInvalidFilament},
		{data.crossSection, "mbh_cross_section", mbhead.ErrEmptyCrossSection},
		{data.ldrPrsn, "mbh_ldr_prsn", mbhead.ErrInvalidLdrPercent},
		{data.finalProduct, "mbh_final_product", mbhead.ErrEmptyFinalProduct},
	}
}

// validateRequiredMBHeadFields returns the first missing required field as a row error, or nil
// when all 11 are present. Numeric fields (denier, filament, ldr %) reuse their "invalid" domain
// message on empty since the domain has no dedicated "empty" sentinel for them.
func validateRequiredMBHeadFields(data mbHeadRowData) *ImportError {
	for _, f := range requiredMBHeadFields(data) {
		if f.value == "" {
			return &ImportError{Field: f.field, Message: f.err.Error()}
		}
	}
	return nil
}

// mbHeadParsedNumerics holds the row's numeric/boolean fields after string parsing.
type mbHeadParsedNumerics struct {
	denier      float64
	filament    int
	ldrPrsn     float64
	dozing      *float64
	isBoughtout bool
}

// parseMBHeadRequiredNumerics parses the three required numeric fields (spec section 2.1). The
// caller must ensure presence via validateRequiredMBHeadFields first; a parse failure here means
// the cell held non-numeric text.
func parseMBHeadRequiredNumerics(data mbHeadRowData) (denier float64, filament int, ldrPrsn float64, field string, err error) {
	denier, err = strconv.ParseFloat(data.denier, 64)
	if err != nil {
		return 0, 0, 0, "mbh_denier", fmt.Errorf("invalid poy denier %q: %w", data.denier, err)
	}
	filament, err = strconv.Atoi(data.filament)
	if err != nil {
		return 0, 0, 0, "mbh_filament", fmt.Errorf("invalid poy filament %q: %w", data.filament, err)
	}
	ldrPrsn, err = strconv.ParseFloat(data.ldrPrsn, 64)
	if err != nil {
		return 0, 0, 0, "mbh_ldr_prsn", fmt.Errorf("invalid ldr %% %q: %w", data.ldrPrsn, err)
	}
	return denier, filament, ldrPrsn, "", nil
}

// parseMBHeadOptionalNumerics parses the optional numeric/boolean fields from a row.
func parseMBHeadOptionalNumerics(data mbHeadRowData) (dozing *float64, isBoughtout bool, field string, err error) {
	if data.dozing != "" {
		v, parseErr := strconv.ParseFloat(data.dozing, 64)
		if parseErr != nil {
			return nil, false, "dozing", fmt.Errorf("invalid dozing %q: %w", data.dozing, parseErr)
		}
		dozing = &v
	}
	if data.isBoughtout != "" {
		v, parseErr := strconv.ParseBool(data.isBoughtout)
		if parseErr != nil {
			return nil, false, "is_boughtout", fmt.Errorf("invalid is bought out %q: %w", data.isBoughtout, parseErr)
		}
		isBoughtout = v
	}
	return dozing, isBoughtout, "", nil
}

// buildImportShades converts the optional Shade Code/Name 2 and 3 columns into shade inputs.
// Duplicate-with-each-other and duplicate-with-header checks are enforced downstream by
// buildShades / entity.ReplaceShades (spec section 5.3), not re-implemented here.
func buildImportShades(data mbHeadRowData) []ShadeInput {
	var shades []ShadeInput
	if data.shadeCode2 != "" || data.shadeName2 != "" {
		shades = append(shades, ShadeInput{SeqNo: mbhead.MinShadeSeqNo, ShadeCode: data.shadeCode2, ShadeName: data.shadeName2})
	}
	if data.shadeCode3 != "" || data.shadeName3 != "" {
		shades = append(shades, ShadeInput{SeqNo: mbhead.MaxShadeSeqNo, ShadeCode: data.shadeCode3, ShadeName: data.shadeName3})
	}
	return shades
}

// mbHeadImportFieldErrors maps domain sentinels a row can still surface after the presence check
// (too-long, out-of-range numeric, invalid no-of-process, uniqueness, shade rules) to their
// import row field, mirroring the delivery layer's mbHeadFieldErrors table (grpc package) so the
// same sentinel reads the same field name in both the RPC response and the import report.
var mbHeadImportFieldErrors = []struct {
	err   error
	field string
}{
	{mbhead.ErrMgtNameTooLong, "mbh_mgt_name"},
	{mbhead.ErrDevCodeTooLong, "mbh_dev_code"},
	{mbhead.ErrDevCodeAlreadyExists, "mbh_dev_code"},
	{mbhead.ErrVsNumberTooLong, "mbh_vs_number"},
	{mbhead.ErrVsNumberAlreadyExists, "mbh_vs_number"},
	{mbhead.ErrNoOfProcessTooLong, "mbh_no_of_process"},
	{mbhead.ErrInvalidNoOfProcess, "mbh_no_of_process"},
	{mbhead.ErrShadeCodeTooLong, "mbh_shade_code"},
	{mbhead.ErrShadeNameTooLong, "mbh_shade_name"},
	{mbhead.ErrCrossSectionTooLong, "mbh_cross_section"},
	{mbhead.ErrInvalidDenier, "mbh_denier"},
	{mbhead.ErrInvalidFilament, "mbh_filament"},
	{mbhead.ErrInvalidLdrPercent, "mbh_ldr_prsn"},
	{mbhead.ErrFinalProductTooLong, "mbh_final_product"},
	{mbhead.ErrAlreadyExists, "mb_costing"},
	{mbhead.ErrMBCostingTooLong, "mb_costing"},
	{mbhead.ErrTooManyShades, "shades"},
	{mbhead.ErrDuplicateShadeCode, "shades"},
	{mbhead.ErrShadeCodeMatchesHeader, "shades"},
	{mbhead.ErrInvalidShadeSeqNo, "shades"},
	{mbhead.ErrDuplicateShadeSeqNo, "shades"},
}

// mbHeadImportErrorField resolves the row field a domain error belongs to, falling back to a
// generic field when the sentinel is not one of the mapped ones.
func mbHeadImportErrorField(err error, fallback string) string {
	for _, m := range mbHeadImportFieldErrors {
		if errors.Is(err, m.err) {
			return m.field
		}
	}
	return fallback
}

// appendMBHeadError records a domain error against the row it belongs to.
func appendMBHeadError(result *ImportResult, rowNum int32, fallbackField string, err error) {
	result.FailedCount++
	result.Errors = append(result.Errors, ImportError{
		RowNumber: rowNum,
		Field:     mbHeadImportErrorField(err, fallbackField),
		Message:   err.Error(),
	})
}

// processRow handles a single row import: the 11 required fields (spec section 2.1), in-file and
// DB dev-code/vs-number uniqueness (spec section 5.3), no-of-process membership against the live
// mst_mb_param_option set, then dispatches to create or duplicate handling. A row-level failure
// never aborts the rest of the file.
func (h *ImportHandler) processRow(
	ctx context.Context, row []string, rowNum int32, cols mbHeadColumnIndex,
	cmd ImportCommand, result *ImportResult, seen *mbHeadSeenTracker,
) {
	data := parseMBHeadRow(row, cols)

	if data.mbCosting == "" {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "mb_costing",
			Message:   "mb costing cannot be empty",
		})
		return
	}

	if ie := validateRequiredMBHeadFields(data); ie != nil {
		ie.RowNumber = rowNum
		result.FailedCount++
		result.Errors = append(result.Errors, *ie)
		return
	}

	if ie := seen.checkAndTrack(data.devCode, data.vsNumber, rowNum); ie != nil {
		ie.RowNumber = rowNum
		result.FailedCount++
		result.Errors = append(result.Errors, *ie)
		return
	}

	denier, filament, ldrPrsn, badField, err := parseMBHeadRequiredNumerics(data)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: badField, Message: err.Error()})
		return
	}

	dozing, isBoughtout, badField, err := parseMBHeadOptionalNumerics(data)
	if err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: badField, Message: err.Error()})
		return
	}

	if err := h.noOfProcess.validate(ctx, data.noOfProcess); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "mbh_no_of_process", Message: err.Error()})
		return
	}

	num := mbHeadParsedNumerics{denier: denier, filament: filament, ldrPrsn: ldrPrsn, dozing: dozing, isBoughtout: isBoughtout}

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
		h.handleDuplicate(ctx, data, num, rowNum, cmd, result)
		return
	}

	h.createMBHead(ctx, data, num, rowNum, cmd.CreatedBy, cmd.DryRun, result)
}

// handleDuplicate handles a duplicate mb_costing based on the specified action.
func (h *ImportHandler) handleDuplicate(
	ctx context.Context, data mbHeadRowData, num mbHeadParsedNumerics,
	rowNum int32, cmd ImportCommand, result *ImportResult,
) {
	switch cmd.DuplicateAction {
	case "skip":
		result.SkippedCount++
	case "update":
		h.updateExisting(ctx, data, num, rowNum, cmd.CreatedBy, cmd.DryRun, result)
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

// updateExisting updates an existing MB Head. When dryRun is true, every validation step runs
// exactly as on the real path, but repo.Update/repo.ReplaceShades are never called.
func (h *ImportHandler) updateExisting(
	ctx context.Context, data mbHeadRowData, num mbHeadParsedNumerics,
	rowNum int32, updatedBy string, dryRun bool, result *ImportResult,
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
		MgtName:         strPtrOrNil(data.mgtName),
		DevCode:         &data.devCode,
		VsNumber:        &data.vsNumber,
		NoOfProcess:     &data.noOfProcess,
		ShadeCode:       &data.shadeCode,
		ShadeName:       &data.shadeName,
		CrossSection:    &data.crossSection,
		MBHFinalProduct: &data.finalProduct,
		LustureCode:     &data.lustureCode,
		Denier:          &num.denier,
		Filament:        &num.filament,
		MBHLdrPrsn:      &num.ldrPrsn,
		Dozing:          num.dozing,
		IsBoughtout:     &num.isBoughtout,
	}

	if err := existing.Update(update, updatedBy); err != nil {
		appendMBHeadError(result, rowNum, "update", err)
		return
	}

	shades, err := buildShades(existing, buildImportShades(data), updatedBy)
	if err != nil {
		appendMBHeadError(result, rowNum, "shades", err)
		return
	}

	existingID := existing.ID()
	if err := checkMBHeadUniqueness(ctx, h.repo, data.devCode, data.vsNumber, &existingID); err != nil {
		appendMBHeadError(result, rowNum, "update", err)
		return
	}

	if !dryRun {
		if err := h.repo.Update(ctx, existing); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{
				RowNumber: rowNum,
				Field:     "update",
				Message:   fmt.Sprintf("failed to update: %v", err),
			})
			return
		}

		if err := h.repo.ReplaceShades(ctx, existing.ID(), shades, updatedBy); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "shades", Message: err.Error()})
			return
		}
	}

	result.UpdatedCount++
}

// createMBHead creates a new MB Head. When dryRun is true, every validation step runs exactly as
// on the real path, but repo.Create/repo.ReplaceShades are never called.
func (h *ImportHandler) createMBHead(
	ctx context.Context, data mbHeadRowData, num mbHeadParsedNumerics,
	rowNum int32, createdBy string, dryRun bool, result *ImportResult,
) {
	entity, err := mbhead.New(mbhead.NewInput{
		MBCosting:    data.mbCosting,
		MgtName:      data.mgtName,
		DevCode:      data.devCode,
		VsNumber:     data.vsNumber,
		NoOfProcess:  data.noOfProcess,
		ShadeCode:    data.shadeCode,
		ShadeName:    data.shadeName,
		CrossSection: data.crossSection,
		FinalProduct: data.finalProduct,
		Denier:       num.denier,
		Filament:     num.filament,
		LdrPrsn:      num.ldrPrsn,
		Dozing:       num.dozing,
		LustureCode:  data.lustureCode,
		CreatedBy:    createdBy,
		IsBoughtout:  num.isBoughtout,
	})
	if err != nil {
		appendMBHeadError(result, rowNum, "create", err)
		return
	}

	shades, err := buildShades(entity, buildImportShades(data), createdBy)
	if err != nil {
		appendMBHeadError(result, rowNum, "shades", err)
		return
	}

	if err := checkMBHeadUniqueness(ctx, h.repo, data.devCode, data.vsNumber, nil); err != nil {
		appendMBHeadError(result, rowNum, "create", err)
		return
	}

	if !dryRun && !h.persistCreatedMBHead(ctx, entity, shades, rowNum, createdBy, result) {
		return
	}

	result.SuccessCount++
}

// persistCreatedMBHead writes a newly built entity and its shades, reporting a row-level error
// and returning false on either failure. Split out of createMBHead solely to keep that function's
// dry-run branch under the nestif nesting-depth threshold.
func (h *ImportHandler) persistCreatedMBHead(
	ctx context.Context, entity *mbhead.Entity, shades []*mbhead.Shade, rowNum int32, createdBy string, result *ImportResult,
) bool {
	if err := h.repo.Create(ctx, entity); err != nil {
		result.FailedCount++
		result.Errors = append(result.Errors, ImportError{
			RowNumber: rowNum,
			Field:     "create",
			Message:   fmt.Sprintf("failed to create: %v", err),
		})
		return false
	}

	if len(shades) > 0 {
		if err := h.repo.ReplaceShades(ctx, entity.ID(), shades, createdBy); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, ImportError{RowNumber: rowNum, Field: "shades", Message: err.Error()})
			return false
		}
	}
	return true
}

// strPtrOrNil returns nil for an empty string, else a pointer to the string.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
