package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"

	"github.com/rs/zerolog"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costbulkimport"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costproductapplicableparam"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
)

// entityLabel returns a human-readable label for the given entity key.
func entityLabel(entity string) string {
	switch entity {
	case costimportjob.EntityProductMaster:
		return "Product Master"
	case costimportjob.EntityCAPP:
		return "Cost Applicable Parameters"
	case costimportjob.EntityCPP:
		return "Cost Product Parameters"
	case costimportjob.EntityBulkProductRouting:
		return "Bulk Import (Product Master + Routing)"
	case costimportjob.EntityBulkProductRoutingExport:
		return "Bulk Export (Product Master + Routing)"
	default:
		return entity
	}
}

// CostingImportHandler handles costing_import RabbitMQ messages.
// It fetches the import file from MinIO then dispatches to the appropriate
// entity-specific async import handler based on the job's entity field.
// On completion it emits a notification to the requesting user via IAM.
type CostingImportHandler struct {
	jobRepo           costimportjob.Repository
	storage           storage.Service
	cpmHandler        *costproductmaster.AsyncImportHandler
	cappHandler       *costproductapplicableparam.AsyncImportHandler
	cppHandler        *costproductparameter.AsyncImportHandler
	bulkImportHandler *costbulkimport.BulkImportHandler
	bulkExportHandler *costbulkimport.ExportHandler
	notif             iamclient.NotificationClient
	logger            zerolog.Logger
}

// NewCostingImportHandler constructs the handler.
// notif may be nil (a NopClient) — notification emission is always best-effort.
// bulkImportHandler and bulkExportHandler may be nil; the corresponding
// entity cases will return an error if they arrive and the handler is absent.
func NewCostingImportHandler(
	jobRepo costimportjob.Repository,
	storageSvc storage.Service,
	cpmHandler *costproductmaster.AsyncImportHandler,
	cappHandler *costproductapplicableparam.AsyncImportHandler,
	cppHandler *costproductparameter.AsyncImportHandler,
	bulkImportHandler *costbulkimport.BulkImportHandler,
	bulkExportHandler *costbulkimport.ExportHandler,
	notif iamclient.NotificationClient,
	logger zerolog.Logger,
) *CostingImportHandler {
	return &CostingImportHandler{
		jobRepo:           jobRepo,
		storage:           storageSvc,
		cpmHandler:        cpmHandler,
		cappHandler:       cappHandler,
		cppHandler:        cppHandler,
		bulkImportHandler: bulkImportHandler,
		bulkExportHandler: bulkExportHandler,
		notif:             notif,
		logger:            logger,
	}
}

// Handle is the entry point bound to the rabbitmq consumer in cmd/worker.
//
// Lifecycle: fetch file from MinIO → dispatch to entity handler →
// handler internally transitions job PENDING→RUNNING→DONE/PARTIAL/FAILED →
// emit completion notification to requestingUserID (best-effort).
func (h *CostingImportHandler) Handle(ctx context.Context, msg rabbitmq.JobMessage) error {
	jobID, err := strconv.ParseInt(msg.JobID, 10, 64)
	if err != nil {
		return fmt.Errorf("costing import: invalid job_id %q: %w", msg.JobID, err)
	}

	job, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("costing import: load job %d: %w", jobID, err)
	}

	// Prefer user ID from the job row (persisted at submission); fall back to
	// the message field for backward-compat with messages published before the
	// schema migration.
	requestingUserID := job.RequestingUserID()
	if requestingUserID == "" {
		requestingUserID = msg.RequestingUserID
	}

	// Export jobs store request parameters as JSON in FileKey — no file to fetch.
	// Dispatch them early before the MinIO fetch path.
	if job.Entity() == costimportjob.EntityBulkProductRoutingExport {
		return h.handleExport(ctx, jobID, job, requestingUserID)
	}

	fileContent, fileName, fetchErr := h.fetchFile(ctx, job.FileKey())
	if fetchErr != nil {
		h.logger.Error().Err(fetchErr).Int64("job_id", jobID).Str("file_key", job.FileKey()).Msg("costing import: fetch file failed")
		job.MarkFailed(fetchErr.Error())
		if updateErr := h.jobRepo.Update(ctx, job); updateErr != nil {
			h.logger.Error().Err(updateErr).Int64("job_id", jobID).Msg("costing import: persist FAILED after file fetch error")
		}
		h.emitNotification(ctx, jobID, job.Entity(), requestingUserID, job)
		// Return nil — message is ACKed; error is recorded on the job row.
		return nil
	}

	var dispatchErr error
	switch job.Entity() {
	case costimportjob.EntityProductMaster:
		dispatchErr = h.cpmHandler.Handle(ctx, jobID, fileContent, fileName, job.CreatedBy())
	case costimportjob.EntityCAPP:
		dispatchErr = h.cappHandler.Handle(ctx, jobID, fileContent, fileName)
	case costimportjob.EntityCPP:
		dispatchErr = h.cppHandler.Handle(ctx, jobID, fileContent, fileName)
	case costimportjob.EntityBulkProductRouting:
		if h.bulkImportHandler == nil {
			return fmt.Errorf("costing import: bulkImportHandler not configured for job %d", jobID)
		}
		dispatchErr = h.bulkImportHandler.Handle(ctx, jobID, fileContent, fileName)
	default:
		return fmt.Errorf("costing import: unknown entity %q for job %d", job.Entity(), jobID)
	}

	// Reload the job to get the final status set by the entity handler.
	finalJob, loadErr := h.jobRepo.GetByID(ctx, jobID)
	if loadErr != nil {
		h.logger.Warn().Err(loadErr).Int64("job_id", jobID).Msg("costing import: reload job for notification failed")
		return dispatchErr
	}
	h.emitNotification(ctx, jobID, job.Entity(), requestingUserID, finalJob)
	return dispatchErr
}

