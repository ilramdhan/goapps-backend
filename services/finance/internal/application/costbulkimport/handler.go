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
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/lookupmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
)

// BulkImportHandler processes async bulk import of a 6-sheet Excel file
// containing product master and routing data from a legacy Oracle system.
type BulkImportHandler struct {
	jobRepo          costimportjob.Repository
	cpmRepo          costproductmaster.Repository
	cppRepo          costproductparameter.Repository
	routeRepo        costroute.Repository
	typeRepo         costproducttype.Repository
	rmGroupRepo      rmgroup.Repository
	lookupMasterRepo lookupmaster.Repository
	storage          storage.Service
	logger           zerolog.Logger
}

// NewBulkImportHandler creates a new BulkImportHandler.
func NewBulkImportHandler(
	jobRepo costimportjob.Repository,
	cpmRepo costproductmaster.Repository,
	cppRepo costproductparameter.Repository,
	routeRepo costroute.Repository,
	typeRepo costproducttype.Repository,
	rmGroupRepo rmgroup.Repository,
	lookupMasterRepo lookupmaster.Repository,
	storageSvc storage.Service,
	logger zerolog.Logger,
) *BulkImportHandler {
	return &BulkImportHandler{
		jobRepo:          jobRepo,
		cpmRepo:          cpmRepo,
		cppRepo:          cppRepo,
		routeRepo:        routeRepo,
		typeRepo:         typeRepo,
		rmGroupRepo:      rmGroupRepo,
		lookupMasterRepo: lookupMasterRepo,
		storage:          storageSvc,
		logger:           logger,
	}
}

