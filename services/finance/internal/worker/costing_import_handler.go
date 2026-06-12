package worker

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"

	"github.com/rs/zerolog"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costproductapplicableparam"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
)

// CostingImportHandler handles costing_import RabbitMQ messages.
// It fetches the import file from MinIO then dispatches to the appropriate
// entity-specific async import handler based on the job's entity field.
type CostingImportHandler struct {
	jobRepo     costimportjob.Repository
	storage     storage.Service
	cpmHandler  *costproductmaster.AsyncImportHandler
	cappHandler *costproductapplicableparam.AsyncImportHandler
	cppHandler  *costproductparameter.AsyncImportHandler
	logger      zerolog.Logger
}

// NewCostingImportHandler constructs the handler.
func NewCostingImportHandler(
	jobRepo costimportjob.Repository,
	storageSvc storage.Service,
	cpmHandler *costproductmaster.AsyncImportHandler,
	cappHandler *costproductapplicableparam.AsyncImportHandler,
	cppHandler *costproductparameter.AsyncImportHandler,
	logger zerolog.Logger,
) *CostingImportHandler {
	return &CostingImportHandler{
		jobRepo:     jobRepo,
		storage:     storageSvc,
		cpmHandler:  cpmHandler,
		cappHandler: cappHandler,
		cppHandler:  cppHandler,
		logger:      logger,
	}
}

// Handle is the entry point bound to the rabbitmq consumer in cmd/worker.
//
// Lifecycle: fetch file from MinIO → dispatch to entity handler →
// handler internally transitions job PENDING→RUNNING→DONE/PARTIAL/FAILED.
func (h *CostingImportHandler) Handle(ctx context.Context, msg rabbitmq.JobMessage) error {
	jobID, err := strconv.ParseInt(msg.JobID, 10, 64)
	if err != nil {
		return fmt.Errorf("costing import: invalid job_id %q: %w", msg.JobID, err)
	}

	job, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("costing import: load job %d: %w", jobID, err)
	}

	fileContent, fileName, fetchErr := h.fetchFile(ctx, job.FileKey())
	if fetchErr != nil {
		h.logger.Error().Err(fetchErr).Int64("job_id", jobID).Str("file_key", job.FileKey()).Msg("costing import: fetch file failed")
		job.MarkFailed(fetchErr.Error())
		if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
			h.logger.Error().Err(updateErr).Int64("job_id", jobID).Msg("costing import: persist FAILED after file fetch error")
		}
		// Return nil — message is ACKed; error is recorded on the job row.
		return nil
	}

	switch job.Entity() {
	case costimportjob.EntityProductMaster:
		return h.cpmHandler.Handle(ctx, jobID, fileContent, fileName, job.CreatedBy())
	case costimportjob.EntityCAPP:
		return h.cappHandler.Handle(ctx, jobID, fileContent, fileName)
	case costimportjob.EntityCPP:
		return h.cppHandler.Handle(ctx, jobID, fileContent, fileName)
	default:
		return fmt.Errorf("costing import: unknown entity %q for job %d", job.Entity(), jobID)
	}
}

// fetchFile downloads the import file from MinIO and returns its content and base name.
func (h *CostingImportHandler) fetchFile(ctx context.Context, fileKey string) ([]byte, string, error) {
	if h.storage == nil {
		return nil, "", fmt.Errorf("storage unavailable")
	}
	rc, _, err := h.storage.GetObject(ctx, fileKey)
	if err != nil {
		return nil, "", fmt.Errorf("get object: %w", err)
	}
	defer func() {
		if closeErr := rc.Close(); closeErr != nil {
			h.logger.Warn().Err(closeErr).Str("file_key", fileKey).Msg("costing import: close object")
		}
	}()
	content, err := io.ReadAll(rc)
	if err != nil {
		return nil, "", fmt.Errorf("read object: %w", err)
	}
	return content, filepath.Base(fileKey), nil
}
