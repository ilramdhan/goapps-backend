// Package costproductmaster contains application use cases for CostProductMaster.
package costproductmaster

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	cptdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

const cpmImportBatchSize = 500

// AsyncImportHandler handles the async bulk import of CostProductMaster rows.
// It is called from the finance-worker after the job is created and queued.
type AsyncImportHandler struct {
	repo     domain.Repository
	typeRepo cptdomain.Repository
	jobRepo  costimportjob.Repository
}

// NewAsyncImportHandler creates a new AsyncImportHandler.
func NewAsyncImportHandler(
	repo domain.Repository,
	typeRepo cptdomain.Repository,
	jobRepo costimportjob.Repository,
) *AsyncImportHandler {
	return &AsyncImportHandler{repo: repo, typeRepo: typeRepo, jobRepo: jobRepo}
}

// AsyncImportError is a row-level import error.
type AsyncImportError struct {
	RowNumber int32
	Field     string
	Message   string
}

// Handle executes the async import. jobID must match an existing PENDING job.
func (h *AsyncImportHandler) Handle(ctx context.Context, jobID int64, fileContent []byte, fileName, updatedBy string) error {
	job, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load import job %d: %w", jobID, err)
	}

	rows, parseErr := parseCPMExcelFile(fileContent, fileName)
	if parseErr != nil {
		job.MarkFailed(parseErr.Error())
		if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
			log.Error().Err(updateErr).Int64("job_id", jobID).Msg("cpm import: failed to mark job failed after parse error")
		}
		return parseErr
	}

	dataRows := rows
	if len(rows) > 0 {
		dataRows = rows[1:] // skip header
	}

	job.SetTotalRows(len(dataRows))
	job.MarkRunning()
	if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
		log.Warn().Err(updateErr).Int64("job_id", jobID).Msg("cpm import: failed to persist RUNNING state")
	}

	// Build product-type code → ID cache upfront to avoid repeated DB calls.
	typeCache := make(map[string]int32)

	var (
		totalSuccess int
		totalFailed  int
		totalSkipped int
		processed    int
		errs         []AsyncImportError
	)

	for batchStart := 0; batchStart < len(dataRows); batchStart += cpmImportBatchSize {
		end := batchStart + cpmImportBatchSize
		if end > len(dataRows) {
			end = len(dataRows)
		}
		batch := dataRows[batchStart:end]

		batchSuccess, batchFailed, batchSkipped, batchErrs := h.processBatch(ctx, batch, batchStart+2, typeCache, updatedBy)
		totalSuccess += batchSuccess
		totalFailed += batchFailed
		totalSkipped += batchSkipped
		errs = append(errs, batchErrs...)
		processed += len(batch)

		job.UpdateProgress(processed, totalSuccess, totalFailed, totalSkipped)
		if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
			log.Warn().Err(updateErr).Int64("job_id", jobID).Msg("cpm import: failed to update progress")
		}
	}

	job.MarkDone("")
	if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
		log.Error().Err(updateErr).Int64("job_id", jobID).Msg("cpm import: failed to persist completion")
	}

	if len(errs) > 0 {
		log.Warn().Int("error_count", len(errs)).Int64("job_id", jobID).Msg("cpm import completed with row errors")
	}
	return nil
}

// cpmRowData holds parsed per-row values.
type cpmRowData struct {
	productCode   string
	productType   string
	productName   string
	gradeCode     string
	shadeCode     string
	shadeName     string
	erpItemCode   string
	erpGradeCode1 string
	erpGradeCode2 string
	flex01        string
	flex02        string
	flex03        string
	description   string
	isActive      string
}

