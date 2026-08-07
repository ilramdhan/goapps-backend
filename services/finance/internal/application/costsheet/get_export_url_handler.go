package costsheet

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// PresignedURLProvider returns a short-lived download URL for a stored object.
// Implemented by the storage package; injected here as an interface for tests.
type PresignedURLProvider interface {
	PresignedGetURL(ctx context.Context, key string, validity time.Duration, downloadName string) (string, error)
}

// GetExportURLCommand identifies the job and the caller for ownership check.
type GetExportURLCommand struct {
	JobID  uuid.UUID
	UserID string
}

// GetExportURLResult carries the URL + suggested filename + expiry.
type GetExportURLResult struct {
	URL       string
	FileName  string
	ExpiresAt time.Time
}

// GetExportURLHandler resolves a presigned download URL for a product cost
// sheet export job.
//
// Lifecycle: load job → verify job type → verify status SUCCESS → verify
// ownership → parse result_summary for file_path/file_name → presign via
// storage → return.
type GetExportURLHandler struct {
	jobRepo  job.Repository
	storage  PresignedURLProvider
	validity time.Duration
}

// NewGetExportURLHandler constructs the handler. Pass validity=0 for default 5min.
func NewGetExportURLHandler(jobRepo job.Repository, storage PresignedURLProvider, validity time.Duration) *GetExportURLHandler {
	if validity <= 0 {
		validity = 5 * time.Minute
	}
	return &GetExportURLHandler{jobRepo: jobRepo, storage: storage, validity: validity}
}

// Handle returns the presigned download URL for the export artifact.
func (h *GetExportURLHandler) Handle(ctx context.Context, cmd GetExportURLCommand) (*GetExportURLResult, error) {
	if h.storage == nil {
		return nil, fmt.Errorf("storage unavailable")
	}
	exec, err := h.jobRepo.GetByID(ctx, cmd.JobID)
	if err != nil {
		return nil, fmt.Errorf("load job: %w", err)
	}
	if exec.JobType() != job.TypeProductCostSheetExport {
		return nil, fmt.Errorf("job %s is not a product_cost_sheet_export job", cmd.JobID)
	}
	if exec.Status() != job.StatusSuccess {
		return nil, fmt.Errorf("export not ready: status=%s", exec.Status())
	}

	if err := h.checkOwnership(exec, cmd.UserID); err != nil {
		return nil, err
	}

	summary, err := parseResultSummary(exec.ResultSummary())
	if err != nil {
		return nil, err
	}

	url, err := h.storage.PresignedGetURL(ctx, summary.FilePath, h.validity, summary.FileName)
	if err != nil {
		return nil, fmt.Errorf("presign url: %w", err)
	}
	return &GetExportURLResult{
		URL:       url,
		FileName:  summary.FileName,
		ExpiresAt: time.Now().UTC().Add(h.validity),
	}, nil
}

// checkOwnership verifies the caller may download this job's artifact.
// Ownership: prefer requesting_user_id from job.params (set canonically as
// the user UUID at submit time). Fall back to created_by (which may be a
// human-readable username — unreliable). This avoids username/uuid mismatch
// when getUserFromContext returned the username on submit.
func (h *GetExportURLHandler) checkOwnership(exec *job.Execution, userID string) error {
	owner := requestingUserFromParams(exec.Params())
	if owner == "" {
		owner = exec.CreatedBy()
	}
	if !ownsJob(owner, userID) {
		return fmt.Errorf("forbidden: caller does not own job %s", exec.ID())
	}
	return nil
}

// exportSummary is the parsed shape of job.Execution.ResultSummary() for a
// completed export job.
type exportSummary struct {
	FilePath string `json:"file_path"`
	FileName string `json:"file_name"`
}

// parseResultSummary decodes the job's result_summary JSON and validates the
// file_path is present.
func parseResultSummary(raw json.RawMessage) (exportSummary, error) {
	var summary exportSummary
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &summary); err != nil {
			return exportSummary{}, fmt.Errorf("parse result_summary: %w", err)
		}
	}
	if summary.FilePath == "" {
		return exportSummary{}, fmt.Errorf("export artifact missing file_path")
	}
	return summary, nil
}

// requestingUserFromParams returns the requesting_user_id field stored in
// job.params at submit time, or "" when the field is missing/malformed.
func requestingUserFromParams(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var p struct {
		RequestingUserID string `json:"requesting_user_id"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return ""
	}
	return p.RequestingUserID
}

// ownsJob compares the persisted created_by string with the authenticated
// user's UUID. The value is typically "user:<uuid>" (set by getUserFromContext)
// or a bare UUID — accept either.
func ownsJob(createdBy, userID string) bool {
	if createdBy == "" || userID == "" {
		return false
	}
	if createdBy == userID {
		return true
	}
	const prefix = "user:"
	if len(createdBy) > len(prefix) && createdBy[:len(prefix)] == prefix {
		return createdBy[len(prefix):] == userID
	}
	return false
}
