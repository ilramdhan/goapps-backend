package costbulkimport

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/lookupmaster"
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
	jobRepo          costimportjob.Repository
	cpmRepo          costproductmaster.Repository
	cppRepo          costproductparameter.Repository
	lookupMasterRepo lookupmaster.Repository
	storage          storage.Service
	logger           zerolog.Logger
}

// NewParamOnlyImportHandler constructs the handler.
func NewParamOnlyImportHandler(
	jobRepo costimportjob.Repository,
	cpmRepo costproductmaster.Repository,
	cppRepo costproductparameter.Repository,
	lookupMasterRepo lookupmaster.Repository,
	storageSvc storage.Service,
	logger zerolog.Logger,
) *ParamOnlyImportHandler {
	return &ParamOnlyImportHandler{
		jobRepo:          jobRepo,
		cpmRepo:          cpmRepo,
		cppRepo:          cppRepo,
		lookupMasterRepo: lookupMasterRepo,
		storage:          storageSvc,
		logger:           logger,
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

	// === Phase 1: pre-validation — param codes only (no product ID hard-check) ===
	// Product IDs not found in the DB are silently skipped during the write phase
	// and surfaced in a "missing_product_ids" sheet in the error report.
	preResults := preValidateParamSheetsNoProductCheck(f, maps)
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
	// Rows referencing product IDs not in ProductMap are skipped by processCPP/processCAP.
	ins2, upd2, errs2, s2Err := processCPP(ctx, f, maps, h.cppRepo, actor, now)
	if s2Err != nil {
		job.MarkFailed(s2Err.Error())
		h.updateJob(ctx, jobID, job)
		return s2Err
	}
	ins3, upd3, errs3, s3Err := processCAP(ctx, f, maps, h.cppRepo, actor, now)
	if s3Err != nil {
		job.MarkFailed(s3Err.Error())
		h.updateJob(ctx, jobID, job)
		return s3Err
	}

	// Count rows skipped due to missing product IDs before converting to sentinels.
	const notFoundMsg = "product not found in ProductMap: "
	skipped2 := countMsgPrefix(errs2, notFoundMsg)
	skipped3 := countMsgPrefix(errs3, notFoundMsg)
	totalSkipped := skipped2 + skipped3
	totalSuccess := ins2 + upd2 + ins3 + upd3
	totalProcessed := ins2 + upd2 + len(errs2) + ins3 + upd3 + len(errs3)
	totalFailed := len(errs2) - skipped2 + len(errs3) - skipped3 // real errors, not product-not-found
	job.UpdateProgress(totalProcessed, totalSuccess, totalFailed, totalSkipped)

	// Convert row-level "product not found" errors to compact sentinels and upload
	// an informational error report (missing_product_ids sheet).
	writeResults := []SheetResult{
		{SheetName: "product_parameters", TotalRows: ins2 + upd2 + len(errs2), Inserted: ins2, Updated: upd2, Errors: errs2},
		{SheetName: "product_applicable_params", TotalRows: ins3 + upd3 + len(errs3), Inserted: ins3, Updated: upd3, Errors: errs3},
	}
	writeResults = convertProductNotFoundToSentinels(writeResults)
	missingProducts := collectMissingProductIDs(writeResults)
	// Always generate report when there are real errors OR missing products,
	// so the caller can see which product IDs need to be imported first.
	if countErrors(writeResults) > 0 || len(missingProducts) > 0 {
		errorKey := h.uploadErrorReport(ctx, jobID, writeResults)
		if errorKey != "" {
			job.SetErrorFile(errorKey)
		}
		h.logger.Info().
			Int64("job_id", jobID).
			Int("skipped_products", len(missingProducts)).
			Int("skipped_rows", totalSkipped).
			Int("imported_rows", totalSuccess).
			Msg("params-only import: completed with some skipped rows")
	}

	job.MarkDone("")
	h.updateJob(ctx, jobID, job)
	return nil
}

// countMsgPrefix returns the number of errors whose Message starts with prefix.
func countMsgPrefix(errs []SheetError, prefix string) int {
	n := 0
	for _, e := range errs {
		if strings.HasPrefix(e.Message, prefix) {
			n++
		}
	}
	return n
}

// convertProductNotFoundToSentinels replaces individual "product not found in ProductMap: X"
// row errors with one aggregated miss_product sentinel per unique product ID.
// This keeps the error report compact (1 row per missing product, not 1 row per param row).
func convertProductNotFoundToSentinels(results []SheetResult) []SheetResult {
	const notFoundMsg = "product not found in ProductMap: "
	out := make([]SheetResult, len(results))
	for i, r := range results {
		counts := make(map[string]int)
		var kept []SheetError
		for _, e := range r.Errors {
			if id, ok := strings.CutPrefix(e.Message, notFoundMsg); ok {
				counts[id]++
			} else {
				kept = append(kept, e)
			}
		}
		for id, cnt := range counts {
			kept = append(kept, SheetError{0, "legacy_oracle_sys_id",
				missProductPrefix + id + ":" + strconv.Itoa(cnt)})
		}
		out[i] = r
		out[i].Errors = kept
	}
	return out
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

	// Load ParamLookupMap and MasterLookupValues for MASTER_LOOKUP validation.
	masterCodesToLoad := make(map[string]struct{})
	for _, p := range params {
		if p.ParamCategory == "MASTER_LOOKUP" && p.LookupMasterCode != "" {
			maps.ParamLookupMap[p.ParamCode] = p.LookupMasterCode
			masterCodesToLoad[p.LookupMasterCode] = struct{}{}
		}
	}
	for masterCode := range masterCodesToLoad {
		opts, optErr := h.lookupMasterRepo.ListMasterOptions(ctx, masterCode)
		if optErr != nil {
			h.logger.Warn().Err(optErr).Str("master_code", masterCode).Msg("loadParamOnlyMaps: cannot load master options")
			continue
		}
		optSet := make(map[string]bool, len(opts))
		for _, o := range opts {
			optSet[o.Value] = true
		}
		maps.MasterLookupValues[masterCode] = optSet
	}

	productLegacyIDs, err := h.cpmRepo.ListAllLegacyIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("load product legacy ID map: %w", err)
	}
	maps.ProductMap = productLegacyIDs

	return maps, nil
}

// preValidateParamSheetsNoProductCheck validates param codes only.
// Product IDs not found in ProductMap are NOT treated as hard errors — they are
// skipped during the write phase and reported in the "missing_product_ids" sheet.
func preValidateParamSheetsNoProductCheck(f *excelize.File, maps *ImportMaps) []SheetResult {
	s2 := preflightParamSheet(f, maps, nil, "product_parameters", []string{"legacy_oracle_sys_id", "param_code", "data_type"})
	s3 := preflightParamSheet(f, maps, nil, "product_applicable_params", []string{"legacy_oracle_sys_id", "param_code", "is_required"})
	return []SheetResult{s2, s3}
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