// Handle processes a bulk import job. Called by finance-worker after dequeuing.
//
// Lifecycle: PENDING → RUNNING → DONE/FAILED.
//
// The handler runs in two phases:
//
//  1. Pre-validation — every sheet is parsed and validated without any DB
//     writes. Cross-sheet references (param codes, product IDs, route head
//     IDs) are checked using in-memory sets built from the file itself.
//     If any validation error is found the import is aborted entirely — no
//     partial rows are written.  An error report is uploaded to MinIO; the
//     "missing_param_codes" sheet in that report lists every unknown param
//     code with its row-skip count so the operator can resolve them all at
//     once before re-importing.
//
//  2. Write — only reached when pre-validation is fully clean.  All six
//     sheets are processed and written to the database.
func (h *BulkImportHandler) Handle(ctx context.Context, jobID int64, fileContent []byte, fileName string) error {
	h.logger.Info().Int64("job_id", jobID).Str("file_name", fileName).Msg("bulk import: starting")
	job, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load bulk import job %d: %w", jobID, err)
	}

	f, openErr := excelize.OpenReader(bytes.NewReader(fileContent))
	if openErr != nil {
		job.MarkFailed(openErr.Error())
		h.updateJob(ctx, jobID, job)
		return openErr
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			h.logger.Warn().Err(closeErr).Int64("job_id", jobID).Msg("bulk import: close excel failed")
		}
	}()

	actor := job.CreatedBy()
	now := time.Now()

	job.MarkRunning()
	h.updateJob(ctx, jobID, job)

	maps, mapsErr := h.loadMaps(ctx)
	if mapsErr != nil {
		job.MarkFailed(mapsErr.Error())
		h.updateJob(ctx, jobID, job)
		return mapsErr
	}

	// === Phase 1: pre-validation (no DB writes) ===
	preResults := preValidateAll(f, maps)
	if countErrors(preResults) > 0 {
		h.logger.Warn().
			Int64("job_id", jobID).
			Int("error_count", countErrors(preResults)).
			Msg("bulk import: pre-validation failed — aborting, no rows written")
		errorKey := h.maybeUploadErrorReport(ctx, jobID, preResults)
		job.MarkFailed(fmt.Sprintf(
			"validation failed: %d error(s) across %d sheet(s) — see error report",
			countErrors(preResults), len(preResults),
		))
		if errorKey != "" {
			job.SetErrorFile(errorKey)
		}
		h.updateJob(ctx, jobID, job)
		return nil
	}

	// === Phase 2: write (all rows validated) ===
	var allResults []SheetResult
	totalProcessed, totalSuccess, totalFailed, totalSkipped := 0, 0, 0, 0

	// Sheet 1: product_master.
	ins1, upd1, errs1, s1Err := processProductMaster(ctx, f, maps, h.cpmRepo, actor, now)
	allResults = append(allResults, SheetResult{
		SheetName: "product_master",
		TotalRows: ins1 + upd1 + len(errs1),
		Inserted:  ins1,
		Updated:   upd1,
		Errors:    errs1,
	})
	if s1Err != nil {
		job.MarkFailed(s1Err.Error())
		h.updateJob(ctx, jobID, job)
		return s1Err
	}
	totalProcessed += ins1 + upd1 + len(errs1)
	totalSuccess += ins1 + upd1
	totalFailed += len(errs1)
	job.UpdateProgress(totalProcessed, totalSuccess, totalFailed, totalSkipped)
	h.updateJob(ctx, jobID, job)

	// Sheet 2: product_parameters (CPP).
	ins2, upd2, errs2, s2Err := processCPP(ctx, f, maps, h.cppRepo, actor, now)
	allResults = append(allResults, SheetResult{
		SheetName: "product_parameters",
		TotalRows: ins2 + upd2 + len(errs2),
		Inserted:  ins2,
		Updated:   upd2,
		Errors:    errs2,
	})
	if s2Err != nil {
		h.rollbackWritePhase(ctx, jobID, maps)
		job.MarkFailed(s2Err.Error())
		h.updateJob(ctx, jobID, job)
		return s2Err
	}
	totalProcessed += ins2 + upd2 + len(errs2)
	totalSuccess += ins2 + upd2
	totalFailed += len(errs2)
	job.UpdateProgress(totalProcessed, totalSuccess, totalFailed, totalSkipped)
	h.updateJob(ctx, jobID, job)

	// Sheet 3: product_applicable_params (CAPP).
	ins3, upd3, errs3, s3Err := processCAP(ctx, f, maps, h.cppRepo, actor, now)
	allResults = append(allResults, SheetResult{
		SheetName: "product_applicable_params",
		TotalRows: ins3 + upd3 + len(errs3),
		Inserted:  ins3,
		Updated:   upd3,
		Errors:    errs3,
	})
	if s3Err != nil {
		h.rollbackWritePhase(ctx, jobID, maps)
		job.MarkFailed(s3Err.Error())
		h.updateJob(ctx, jobID, job)
		return s3Err
	}
	totalProcessed += ins3 + upd3 + len(errs3)
	totalSuccess += ins3 + upd3
	totalFailed += len(errs3)
	job.UpdateProgress(totalProcessed, totalSuccess, totalFailed, totalSkipped)
	h.updateJob(ctx, jobID, job)

	// Sheet 4: route_head.
	ins4, upd4, skip4, errs4, s4Err := processRouteHead(ctx, f, maps, h.routeRepo, actor, now)
	allResults = append(allResults, SheetResult{
		SheetName: "route_head",
		TotalRows: ins4 + upd4 + skip4 + len(errs4),
		Inserted:  ins4,
		Updated:   upd4,
		Skipped:   skip4,
		Errors:    errs4,
	})
	if s4Err != nil {
		h.rollbackWritePhase(ctx, jobID, maps)
		job.MarkFailed(s4Err.Error())
		h.updateJob(ctx, jobID, job)
		return s4Err
	}
	totalProcessed += ins4 + upd4 + skip4 + len(errs4)
	totalSuccess += ins4 + upd4
	totalFailed += len(errs4)
	totalSkipped += skip4
	job.UpdateProgress(totalProcessed, totalSuccess, totalFailed, totalSkipped)
	h.updateJob(ctx, jobID, job)

	// Sheet 5: route_sequences.
	ins5, upd5, errs5, s5Err := processRouteSeq(ctx, f, maps, h.routeRepo, actor, now)
	allResults = append(allResults, SheetResult{
		SheetName: "route_sequences",
		TotalRows: ins5 + upd5 + len(errs5),
		Inserted:  ins5,
		Updated:   upd5,
		Errors:    errs5,
	})
	if s5Err != nil {
		h.rollbackWritePhase(ctx, jobID, maps)
		job.MarkFailed(s5Err.Error())
		h.updateJob(ctx, jobID, job)
		return s5Err
	}
	totalProcessed += ins5 + upd5 + len(errs5)
	totalSuccess += ins5 + upd5
	totalFailed += len(errs5)
	job.UpdateProgress(totalProcessed, totalSuccess, totalFailed, totalSkipped)
	h.updateJob(ctx, jobID, job)

	// Sheet 6: route_rms.
	replaced6, errs6, s6Err := processRouteRM(ctx, f, maps, h.routeRepo, actor, now)
	allResults = append(allResults, SheetResult{
		SheetName: "route_rms",
		TotalRows: replaced6 + len(errs6),
		Inserted:  replaced6,
		Errors:    errs6,
	})
	if s6Err != nil {
		h.rollbackWritePhase(ctx, jobID, maps)
		job.MarkFailed(s6Err.Error())
		h.updateJob(ctx, jobID, job)
		return s6Err
	}
	totalProcessed += replaced6 + len(errs6)
	totalSuccess += replaced6
	totalFailed += len(errs6)
	job.UpdateProgress(totalProcessed, totalSuccess, totalFailed, totalSkipped)
	h.updateJob(ctx, jobID, job)

	// Generate error report if any rows failed or missing products.
	if countErrors(allResults) > 0 {
		if errorKey := h.maybeUploadErrorReport(ctx, jobID, allResults); errorKey != "" {
			job.SetErrorFile(errorKey)
		}
	}

	job.MarkDone("")
	h.updateJob(ctx, jobID, job)
	return nil
}

