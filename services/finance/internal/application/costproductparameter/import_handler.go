// Package costproductparameter wires CPP_ use cases.
package costproductparameter

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

const cppImportBatchSize = 5000

// AsyncImportHandler handles the async bulk import of CPP rows.
type AsyncImportHandler struct {
	repo    cpp.Repository
	jobRepo costimportjob.Repository
	// mbSpinRepo resolves cpp_value_mb_spin_id for MB_SPIN lookup parameters,
	// mirroring the interactive Upsert path (Handlers.resolveMBSpinID via
	// resolveMBSpinValue). Nil-safe: when nil, resolution is skipped entirely
	// and cpp_value_text is still written exactly as before this column
	// existed.
	mbSpinRepo mbspin.Repository
}

// NewAsyncImportHandler creates a new AsyncImportHandler.
func NewAsyncImportHandler(repo cpp.Repository, jobRepo costimportjob.Repository, mbSpinRepo mbspin.Repository) *AsyncImportHandler {
	return &AsyncImportHandler{repo: repo, jobRepo: jobRepo, mbSpinRepo: mbSpinRepo}
}

// AsyncImportError is a row-level import error.
type AsyncImportError struct {
	RowNumber int32
	Field     string
	Message   string
}

// Handle executes the async CPP import.
func (h *AsyncImportHandler) Handle(ctx context.Context, jobID int64, fileContent []byte, fileName string) error {
	job, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load import job %d: %w", jobID, err)
	}

	rows, parseErr := parseCPPExcelFile(fileContent, fileName)
	if parseErr != nil {
		job.MarkFailed(parseErr.Error())
		if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
			log.Error().Err(updateErr).Int64("job_id", jobID).Msg("cpp import: failed to mark job failed")
		}
		return parseErr
	}

	dataRows := rows
	if len(rows) > 0 {
		dataRows = rows[1:]
	}

	job.SetTotalRows(len(dataRows))
	job.MarkRunning()
	if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
		log.Warn().Err(updateErr).Int64("job_id", jobID).Msg("cpp import: failed to persist RUNNING state")
	}

	// Pre-build caches for product_code and param_code lookups.
	productCache := make(map[string]int64)
	paramCache := make(map[string]string) // param_code → param UUID string
	paramMetaCache := make(map[string]*cpp.ParamMeta)
	// mbSpinCache memoizes resolveMBSpinValue by raw incoming text (ORION code
	// or UUID string): 177 legacy ORION codes are shared by up to 16 rows
	// each in production, so a wide import file can repeat the same code many
	// times. A cache hit for "no match" is stored as a present key with a nil
	// value (checked via comma-ok, not a nil check on the value).
	mbSpinCache := make(map[string]*uuid.UUID)

	var (
		totalSuccess int
		totalFailed  int
		totalSkipped int
		processed    int
	)

	for batchStart := 0; batchStart < len(dataRows); batchStart += cppImportBatchSize {
		end := batchStart + cppImportBatchSize
		if end > len(dataRows) {
			end = len(dataRows)
		}
		batch := dataRows[batchStart:end]

		batchSuccess, batchFailed, batchSkipped := h.processBatch(ctx, batch, batchStart+2, productCache, paramCache, paramMetaCache, mbSpinCache)
		totalSuccess += batchSuccess
		totalFailed += batchFailed
		totalSkipped += batchSkipped
		processed += len(batch)

		job.UpdateProgress(processed, totalSuccess, totalFailed, totalSkipped)
		if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
			log.Warn().Err(updateErr).Int64("job_id", jobID).Msg("cpp import: failed to update progress")
		}
	}

	job.MarkDone("")
	if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
		log.Error().Err(updateErr).Int64("job_id", jobID).Msg("cpp import: failed to persist completion")
	}
	return nil
}

// cppRowData holds parsed per-row values.
type cppRowData struct {
	productCode  string
	paramCode    string
	valueNumeric string
	valueText    string
	valueFlag    string
}

