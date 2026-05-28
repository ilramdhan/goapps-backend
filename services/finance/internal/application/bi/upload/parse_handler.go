package upload

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	uploaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/upload"
)

// maxUploadRows caps how many data rows one upload may contain.
const maxUploadRows = 50000

// ParseCommand is the input to ParseHandler.
type ParseCommand struct {
	TargetType  string
	FileName    string
	FileContent []byte
	UploadedBy  uuid.UUID
}

// ParseResult bundles the created session and the collected (capped) per-row errors.
type ParseResult struct {
	Upload *uploaddomain.Upload
	Errors []uploaddomain.FieldError
}

// ParseHandler parses an uploaded xlsx, validates rows, and writes them to staging.
type ParseHandler struct {
	repo     uploaddomain.Repository
	sourceID func(ctx context.Context) (uuid.UUID, error)
}

// NewParseHandler constructs a ParseHandler. sourceLookup resolves the EXCEL_UPLOAD
// data-source id (cached upstream is fine).
func NewParseHandler(repo uploaddomain.Repository, sourceLookup func(ctx context.Context) (uuid.UUID, error)) *ParseHandler {
	return &ParseHandler{repo: repo, sourceID: sourceLookup}
}

// Handle parses, validates, stages, and creates the upload session.
func (h *ParseHandler) Handle(ctx context.Context, cmd ParseCommand) (*ParseResult, error) {
	if cmd.TargetType == "" {
		return nil, uploaddomain.ErrInvalidTargetType
	}
	rows, err := openRows(cmd.FileContent, cmd.FileName)
	if err != nil {
		return nil, err
	}
	if len(rows) <= 1 {
		return nil, uploaddomain.ErrNoDataRows
	}
	dataRows := rows[1:]
	if len(dataRows) > maxUploadRows {
		return nil, uploaddomain.ErrTooManyRows
	}

	colIdx := mapColumns(rows[0])
	staging, fieldErrs := h.validateRows(dataRows, colIdx, cmd.TargetType)

	sourceID, err := h.sourceID(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve excel upload source: %w", err)
	}

	session := uploaddomain.NewUpload(
		sourceID, cmd.TargetType, cmd.FileName, len(cmd.FileContent),
		uploaddomain.StatusValidated, cmd.UploadedBy,
	)
	valid, invalid := countStatuses(staging)
	session.SetCounts(len(staging), valid, invalid, 0)

	if err := h.persist(ctx, session, staging); err != nil {
		return nil, err
	}

	overwrites, err := h.repo.MarkOverwrites(ctx, session.ID())
	if err != nil {
		return nil, fmt.Errorf("mark overwrites: %w", err)
	}
	session.SetOverwriteRows(overwrites)
	if err := h.repo.UpdateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("update overwrite count: %w", err)
	}

	return &ParseResult{Upload: session, Errors: capErrors(fieldErrs)}, nil
}

func (h *ParseHandler) persist(ctx context.Context, session *uploaddomain.Upload, staging []uploaddomain.StagingRow) error {
	if err := h.repo.CreateSession(ctx, session); err != nil {
		return fmt.Errorf("create upload session: %w", err)
	}
	if err := h.repo.InsertStaging(ctx, session.ID(), staging); err != nil {
		return fmt.Errorf("insert staging rows: %w", err)
	}
	return nil
}

// validateRows validates each data row and marks intra-file duplicate keys invalid.
func (h *ParseHandler) validateRows(dataRows [][]string, colIdx map[string]int, targetType string) ([]uploaddomain.StagingRow, []uploaddomain.FieldError) {
	staging := make([]uploaddomain.StagingRow, 0, len(dataRows))
	var fieldErrs []uploaddomain.FieldError
	seen := make(map[string]struct{}, len(dataRows))

	for i, cells := range dataRows {
		rowNumber := i + 2 // 1-indexed, header is row 1
		raw := buildRawRow(cells, colIdx)
		row, errs := uploaddomain.ValidateRow(raw, targetType, rowNumber)
		if row.ValidationStatus == uploaddomain.ValidationValid {
			key := uploaddomain.BusinessKey(row)
			if _, dup := seen[key]; dup {
				row.ValidationStatus = uploaddomain.ValidationInvalid
				row.ValidationMsg = "duplicate business key within file"
				errs = append(errs, uploaddomain.FieldError{
					Row: rowNumber, Column: "GROUP_1", Value: raw.Group1,
					Issue: "duplicate business key within file", Expected: "unique",
				})
			} else {
				seen[key] = struct{}{}
			}
		}
		staging = append(staging, row)
		fieldErrs = append(fieldErrs, errs...)
	}
	return staging, fieldErrs
}

// openRows opens an xlsx byte slice and returns the FACT_METRIC sheet rows
// (falling back to the first sheet).
func openRows(content []byte, fileName string) ([][]string, error) {
	if !strings.HasSuffix(strings.ToLower(fileName), ".xlsx") {
		return nil, fmt.Errorf("unsupported file format: expected .xlsx")
	}
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("open excel file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close uploaded Excel file")
		}
	}()

	sheet := uploadSheetName
	idx, idxErr := f.GetSheetIndex(uploadSheetName)
	if idxErr != nil || idx < 0 {
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return nil, fmt.Errorf("no sheets found in file")
		}
		sheet = sheets[0]
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("read rows: %w", err)
	}
	return rows, nil
}

// mapColumns builds a header-name → column-index map (case-insensitive),
// tolerating column reordering.
func mapColumns(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, h := range header {
		key := strings.ToUpper(strings.TrimSpace(h))
		if key == "" {
			continue
		}
		if _, exists := idx[key]; !exists {
			idx[key] = i
		}
	}
	return idx
}

// buildRawRow extracts cells by mapped column index, tolerating sparse rows.
func buildRawRow(cells []string, colIdx map[string]int) uploaddomain.RawRow {
	get := func(col string) string {
		i, ok := colIdx[col]
		if !ok || i >= len(cells) {
			return ""
		}
		return strings.TrimSpace(cells[i])
	}
	// getAny returns the first non-empty match — lets us accept both the PRD template header
	// ("PERIODE") and the analyst's source-file header ("PERIODE_DATE") for the date column.
	getAny := func(cols ...string) string {
		for _, c := range cols {
			if v := get(c); v != "" {
				return v
			}
		}
		return ""
	}
	return uploaddomain.RawRow{
		Type:        get(uploadHeaders[0]),
		Group1:      get(uploadHeaders[1]),
		Group2:      get(uploadHeaders[2]),
		Group3:      get(uploadHeaders[3]),
		Group1Order: get(uploadHeaders[4]),
		Group2Order: get(uploadHeaders[5]),
		Group3Order: get(uploadHeaders[6]),
		Grain:       get(uploadHeaders[7]),
		Periode:     getAny(uploadHeaders[8], "PERIODE_DATE"),
		Value:       get(uploadHeaders[9]),
		UOM:         get(uploadHeaders[10]),
		Scenario:    get(uploadHeaders[11]),
	}
}

func countStatuses(rows []uploaddomain.StagingRow) (valid, invalid int) {
	for _, r := range rows {
		if r.ValidationStatus == uploaddomain.ValidationInvalid {
			invalid++
		} else {
			valid++
		}
	}
	return valid, invalid
}

// capErrors limits the returned error slice for payload size; counts stay accurate.
func capErrors(errs []uploaddomain.FieldError) []uploaddomain.FieldError {
	const maxReturned = 200
	if len(errs) > maxReturned {
		return errs[:maxReturned]
	}
	return errs
}
