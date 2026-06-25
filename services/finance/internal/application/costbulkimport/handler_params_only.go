package costbulkimport

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
)

// ParamOnlyImportHandler imports product_parameters and product_applicable_params
// from a file that does NOT include a product_master sheet. Products must already
// exist in the database from a prior bulk import run.
//
// Workflow:
//  1. Load ParamMap (all active params) and ProductMap (all active products' legacy IDs) from DB.
//  2. Pre-validate all param rows — all-or-nothing, zero rows written on any error.
//  3. Write all rows if validation passes.
type ParamOnlyImportHandler struct {
	jobRepo costimportjob.Repository
	cpmRepo costproductmaster.Repository
	cppRepo costproductparameter.Repository
	storage storage.Service
	logger  zerolog.Logger
}

// NewParamOnlyImportHandler constructs the handler.
func NewParamOnlyImportHandler(
	jobRepo costimportjob.Repository,
	cpmRepo costproductmaster.Repository,
	cppRepo costproductparameter.Repository,
	storageSvc storage.Service,
	logger zerolog.Logger,
) *ParamOnlyImportHandler {
	return &ParamOnlyImportHandler{
		jobRepo: jobRepo,
		cpmRepo: cpmRepo,
		cppRepo: cppRepo,
		storage: storageSvc,
		logger:  logger,
	}
}

// Handle processes a params-only import job.
func (h *ParamOnlyImportHandler) Handle(ctx context.Context, jobID int64, fileContent []byte, fileName string) error {
	h.logger.Info().Int64("job_id", jobID).Str("file_name", fileName).Msg("params-only import: starting")
	job, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load params-only import job %d: %w", jobID, err)
	}

	f, openErr := excelize.OpenReader(bytes.NewReader(fileContent))
	if openErr != nil {
		job.MarkFailed(openErr.Error())
		h.updateJob(ctx, jobID, job)
		return openErr
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			h.logger.Warn().Err(closeErr).Int64("job_id", jobID).Msg("params-only import: close excel failed")
		}
	}()

	actor := job.CreatedBy()
	now := time.Now()

	job.MarkRunning()
	h.updateJob(ctx, jobID, job)

	maps, mapsErr := h.loadParamOnlyMaps(ctx)
	if mapsErr != nil {
		job.MarkFailed(mapsErr.Error())
		h.updateJob(ctx, jobID, job)
		return mapsErr
	}

	// === Phase 1: pre-validation (no DB writes) ===
	preResults := preValidateParamSheets(f, maps)
	if countErrors(preResults) > 0 {
		h.logger.Warn().
			Int64("job_id", jobID).
			Int("error_count", countErrors(preResults)).
			Msg("params-only import: pre-validation failed — aborting")
		errorKey := h.uploadErrorReport(ctx, jobID, preResults)
		job.MarkFailed(fmt.Sprintf(
			"validation failed: %d error(s) — see error report",
			countErrors(preResults),
		))
		if errorKey != "" {
			job.SetErrorFile(errorKey)
		}
		h.updateJob(ctx, jobID, job)
		return nil
	}

	// === Phase 2: write ===
	_, _, _, s2Err := processCPP(ctx, f, maps, h.cppRepo, actor, now)
	if s2Err != nil {
		job.MarkFailed(s2Err.Error())
		h.updateJob(ctx, jobID, job)
		return s2Err
	}
	_, _, _, s3Err := processCAP(ctx, f, maps, h.cppRepo, actor, now)
	if s3Err != nil {
		job.MarkFailed(s3Err.Error())
		h.updateJob(ctx, jobID, job)
		return s3Err
	}

	job.MarkDone("")
	h.updateJob(ctx, jobID, job)
	return nil
}

// loadParamOnlyMaps preloads ParamMap and ProductMap from the database.
// ProductMap is populated from existing product legacy IDs, not from an Excel sheet.
func (h *ParamOnlyImportHandler) loadParamOnlyMaps(ctx context.Context) (*ImportMaps, error) {
	maps := NewImportMaps()

	params, err := h.cppRepo.ListAllParams(ctx)
	if err != nil {
		return nil, fmt.Errorf("load param map: %w", err)
	}
	for _, p := range params {
		maps.ParamMap[p.ParamCode] = p.ParamID
	}

	productLegacyIDs, err := h.cpmRepo.ListAllLegacyIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("load product legacy ID map: %w", err)
	}
	maps.ProductMap = productLegacyIDs

	return maps, nil
}

// preValidateParamSheets validates product_parameters and product_applicable_params
// without writing to the database.
func preValidateParamSheets(f *excelize.File, maps *ImportMaps) []SheetResult {
	s2 := preflightParamSheet(f, maps, nil, "product_parameters", []string{"legacy_oracle_sys_id", "param_code", "data_type"})
	s3 := preflightParamSheet(f, maps, nil, "product_applicable_params", []string{"legacy_oracle_sys_id", "param_code", "is_required"})

	// Cross-check legacy IDs against ProductMap from DB.
	s2 = crossCheckProductMap(s2, f, maps, "product_parameters", "legacy_oracle_sys_id")
	s3 = crossCheckProductMap(s3, f, maps, "product_applicable_params", "legacy_oracle_sys_id")

	return []SheetResult{s2, s3}
}

// crossCheckProductMap records unique product IDs that are not in ProductMap.
// Instead of adding per-row errors (which can be millions), it emits exactly
// one synthetic error per unique missing product ID using the missProductPrefix
// sentinel so GenerateErrorReport can surface them in a dedicated summary sheet.
func crossCheckProductMap(existing SheetResult, f *excelize.File, maps *ImportMaps, sheetName, idField string) SheetResult {
	rows, parseErr := ParseSheet(f, sheetName, []string{idField})
	if parseErr != nil {
		return existing
	}
	missing := make(map[string]int) // legacyID → affected row count
	for _, row := range rows {
		legacyID := row[idField]
		if legacyID == "" {
			continue
		}
		if _, ok := maps.ProductMap[legacyID]; !ok {
			missing[legacyID]++
		}
	}
	// Emit one synthetic error per unique missing product ID.
	for id, cnt := range missing {
		existing.Errors = append(existing.Errors,
			SheetError{0, idField, missProductPrefix + id + ":" + fmt.Sprintf("%d", cnt)},
		)
	}
	return existing
}

func (h *ParamOnlyImportHandler) uploadErrorReport(ctx context.Context, jobID int64, results []SheetResult) string {
	if countErrors(results) == 0 {
		return ""
	}
	reportBytes, genErr := GenerateErrorReport(results)
	if genErr != nil {
		h.logger.Error().Err(genErr).Int64("job_id", jobID).Msg("params-only import: generate error report failed")
		return ""
	}
	key := fmt.Sprintf("imports/bulk-params/%s/error-report.xlsx", strconv.FormatInt(jobID, 10))
	putErr := h.storage.PutObject(ctx, key, bytes.NewReader(reportBytes), int64(len(reportBytes)),
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if putErr != nil {
		h.logger.Error().Err(putErr).Str("key", key).Msg("params-only import: upload error report failed")
		return ""
	}
	return key
}

func (h *ParamOnlyImportHandler) updateJob(ctx context.Context, jobID int64, job *costimportjob.CostImportJob) {
	if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
		h.logger.Warn().Err(updateErr).Int64("job_id", jobID).Msg("params-only import: failed to persist job state")
	}
}