// emitNotification sends a best-effort notification to the requesting user.
// It swallows all errors — a notification failure must never block job completion.
func (h *CostingImportHandler) emitNotification(
	ctx context.Context,
	jobID int64,
	entity, requestingUserID string,
	job *costimportjob.CostImportJob,
) {
	if h.notif == nil || requestingUserID == "" {
		return
	}

	label := entityLabel(entity)
	jobIDStr := strconv.FormatInt(jobID, 10)

	var (
		title      string
		body       string
		notifType  iamv1.NotificationType
		severity   iamv1.NotificationSeverity
		actionType iamv1.NotificationActionType
	)

	switch job.Status() {
	case costimportjob.StatusDone:
		title = fmt.Sprintf("Import %s completed", label)
		body = fmt.Sprintf("%d rows imported successfully.", job.Success())
		notifType = iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM
		severity = iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_SUCCESS
		actionType = iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_ACKNOWLEDGE
	case costimportjob.StatusPartial:
		title = fmt.Sprintf("Import %s completed with errors", label)
		body = fmt.Sprintf("%d succeeded, %d failed, %d skipped.",
			job.Success(), job.Failed(), job.Skipped())
		notifType = iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM
		severity = iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_WARNING
		actionType = iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_ACKNOWLEDGE
	default: // FAILED or unexpected
		title = fmt.Sprintf("Import %s failed", label)
		body = job.ErrorDetail()
		if body == "" {
			body = "An error occurred while processing the import file."
		}
		notifType = iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM
		severity = iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_ERROR
		actionType = iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_NONE
	}

	if err := h.notif.Create(ctx, iamclient.CreateNotificationParams{
		RecipientUserID: requestingUserID,
		Type:            notifType,
		Severity:        severity,
		Title:           title,
		Body:            body,
		ActionType:      actionType,
		SourceType:      "finance.costing_import",
		SourceID:        jobIDStr,
	}); err != nil {
		h.logger.Warn().Err(err).
			Int64("job_id", jobID).
			Str("user_id", requestingUserID).
			Msg("costing import: send completion notification failed")
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

// handleExport dispatches a bulk export job. Export jobs store their request
// parameters as JSON in FileKey instead of a MinIO object path, so they bypass
// the normal fetchFile path.
func (h *CostingImportHandler) handleExport(
	ctx context.Context,
	jobID int64,
	job *costimportjob.CostImportJob,
	requestingUserID string,
) error {
	if h.bulkExportHandler == nil {
		return fmt.Errorf("costing import: bulkExportHandler not configured for job %d", jobID)
	}
	req, parseErr := unmarshalExportRequest(job.FileKey())
	if parseErr != nil {
		return fmt.Errorf("costing import: parse export request for job %d: %w", jobID, parseErr)
	}
	dispatchErr := h.bulkExportHandler.Handle(ctx, jobID, req)
	finalJob, loadErr := h.jobRepo.GetByID(ctx, jobID)
	if loadErr != nil {
		h.logger.Warn().Err(loadErr).Int64("job_id", jobID).Msg("costing import: reload export job for notification failed")
		return dispatchErr
	}
	h.emitNotification(ctx, jobID, job.Entity(), requestingUserID, finalJob)
	return dispatchErr
}

// unmarshalExportRequest decodes a JSON-encoded ExportRequest from the job's
// file_key field. Export jobs store their parameters there instead of a MinIO path.
func unmarshalExportRequest(fileKey string) (costbulkimport.ExportRequest, error) {
	var req costbulkimport.ExportRequest
	if err := json.Unmarshal([]byte(fileKey), &req); err != nil {
		return req, fmt.Errorf("unmarshal export request: %w", err)
	}
	return req, nil
}
