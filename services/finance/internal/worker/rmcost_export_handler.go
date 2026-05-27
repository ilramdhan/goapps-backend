package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	rmcostdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
)

// ExportNotificationExpiry mirrors the MinIO bucket lifecycle for export
// artifacts (30 days). The notification's expires_at uses the same horizon so
// the entry disappears once the file is unreachable.
const ExportNotificationExpiry = 30 * 24 * time.Hour

// RMCostExportHandler renders an RM cost export to xlsx, uploads it to MinIO,
// emits a notification, and updates the job_execution row.
type RMCostExportHandler struct {
	jobRepo    job.Repository
	costRepo   rmcostdomain.Repository
	detailRepo rmcostdomain.CostDetailRepository
	storage    storage.Service
	notif      iamclient.NotificationClient
	logger     zerolog.Logger
}

// NewRMCostExportHandler constructs the handler.
func NewRMCostExportHandler(
	jobRepo job.Repository,
	costRepo rmcostdomain.Repository,
	detailRepo rmcostdomain.CostDetailRepository,
	storageSvc storage.Service,
	notif iamclient.NotificationClient,
	logger zerolog.Logger,
) *RMCostExportHandler {
	return &RMCostExportHandler{
		jobRepo:    jobRepo,
		costRepo:   costRepo,
		detailRepo: detailRepo,
		storage:    storageSvc,
		notif:      notif,
		logger:     logger,
	}
}

// Handle is the entry point bound to the rabbitmq consumer in cmd/worker.
//
// Lifecycle: PROCESSING → (success: COMPLETED + notif SUCCESS) | (failure: FAILED + notif ERROR).
func (h *RMCostExportHandler) Handle(ctx context.Context, msg rabbitmq.JobMessage) error {
	jobID, err := uuid.Parse(msg.JobID)
	if err != nil {
		return fmt.Errorf("invalid job id: %w", err)
	}

	exec, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load job: %w", err)
	}
	if err := exec.Start(); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("export: job state transition failed; continuing")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("export: persist PROCESSING failed")
	}

	result, runErr := h.runExport(ctx, msg)
	if runErr != nil {
		h.markFailedAndNotify(ctx, exec, msg, runErr)
		// Don't return the error — we've handled it via job_execution + notification.
		// Returning nil keeps the message OFF the dead-letter queue.
		return nil
	}

	// Success: persist + notify.
	summaryJSON, mErr := json.Marshal(result)
	if mErr != nil {
		// Fall back to empty summary — Complete() will still update status.
		h.logger.Warn().Err(mErr).Str("job_id", msg.JobID).Msg("export: marshal result_summary failed")
		summaryJSON = []byte("{}")
	}
	if err := exec.Complete(summaryJSON); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("export: complete state transition failed")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Msg("export: persist COMPLETED failed")
	}

	h.emitReadyNotification(ctx, msg, result)
	h.logger.Info().
		Str("job_id", msg.JobID).
		Str("file_path", result.FilePath).
		Int("header_rows", result.HeaderRows).
		Int("detail_rows", result.DetailRows).
		Msg("rm cost export completed")
	return nil
}

// runResult summarizes a successful export run. Persisted as job result_summary
// and embedded into the action_payload of the EXPORT_READY notification.
type runResult struct {
	FilePath   string `json:"file_path"`
	FileName   string `json:"file_name"`
	SizeBytes  int    `json:"size_bytes"`
	HeaderRows int    `json:"header_rows"`
	DetailRows int    `json:"detail_rows"`
	Period     string `json:"period"`
}

