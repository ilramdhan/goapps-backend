// Package costproductapplicableparam contains application use cases for
// Cost Product Applicable Param (CAPP_) operations.
package costproductapplicableparam

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

const cappImportBatchSize = 5000

// AsyncImportHandler handles the async bulk import of CAPP rows.
type AsyncImportHandler struct {
	repo    cpp.Repository
	jobRepo costimportjob.Repository
}

// NewAsyncImportHandler creates a new AsyncImportHandler.
func NewAsyncImportHandler(repo cpp.Repository, jobRepo costimportjob.Repository) *AsyncImportHandler {
	return &AsyncImportHandler{repo: repo, jobRepo: jobRepo}
}

// AsyncImportError is a row-level import error.
type AsyncImportError struct {
	RowNumber int32
	Field     string
	Message   string
}

// Handle executes the async CAPP import.
func (h *AsyncImportHandler) Handle(ctx context.Context, jobID int64, fileContent []byte, fileName string) error {
	job, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load import job %d: %w", jobID, err)
	}

	rows, parseErr := parseCAPPExcelFile(fileContent, fileName)
	if parseErr != nil {
		job.MarkFailed(parseErr.Error())
		if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
			log.Error().Err(updateErr).Int64("job_id", jobID).Msg("capp import: failed to mark job failed")
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
		log.Warn().Err(updateErr).Int64("job_id", jobID).Msg("capp import: failed to persist RUNNING state")
	}

	// Pre-build caches for product_code and param_code lookups.
	productCache := make(map[string]int64)
	paramCache := make(map[string]string) // param_code → param UUID string

	var (
		totalSuccess int
		totalFailed  int
		totalSkipped int
		processed    int
	)

	for batchStart := 0; batchStart < len(dataRows); batchStart += cappImportBatchSize {
		end := batchStart + cappImportBatchSize
		if end > len(dataRows) {
			end = len(dataRows)
		}
		batch := dataRows[batchStart:end]

		batchSuccess, batchFailed, batchSkipped := h.processBatch(ctx, batch, batchStart+2, productCache, paramCache)
		totalSuccess += batchSuccess
		totalFailed += batchFailed
		totalSkipped += batchSkipped
		processed += len(batch)

		job.UpdateProgress(processed, totalSuccess, totalFailed, totalSkipped)
		if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
			log.Warn().Err(updateErr).Int64("job_id", jobID).Msg("capp import: failed to update progress")
		}
	}

	job.MarkDone("")
	if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
		log.Error().Err(updateErr).Int64("job_id", jobID).Msg("capp import: failed to persist completion")
	}
	return nil
}

// cappRowData holds parsed per-row values.
type cappRowData struct {
	productCode  string
	paramCode    string
	isRequired   string
	displayOrder string
}

// parseCAPPExcelFile opens the file and returns all rows.
func parseCAPPExcelFile(content []byte, fileName string) ([][]string, error) {
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
			log.Warn().Err(closeErr).Msg("capp import: failed to close excel file")
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

// parseCAPPRow extracts cell values by position.
func parseCAPPRow(row []string) cappRowData {
	return cappRowData{
		productCode:  getCAPPCell(row, 0),
		paramCode:    getCAPPCell(row, 1),
		isRequired:   getCAPPCell(row, 2),
		displayOrder: getCAPPCell(row, 3),
	}
}

// processBatch processes a slice of rows and returns per-batch counters.
func (h *AsyncImportHandler) processBatch(
	ctx context.Context,
	rows [][]string,
	startRowNum int,
	productCache map[string]int64,
	paramCache map[string]string,
) (success, failed, skipped int) {
	for i, row := range rows {
		rowNum := safeconv.IntToInt32(startRowNum + i)
		data := parseCAPPRow(row)

		if data.productCode == "" || data.paramCode == "" {
			skipped++
			continue
		}

		productSysID, resolveErr := h.resolveProduct(ctx, data.productCode, productCache)
		if resolveErr != nil {
			failed++
			log.Warn().Int32("row", rowNum).Str("product_code", data.productCode).Err(resolveErr).Msg("capp import: resolve product failed")
			continue
		}

		paramID, resolveErr := h.resolveParam(ctx, data.paramCode, paramCache)
		if resolveErr != nil {
			failed++
			log.Warn().Int32("row", rowNum).Str("param_code", data.paramCode).Err(resolveErr).Msg("capp import: resolve param failed")
			continue
		}

		isRequired := parseBoolFlag(data.isRequired, false)
		displayOrder := parseDisplayOrder(data.displayOrder)

		a := &cpp.Applicability{
			ProductSysID: productSysID,
			ParamID:      paramID,
			IsRequired:   isRequired,
			DisplayOrder: displayOrder,
			CreatedBy:    "import",
		}
		if upsertErr := h.repo.AddApplicable(ctx, a); upsertErr != nil {
			failed++
			log.Warn().Int32("row", rowNum).Err(upsertErr).Msg("capp import: upsert failed")
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

// parseBoolFlag parses a boolean cell string.
func parseBoolFlag(raw string, defaultVal bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	default:
		return defaultVal
	}
}

// parseDisplayOrder parses an optional display order cell.
func parseDisplayOrder(raw string) *int32 {
	if raw == "" {
		return nil
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < 0 {
		return nil
	}
	v := int32(n) //nolint:gosec // ParseInt with bitSize=32 guarantees int32 range
	return &v
}

// getCAPPCell safely returns the trimmed cell value at the given index.
func getCAPPCell(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}