// parseCPMExcelFile opens the file and returns all rows (including the header row).
func parseCPMExcelFile(content []byte, fileName string) ([][]string, error) {
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
			log.Warn().Err(closeErr).Msg("cpm import: failed to close excel file")
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

// parseCPMRow extracts cell values by position.
func parseCPMRow(row []string) cpmRowData {
	return cpmRowData{
		productCode:   getCPMCell(row, 0),
		productType:   getCPMCell(row, 1),
		productName:   getCPMCell(row, 2),
		gradeCode:     getCPMCell(row, 3),
		shadeCode:     getCPMCell(row, 4),
		shadeName:     getCPMCell(row, 5),
		erpItemCode:   getCPMCell(row, 6),
		erpGradeCode1: getCPMCell(row, 7),
		erpGradeCode2: getCPMCell(row, 8),
		flex01:        getCPMCell(row, 9),
		flex02:        getCPMCell(row, 10),
		flex03:        getCPMCell(row, 11),
		description:   getCPMCell(row, 12),
		isActive:      getCPMCell(row, 13),
	}
}

// processBatch upserts a slice of rows and returns per-batch counters.
func (h *AsyncImportHandler) processBatch(
	ctx context.Context,
	rows [][]string,
	startRowNum int,
	typeCache map[string]int32,
	updatedBy string,
) (success, failed, skipped int, errs []AsyncImportError) {
	batch := make([]*domain.CostProductMaster, 0, len(rows))

	for i, row := range rows {
		rowNum := safeconv.IntToInt32(startRowNum + i)
		data := parseCPMRow(row)

		if data.productCode == "" {
			skipped++
			continue
		}
		if data.productName == "" {
			failed++
			errs = append(errs, AsyncImportError{RowNumber: rowNum, Field: "product_name", Message: "product_name cannot be empty"})
			continue
		}

		typeID, resolveErr := h.resolveTypeCode(ctx, data.productType, typeCache)
		if resolveErr != nil {
			failed++
			errs = append(errs, AsyncImportError{RowNumber: rowNum, Field: "product_type_code", Message: resolveErr.Error()})
			continue
		}

		p, buildErr := buildEntity(data, typeID)
		if buildErr != nil {
			failed++
			errs = append(errs, AsyncImportError{RowNumber: rowNum, Field: "entity", Message: buildErr.Error()})
			continue
		}
		batch = append(batch, p)
	}

	if len(batch) == 0 {
		return success, failed, skipped, errs
	}

	_, upsertErr := h.repo.BulkCreate(ctx, batch, updatedBy)
	if upsertErr != nil {
		// Mark the whole mini-batch as failed.
		for range batch {
			failed++
		}
		errs = append(errs, AsyncImportError{
			RowNumber: safeconv.IntToInt32(startRowNum),
			Field:     "bulk_create",
			Message:   fmt.Sprintf("batch upsert failed: %v", upsertErr),
		})
		return success, failed, skipped, errs
	}
	success += len(batch)
	return success, failed, skipped, errs
}

// resolveTypeCode looks up the product type code, using typeCache to avoid redundant DB calls.
func (h *AsyncImportHandler) resolveTypeCode(ctx context.Context, typeCode string, cache map[string]int32) (int32, error) {
	if typeCode == "" {
		return 0, fmt.Errorf("product_type_code cannot be empty")
	}
	if id, ok := cache[typeCode]; ok {
		return id, nil
	}
	pt, err := h.typeRepo.GetByCode(ctx, typeCode)
	if err != nil {
		return 0, fmt.Errorf("product type '%s' not found", typeCode)
	}
	cache[typeCode] = pt.TypeID()
	return pt.TypeID(), nil
}

// buildEntity constructs a CostProductMaster aggregate from a parsed row.
func buildEntity(data cpmRowData, typeID int32) (*domain.CostProductMaster, error) {
	p, err := domain.New(domain.NewInput{
		ProductTypeID: typeID,
		ProductName:   data.productName,
		ShadeCode:     data.shadeCode,
		GradeCode:     data.gradeCode,
		Description:   data.description,
		ShadeName:     data.shadeName,
		Flex01:        data.flex01,
		Flex02:        data.flex02,
		Flex03:        data.flex03,
		ActorUserID:   "",
	})
	if err != nil {
		return nil, err
	}
	// BulkCreate uses product_code from SetGeneratedCode. For import rows that already
	// supply a code (e.g., re-import / upsert scenario), inject it directly.
	if data.productCode != "" {
		p.SetGeneratedCode(0, data.productCode)
	}
	if data.erpItemCode != "" {
		p.LinkErp(data.erpItemCode, data.erpGradeCode1, data.erpGradeCode2, "")
	}
	isActive := parseCPMIsActive(data.isActive)
	if !isActive {
		p.Deactivate("")
	}
	return p, nil
}

// parseCPMIsActive interprets an optional active flag string; defaults to true.
func parseCPMIsActive(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "false", "no", "0", "inactive":
		return false
	default:
		return true
	}
}

// getCPMCell safely returns the trimmed cell value at the given index.
func getCPMCell(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}
