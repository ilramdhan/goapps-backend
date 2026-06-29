package costimportetl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/rs/zerolog"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costbulkimport"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
)

// Layer match tokens. Each token is matched (case-insensitive substring) against
// the uploaded sheet names (xlsx) or zip entry names (csv) to locate the source
// rows for the corresponding staging table.
const (
	tokenProductMaster    = "product_master"
	tokenProductParameter = "product_parameter"
	tokenApplicableParam  = "applicable_param"
	tokenRouteHead        = "route_head"
	tokenRouteSeq         = "route_seq"
	tokenRouteRM          = "route_rm"
)

// reportContentType is the MIME type of the generated Excel error report.
const reportContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// StagingPipeline is the full staging-table contract the ETL handler needs:
// streaming COPY into the UNLOGGED stg_import_* tables, set-based resolution into
// the real costing tables, and teardown of the staged rows afterwards. The
// concrete CostImportStagingRepository satisfies all three embedded interfaces.
type StagingPipeline interface {
	StagingRepository
	Resolver
	StagingMaintainer
	MasterLookupValidator
}

// Handler orchestrates the v2 ETL bulk-import pipeline: stream the uploaded file
// from object storage into staging, resolve every cross-reference set-based in
// SQL layer by layer, then emit an error report and clean up. It never loads a
// whole file into memory, so the worker's resident memory stays bounded.
type Handler struct {
	jobRepo costimportjob.Repository
	staging StagingPipeline
	storage storage.Service
	lookups MasterOptionsSource
	logger  zerolog.Logger
}

// NewHandler constructs an ETL import Handler. lookups may be nil, in which case
// MASTER_LOOKUP value validation is skipped (staged values are imported as-is).
func NewHandler(
	jobRepo costimportjob.Repository,
	staging StagingPipeline,
	storageSvc storage.Service,
	lookups MasterOptionsSource,
	logger zerolog.Logger,
) *Handler {
	return &Handler{
		jobRepo: jobRepo,
		staging: staging,
		storage: storageSvc,
		lookups: lookups,
		logger:  logger,
	}
}

// importLayer binds a logical sheet/entry token to its staging COPY function and
// its set-based resolve function. Layers are processed in declaration order.
type importLayer struct {
	token   string
	copy    func(ctx context.Context, jobID int64, produce RowProducer) (int64, error)
	resolve func(ctx context.Context, jobID int64, actor string) (int, error)
}

// Handle runs the full ETL pipeline for one import job. kind selects which
// staging layers participate (costimportjob.EntityBulkProductRouting streams all
// six layers; costimportjob.EntityBulkParamsOnly streams only the two parameter
// layers). The job transitions PENDING -> RUNNING -> DONE/PARTIAL/FAILED; rows
// with missing references are captured in stg_import_error and reported rather
// than aborting the job (status PARTIAL). Staging rows are always cleaned up.
func (h *Handler) Handle(ctx context.Context, jobID int64, kind string) error {
	h.logger.Info().Int64("job_id", jobID).Str("kind", kind).Msg("etl import: starting")

	job, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load import job %d: %w", jobID, err)
	}

	defer h.cleanup(ctx, jobID)

	job.MarkRunning()
	h.updateJob(ctx, jobID, job)

	rc, err := h.storage.GetObjectStream(ctx, job.FileKey())
	if err != nil {
		h.fail(ctx, jobID, job, fmt.Sprintf("fetch upload from storage: %v", err))
		return fmt.Errorf("fetch upload %q: %w", job.FileKey(), err)
	}
	defer h.closeReader(jobID, rc)

	src, err := h.openSource(rc, job.FileKey())
	if err != nil {
		h.fail(ctx, jobID, job, fmt.Sprintf("open upload container: %v", err))
		return fmt.Errorf("open source: %w", err)
	}
	defer h.closeSource(jobID, src)

	return h.run(ctx, jobID, kind, job, src)
}