// parseCPPExcelFile opens the file and returns all rows.
func parseCPPExcelFile(content []byte, fileName string) ([][]string, error) {
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
			log.Warn().Err(closeErr).Msg("cpp import: failed to close excel file")
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

// parseCPPRow extracts cell values by position.
func parseCPPRow(row []string) cppRowData {
	return cppRowData{
		productCode:  getCPPCell(row, 0),
		paramCode:    getCPPCell(row, 1),
		valueNumeric: getCPPCell(row, 2),
		valueText:    getCPPCell(row, 3),
		valueFlag:    getCPPCell(row, 4),
	}
}

// processBatch processes a slice of rows and returns per-batch counters.
func (h *AsyncImportHandler) processBatch(
	ctx context.Context,
	rows [][]string,
	startRowNum int,
	productCache map[string]int64,
	paramCache map[string]string,
	paramMetaCache map[string]*cpp.ParamMeta,
	mbSpinCache map[string]*uuid.UUID,
) (success, failed, skipped int) {
	for i, row := range rows {
		rowNum := safeconv.IntToInt32(startRowNum + i)
		data := parseCPPRow(row)

		if data.productCode == "" || data.paramCode == "" {
			skipped++
			continue
		}

		productSysID, resolveErr := h.resolveProduct(ctx, data.productCode, productCache)
		if resolveErr != nil {
			failed++
			log.Warn().Int32("row", rowNum).Str("product_code", data.productCode).Err(resolveErr).Msg("cpp import: resolve product failed")
			continue
		}

		paramID, resolveErr := h.resolveParam(ctx, data.paramCode, paramCache)
		if resolveErr != nil {
			failed++
			log.Warn().Int32("row", rowNum).Str("param_code", data.paramCode).Err(resolveErr).Msg("cpp import: resolve param failed")
			continue
		}

		meta, metaErr := h.resolveParamMeta(ctx, paramID, data.paramCode, paramMetaCache)
		if metaErr != nil {
			failed++
			log.Warn().Int32("row", rowNum).Str("param_code", data.paramCode).Err(metaErr).Msg("cpp import: get param meta failed")
			continue
		}

		valueNumeric, valueText, valueFlag, shapeErr := resolveValueShape(data, meta.DataType)
		if shapeErr != nil {
			failed++
			log.Warn().Int32("row", rowNum).Str("param_code", data.paramCode).Err(shapeErr).Msg("cpp import: invalid value shape")
			continue
		}

		v := &cpp.Value{
			ProductSysID: productSysID,
			ParamID:      paramID,
			ValueNumeric: valueNumeric,
			ValueText:    valueText,
			ValueFlag:    valueFlag,
			FilledBy:     "import",
			CreatedBy:    "import",
		}
		if meta.LookupMasterCode == mbSpinLookupMasterCode {
			v.ValueMBSpinID = h.resolveMBSpinIDCached(ctx, valueText, mbSpinCache)
		}
		if upsertErr := h.repo.Upsert(ctx, v); upsertErr != nil {
			failed++
			log.Warn().Int32("row", rowNum).Err(upsertErr).Msg("cpp import: upsert failed")
			continue
		}
		success++
	}
	return success, failed, skipped
}

// resolveProduct looks up productSysID from cache or DB.
func (h *AsyncImportHandler) resolveProduct(ctx context.Context, code string, cache map[string]int64) (int64, error) {
	if id, ok := cache[code]; ok {
		return id, nil
	}
	id, err := h.repo.GetProductSysIDByCode(ctx, code)
	if err != nil {
		return 0, fmt.Errorf("product '%s' not found: %w", code, err)
	}
	cache[code] = id
	return id, nil
}

// resolveParam looks up paramID UUID from cache or DB.
func (h *AsyncImportHandler) resolveParam(ctx context.Context, code string, cache map[string]string) (uuid.UUID, error) {
	if idStr, ok := cache[code]; ok {
		parsed, parseErr := uuid.Parse(idStr)
		if parseErr != nil {
			return uuid.Nil, fmt.Errorf("invalid cached uuid for param '%s': %w", code, parseErr)
		}
		return parsed, nil
	}
	id, err := h.repo.GetParamIDByCode(ctx, code)
	if err != nil {
		return uuid.Nil, fmt.Errorf("param '%s' not found: %w", code, err)
	}
	cache[code] = id.String()
	return id, nil
}

// resolveParamMeta returns cached ParamMeta or fetches it.
func (h *AsyncImportHandler) resolveParamMeta(ctx context.Context, paramID uuid.UUID, code string, cache map[string]*cpp.ParamMeta) (*cpp.ParamMeta, error) {
	if m, ok := cache[code]; ok {
		return m, nil
	}
	meta, err := h.repo.GetMeta(ctx, paramID)
	if err != nil {
		return nil, fmt.Errorf("get meta for param '%s': %w", code, err)
	}
	cache[code] = meta
	return meta, nil
}

// resolveMBSpinIDCached applies the shared MB_SPIN resolution rule
// (resolveMBSpinValue, also used by the interactive Handlers.Upsert path) with
// a per-import cache keyed by the raw incoming text. This avoids one
// ResolveUniqueByOrionItemCode round-trip per row: 177 legacy ORION codes are
// shared by up to 16 rows each in production, so a wide import file can repeat
// the same code many times within a batch. A cache hit for "no match" is
// stored as a present key with a nil value (checked via comma-ok).
func (h *AsyncImportHandler) resolveMBSpinIDCached(ctx context.Context, valueText *string, cache map[string]*uuid.UUID) *uuid.UUID {
	if valueText == nil || *valueText == "" {
		return nil
	}
	key := *valueText
	if id, ok := cache[key]; ok {
		return id
	}
	id := resolveMBSpinValue(ctx, h.mbSpinRepo, valueText)
	cache[key] = id
	return id
}

// resolveValueShape picks the right value pointer based on the column data and data_type.
// Exactly one of the three output pointers will be non-nil on success.
func resolveValueShape(data cppRowData, dataType string) (valueNumeric, valueText *string, valueFlag *bool, err error) {
	switch cpp.DataType(strings.ToUpper(strings.TrimSpace(dataType))) {
	case cpp.DataTypeNumber:
		if data.valueNumeric == "" {
			return nil, nil, nil, fmt.Errorf("param data_type is NUMBER but value_numeric is empty")
		}
		vn := data.valueNumeric
		return &vn, nil, nil, nil
	case cpp.DataTypeText:
		if data.valueText == "" {
			return nil, nil, nil, fmt.Errorf("param data_type is TEXT but value_text is empty")
		}
		vt := data.valueText
		return nil, &vt, nil, nil
	case cpp.DataTypeBoolean:
		if data.valueFlag == "" {
			return nil, nil, nil, fmt.Errorf("param data_type is BOOLEAN but value_flag is empty")
		}
		vf := parseBoolValue(data.valueFlag)
		return nil, nil, &vf, nil
	default:
		return nil, nil, nil, fmt.Errorf("unknown data_type: %s", dataType)
	}
}

// parseBoolValue parses a boolean string.
func parseBoolValue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "yes", "1":
		return true
	default:
		return false
	}
}

// getCPPCell safely returns the trimmed cell value at the given index.
func getCPPCell(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}