func (h *RMCostExportHandler) runExport(ctx context.Context, msg rabbitmq.JobMessage) (*runResult, error) {
	if h.storage == nil {
		return nil, fmt.Errorf("storage unavailable")
	}

	filter := rmcostdomain.ExportFilter{
		Period: msg.Period,
		RMType: rmcostdomain.RMType(msg.RMType),
		Search: msg.Search,
	}
	if msg.GroupHeadID != "" {
		gid, err := uuid.Parse(msg.GroupHeadID)
		if err != nil {
			return nil, fmt.Errorf("parse group_head_id: %w", err)
		}
		filter.GroupHeadID = &gid
	}

	headers, err := h.costRepo.ListAll(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list cost headers: %w", err)
	}

	// Fetch details for each header. Sequential is fine — typical period has
	// O(100) headers and details per header are <100 rows. If that scales up we
	// can parallelize with errgroup later.
	var details []*rmcostdomain.CostDetail
	for _, c := range headers {
		ds, dErr := h.detailRepo.ListByCostID(ctx, c.ID())
		if dErr != nil {
			return nil, fmt.Errorf("list details for cost %s: %w", c.ID(), dErr)
		}
		details = append(details, ds...)
	}

	xlsxBytes, err := BuildRMCostExcel(headers, details)
	if err != nil {
		return nil, fmt.Errorf("build excel: %w", err)
	}

	// MinIO key under exports/finance/rm-cost/{YYYY-MM}/{user_id}/{ts}-{shortJobID}.xlsx
	yyyymm := normalizePeriod(msg.Period)
	now := time.Now().UTC()
	ts := now.Format("20060102-150405")
	shortID := strings.SplitN(msg.JobID, "-", 2)[0]
	fileName := fmt.Sprintf("rm-cost-%s-%s.xlsx", msg.Period, ts)
	userIDDir := msg.RequestingUserID
	if userIDDir == "" {
		userIDDir = "unknown"
	}
	objectKey := fmt.Sprintf("exports/finance/rm-cost/%s/%s/%s-%s.xlsx", yyyymm, userIDDir, ts, shortID)

	if err := h.storage.PutObject(
		ctx, objectKey,
		bytes.NewReader(xlsxBytes), int64(len(xlsxBytes)),
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	); err != nil {
		return nil, fmt.Errorf("upload xlsx: %w", err)
	}

	return &runResult{
		FilePath:   objectKey,
		FileName:   fileName,
		SizeBytes:  len(xlsxBytes),
		HeaderRows: len(headers),
		DetailRows: len(details),
		Period:     msg.Period,
	}, nil
}

// markFailedAndNotify updates the job_execution row to FAILED and emits an
// ERROR notification to the requester. Best-effort: any internal error is
// logged but not propagated, so the rabbitmq message is still ACKed.
func (h *RMCostExportHandler) markFailedAndNotify(ctx context.Context, exec *job.Execution, msg rabbitmq.JobMessage, runErr error) {
	if err := exec.Fail(runErr.Error()); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("export: fail state transition")
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Error().Err(err).Str("job_id", msg.JobID).Msg("export: persist FAILED")
	}
	h.emitFailureNotification(ctx, msg, runErr)
	h.logger.Error().Err(runErr).Str("job_id", msg.JobID).Msg("rm cost export failed")
}

func (h *RMCostExportHandler) emitReadyNotification(ctx context.Context, msg rabbitmq.JobMessage, r *runResult) {
	if h.notif == nil || msg.RequestingUserID == "" {
		return
	}
	expiresAt := time.Now().UTC().Add(ExportNotificationExpiry).Format(time.RFC3339)
	payload, mErr := json.Marshal(map[string]any{
		"file_path":  r.FilePath,
		"file_name":  r.FileName,
		"size_bytes": r.SizeBytes,
		"expires_at": expiresAt,
	})
	if mErr != nil {
		h.logger.Warn().Err(mErr).Str("job_id", msg.JobID).Msg("export: marshal action_payload failed")
		payload = []byte("{}")
	}
	body := fmt.Sprintf("Period %s • %d header rows • %d detail rows • %d KB",
		r.Period, r.HeaderRows, r.DetailRows, r.SizeBytes/1024)
	if err := h.notif.Create(ctx, iamclient.CreateNotificationParams{
		RecipientUserID: msg.RequestingUserID,
		Type:            iamv1.NotificationType_NOTIFICATION_TYPE_EXPORT_READY,
		Severity:        iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_SUCCESS,
		Title:           "Export RM Cost selesai",
		Body:            body,
		ActionType:      iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_DOWNLOAD,
		ActionPayload:   string(payload),
		SourceType:      "finance.rm_cost_export",
		SourceID:        msg.JobID,
		ExpiresAt:       expiresAt,
	}); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("export: create EXPORT_READY notification failed")
	}
}

func (h *RMCostExportHandler) emitFailureNotification(ctx context.Context, msg rabbitmq.JobMessage, runErr error) {
	if h.notif == nil || msg.RequestingUserID == "" {
		return
	}
	body := truncate(runErr.Error(), 500)
	if err := h.notif.Create(ctx, iamclient.CreateNotificationParams{
		RecipientUserID: msg.RequestingUserID,
		Type:            iamv1.NotificationType_NOTIFICATION_TYPE_EXPORT_READY,
		Severity:        iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_ERROR,
		Title:           "Export RM Cost gagal",
		Body:            body,
		ActionType:      iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_NONE,
		ActionPayload:   "",
		SourceType:      "finance.rm_cost_export",
		SourceID:        msg.JobID,
	}); err != nil {
		h.logger.Warn().Err(err).Str("job_id", msg.JobID).Msg("export: create failure notification failed")
	}
}

// normalizePeriod converts YYYYMM to YYYY-MM for human-readable dir paths in MinIO.
func normalizePeriod(p string) string {
	if len(p) != 6 {
		return p
	}
	return p[:4] + "-" + p[4:]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