// run executes the copy -> resolve -> report stages once the source is open.
func (h *Handler) run(
	ctx context.Context, jobID int64, kind string,
	job *costimportjob.CostImportJob, src rowSource,
) error {
	layers := layersForKind(h.staging, kind)

	staged, err := h.copyToStaging(ctx, jobID, layers, src)
	if err != nil {
		h.fail(ctx, jobID, job, fmt.Sprintf("copy to staging: %v", err))
		return err
	}
	job.SetTotalRows(clampToInt(staged))
	h.updateJob(ctx, jobID, job)

	if err := h.validateMasterLookups(ctx, jobID); err != nil {
		h.fail(ctx, jobID, job, fmt.Sprintf("validate master lookups: %v", err))
		return err
	}

	written, err := h.resolveLayers(ctx, jobID, job, layers)
	if err != nil {
		h.fail(ctx, jobID, job, fmt.Sprintf("resolve staging: %v", err))
		return err
	}

	return h.finalize(ctx, jobID, kind, job, written)
}

// copyToStaging streams every layer's rows into its staging table and returns the
// total number of data rows copied across all layers.
func (h *Handler) copyToStaging(
	ctx context.Context, jobID int64, layers []importLayer, src rowSource,
) (int64, error) {
	var total int64
	for _, layer := range layers {
		n, err := layer.copy(ctx, jobID, src.produce(layer.token))
		if err != nil {
			return total, fmt.Errorf("copy staging %s: %w", layer.token, err)
		}
		total += n
		h.logger.Info().Int64("job_id", jobID).Str("layer", layer.token).Int64("rows", n).Msg("etl import: staged")
	}
	return total, nil
}

// resolveLayers runs each layer's set-based resolution in order and returns the
// total number of target rows written (inserted or updated).
func (h *Handler) resolveLayers(
	ctx context.Context, jobID int64,
	job *costimportjob.CostImportJob, layers []importLayer,
) (int, error) {
	total := 0
	for _, layer := range layers {
		n, err := layer.resolve(ctx, jobID, job.CreatedBy())
		if err != nil {
			return total, fmt.Errorf("resolve %s: %w", layer.token, err)
		}
		total += n
		job.UpdateProgress(total, total, 0, 0)
		h.updateJob(ctx, jobID, job)
		h.logger.Info().Int64("job_id", jobID).Str("layer", layer.token).Int("written", n).Msg("etl import: resolved")
	}
	return total, nil
}

// finalize collects row-level errors, uploads an error report when any exist,
// records the final counts, and marks the job DONE (clean) or PARTIAL (errors).
func (h *Handler) finalize(
	ctx context.Context, jobID int64, kind string,
	job *costimportjob.CostImportJob, written int,
) error {
	stagingErrs, err := h.staging.CollectErrors(ctx, jobID)
	if err != nil {
		h.fail(ctx, jobID, job, fmt.Sprintf("collect staging errors: %v", err))
		return fmt.Errorf("collect errors: %w", err)
	}

	failed := len(stagingErrs)
	job.UpdateProgress(written+failed, written, failed, 0)

	if failed > 0 {
		if key := h.maybeUploadErrorReport(ctx, jobID, kind, stagingErrs); key != "" {
			job.SetErrorFile(key)
		}
	}

	job.MarkDone("")
	h.updateJob(ctx, jobID, job)
	h.logger.Info().
		Int64("job_id", jobID).
		Int("written", written).
		Int("errors", failed).
		Str("status", job.Status()).
		Msg("etl import: completed")
	return nil
}