// loadMaps preloads ParamMap, ProductTypeMap, and RmGroupMap from the database.
func (h *BulkImportHandler) loadMaps(ctx context.Context) (*ImportMaps, error) {
	m := NewImportMaps()

	params, err := h.cppRepo.ListAllParams(ctx)
	if err != nil {
		return nil, fmt.Errorf("load param map: %w", err)
	}
	// Collect unique MASTER_LOOKUP codes to load options for.
	masterCodesToLoad := make(map[string]struct{})
	for _, p := range params {
		m.ParamMap[p.ParamCode] = p.ParamID
		if p.ParamCategory == "MASTER_LOOKUP" && p.LookupMasterCode != "" {
			m.ParamLookupMap[p.ParamCode] = p.LookupMasterCode
			masterCodesToLoad[p.LookupMasterCode] = struct{}{}
		}
	}
	// Load valid option codes for each referenced lookup master.
	for masterCode := range masterCodesToLoad {
		opts, optErr := h.lookupMasterRepo.ListMasterOptions(ctx, masterCode)
		if optErr != nil {
			h.logger.Warn().Err(optErr).Str("master_code", masterCode).Msg("loadMaps: failed to load master options — MASTER_LOOKUP values will not be validated")
			continue
		}
		optSet := make(map[string]bool, len(opts))
		for _, o := range opts {
			optSet[o.Value] = true
		}
		m.MasterLookupValues[masterCode] = optSet
	}

	types, err := h.typeRepo.ListAllActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("load product type map: %w", err)
	}
	for _, t := range types {
		m.ProductTypeMap[t.TypeCode()] = t.TypeID()
	}

	active := true
	groups, err := h.rmGroupRepo.ListAllHeads(ctx, &active)
	if err != nil {
		return nil, fmt.Errorf("load rm group map: %w", err)
	}
	for _, g := range groups {
		m.RmGroupMap[g.Code().String()] = true
	}

	return m, nil
}

// rollbackWritePhase deletes all products (and their cascaded/ordered routing data)
// that were newly inserted during Sheet 1. Called when any later write step fails.
func (h *BulkImportHandler) rollbackWritePhase(ctx context.Context, jobID int64, maps *ImportMaps) {
	if len(maps.InsertedProductSysIDs) == 0 {
		return
	}
	h.logger.Info().
		Int64("job_id", jobID).
		Int("count", len(maps.InsertedProductSysIDs)).
		Msg("bulk import: rolling back newly inserted products")
	if rbErr := h.cpmRepo.RollbackImport(ctx, maps.InsertedProductSysIDs); rbErr != nil {
		h.logger.Error().Err(rbErr).Int64("job_id", jobID).Msg("bulk import: rollback failed — DB may be partially written")
	}
}

// updateJob persists job state and logs on failure without propagating.
func (h *BulkImportHandler) updateJob(ctx context.Context, jobID int64, job *costimportjob.CostImportJob) {
	if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
		h.logger.Warn().Err(updateErr).Int64("job_id", jobID).Msg("bulk import: failed to persist job state")
	}
}

// maybeUploadErrorReport generates and uploads an error report when there are row errors.
// Returns the MinIO key on success, or empty string when no errors or on failure.
func (h *BulkImportHandler) maybeUploadErrorReport(ctx context.Context, jobID int64, results []SheetResult) string {
	if countErrors(results) == 0 {
		return ""
	}
	reportBytes, genErr := GenerateErrorReport(results)
	if genErr != nil {
		h.logger.Error().Err(genErr).Int64("job_id", jobID).Msg("bulk import: generate error report failed")
		return ""
	}
	key := fmt.Sprintf("imports/bulk-product-routing/%s/error-report.xlsx", strconv.FormatInt(jobID, 10))
	putErr := h.storage.PutObject(
		ctx, key,
		bytes.NewReader(reportBytes),
		int64(len(reportBytes)),
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	)
	if putErr != nil {
		h.logger.Error().Err(putErr).Int64("job_id", jobID).Str("key", key).Msg("bulk import: upload error report failed")
		return ""
	}
	h.logger.Info().Int64("job_id", jobID).Str("key", key).Msg("bulk import: error report uploaded")
	return key
}

// countErrors returns the total number of row errors across all sheet results.
func countErrors(results []SheetResult) int {
	total := 0
	for _, r := range results {
		total += len(r.Errors)
	}
	return total
}