// layersForKind returns the ordered layers participating for the given import
// kind. Params-only imports reference existing products, so only the parameter
// layers run; product+routing imports run all six layers in dependency order.
func layersForKind(s StagingPipeline, kind string) []importLayer {
	paramLayer := importLayer{tokenProductParameter, s.CopyStagingProductParameter, s.ResolveLayer2Params}
	applicableLayer := importLayer{tokenApplicableParam, s.CopyStagingApplicableParam, s.ResolveLayer3Applicable}

	if kind == costimportjob.EntityBulkParamsOnly {
		return []importLayer{paramLayer, applicableLayer}
	}
	return []importLayer{
		{tokenProductMaster, s.CopyStagingProductMaster, s.ResolveLayer1Products},
		paramLayer,
		applicableLayer,
		{tokenRouteHead, s.CopyStagingRouteHead, s.ResolveLayer4RouteHead},
		{tokenRouteSeq, s.CopyStagingRouteSeq, s.ResolveLayer5RouteSeq},
		{tokenRouteRM, s.CopyStagingRouteRM, s.ResolveLayer6RouteRM},
	}
}

// openSource prepares a uniform row source over the upload. The underlying
// container handles transport: a .zip (params bundle or routing csv bundle) is
// spooled to a temp file so its entries stream independently, while an .xlsx
// routing workbook is opened once and its sheets iterated via the Rows() iterator.
func (h *Handler) openSource(rc io.ReadCloser, fileName string) (rowSource, error) {
	c, err := openContainer(rc, fileName)
	if err != nil {
		return nil, fmt.Errorf("open upload container: %w", err)
	}
	if c.kind == containerXLSX {
		return newXLSXSource(c), nil
	}
	return &zipSource{container: c}, nil
}

// validateMasterLookups rejects staged MASTER_LOOKUP parameter values that do not
// exist in their registered master tables. The master table name is resolved per
// master code from the registry at runtime, so this is driven in Go (one option
// fetch per distinct master code) rather than as a static SQL JOIN. Rejected rows
// are recorded in stg_import_error and removed from staging so ResolveLayer2Params
// never imports them — matching the old params-only importer's behavior. It is a
// no-op when no MasterOptionsSource is configured or no MASTER_LOOKUP rows staged.
func (h *Handler) validateMasterLookups(ctx context.Context, jobID int64) error {
	if h.lookups == nil {
		return nil
	}
	candidates, err := h.staging.MasterLookupCandidates(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load master-lookup candidates: %w", err)
	}
	if len(candidates) == 0 {
		return nil
	}

	validByMaster := h.loadValidOptions(ctx, jobID, candidates)

	rejected := make([]MasterLookupCandidate, 0)
	for _, c := range candidates {
		valid, known := validByMaster[c.MasterCode]
		if !known {
			continue // options could not be loaded — skip rather than reject all
		}
		if _, ok := valid[c.Value]; !ok {
			rejected = append(rejected, c)
		}
	}
	if len(rejected) == 0 {
		return nil
	}

	removed, err := h.staging.RejectMasterLookupValues(ctx, jobID, rejected)
	if err != nil {
		return fmt.Errorf("reject master-lookup values: %w", err)
	}
	h.logger.Info().Int64("job_id", jobID).Int("distinct_rejected", len(rejected)).Int("rows_removed", removed).Msg("etl import: rejected unknown master-lookup values")
	return nil
}

// loadValidOptions fetches the valid value set for every distinct master code
// among the candidates. A master code whose options cannot be loaded is omitted
// from the result (its rows are then left untouched, never blanket-rejected).
func (h *Handler) loadValidOptions(ctx context.Context, jobID int64, candidates []MasterLookupCandidate) map[string]map[string]struct{} {
	validByMaster := make(map[string]map[string]struct{})
	for _, c := range candidates {
		if _, seen := validByMaster[c.MasterCode]; seen {
			continue
		}
		opts, optErr := h.lookups.ListMasterOptions(ctx, c.MasterCode)
		if optErr != nil {
			h.logger.Warn().Err(optErr).Int64("job_id", jobID).Str("master_code", c.MasterCode).Msg("etl import: cannot load master options; skipping validation for this master")
			continue
		}
		set := make(map[string]struct{}, len(opts))
		for _, o := range opts {
			set[o.Value] = struct{}{}
		}
		validByMaster[c.MasterCode] = set
	}
	return validByMaster
}

// cleanup tears down the staging rows for a finished job, logging on failure.
func (h *Handler) cleanup(ctx context.Context, jobID int64) {
	if err := h.staging.CleanupStaging(ctx, jobID); err != nil {
		h.logger.Warn().Err(err).Int64("job_id", jobID).Msg("etl import: cleanup staging failed")
	}
}

// fail records a fatal error on the job and persists it.
func (h *Handler) fail(ctx context.Context, jobID int64, job *costimportjob.CostImportJob, detail string) {
	job.MarkFailed(detail)
	h.updateJob(ctx, jobID, job)
}

// updateJob persists job state, logging (not propagating) on failure.
func (h *Handler) updateJob(ctx context.Context, jobID int64, job *costimportjob.CostImportJob) {
	if err := h.jobRepo.Update(ctx, job); err != nil {
		h.logger.Warn().Err(err).Int64("job_id", jobID).Msg("etl import: failed to persist job state")
	}
}

// closeReader closes the storage stream, logging on failure.
func (h *Handler) closeReader(jobID int64, rc io.ReadCloser) {
	if err := rc.Close(); err != nil {
		h.logger.Warn().Err(err).Int64("job_id", jobID).Msg("etl import: close upload stream failed")
	}
}

// closeSource releases the row source (temp file for zip, workbook for xlsx).
func (h *Handler) closeSource(jobID int64, src rowSource) {
	if err := src.Close(); err != nil {
		h.logger.Warn().Err(err).Int64("job_id", jobID).Msg("etl import: close source failed")
	}
}

// maybeUploadErrorReport groups the staged errors into per-sheet results, reuses
// the costbulkimport report generator, uploads it to storage, and returns the
// object key (empty on any failure).
func (h *Handler) maybeUploadErrorReport(
	ctx context.Context, jobID int64, kind string, stagingErrs []StagingError,
) string {
	results := groupErrors(stagingErrs)
	reportBytes, genErr := costbulkimport.GenerateErrorReport(results)
	if genErr != nil {
		h.logger.Error().Err(genErr).Int64("job_id", jobID).Msg("etl import: generate error report failed")
		return ""
	}
	key := fmt.Sprintf("imports/%s/%s/error-report.xlsx", kind, strconv.FormatInt(jobID, 10))
	putErr := h.storage.PutObject(ctx, key, bytes.NewReader(reportBytes), int64(len(reportBytes)), reportContentType)
	if putErr != nil {
		h.logger.Error().Err(putErr).Int64("job_id", jobID).Str("key", key).Msg("etl import: upload error report failed")
		return ""
	}
	h.logger.Info().Int64("job_id", jobID).Str("key", key).Msg("etl import: error report uploaded")
	return key
}

// groupErrors collapses the ordered (by sheet, then row) staged errors into one
// costbulkimport.SheetResult per sheet so the existing report generator can
// render per-sheet error tabs and the de-duplicated summary sheets.
func groupErrors(errs []StagingError) []costbulkimport.SheetResult {
	results := make([]costbulkimport.SheetResult, 0)
	for _, e := range errs {
		n := len(results)
		if n == 0 || results[n-1].SheetName != e.Sheet {
			results = append(results, costbulkimport.SheetResult{SheetName: e.Sheet})
			n = len(results)
		}
		results[n-1].Errors = append(results[n-1].Errors, costbulkimport.SheetError{
			RowNumber: e.RowNumber,
			Field:     e.KeyInfo,
			Message:   e.Message,
		})
	}
	return results
}

// clampToInt converts a non-negative staged row count to int, clamping to the
// int32 range used by the job's protobuf representation downstream.
func clampToInt(v int64) int {
	if v < 0 {
		return 0
	}
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(v) //nolint:gosec // bounds checked above
}
