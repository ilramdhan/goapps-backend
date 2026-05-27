package rmcost

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

// GetExportURLHandler resolves a presigned download URL for an export job.
//
// Lifecycle: load job → verify status COMPLETED → verify ownership → parse
// result_summary for file_path/file_name → presign via storage → return.
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
	if exec.JobType() != job.TypeRMCostExport {
		return nil, fmt.Errorf("job %s is not an rm_cost_export job", cmd.JobID)
	}
	if exec.Status() != job.StatusSuccess {
		return nil, fmt.Errorf("export not ready: status=%s", exec.Status())
	}

	// Ownership: prefer requesting_user_id from job.params (set canonically as
	// the user UUID at submit time). Fall back to created_by (which may be a
	// human-readable username — unreliable). This avoids username/uuid mismatch
	// when getUserFromContext returned the username on submit.
	owner := requestingUserFromParams(exec.Params())
	if owner == "" {
		owner = exec.CreatedBy()
	}
	if !ownsJob(owner, cmd.UserID) {
		return nil, fmt.Errorf("forbidden: caller does not own job %s", cmd.JobID)
	}

	var summary struct {
		FilePath string `json:"file_path"`
		FileName string `json:"file_name"`
	}
	if raw := exec.ResultSummary(); len(raw) > 0 {
		if jErr := json.Unmarshal(raw, &summary); jErr != nil {
			return nil, fmt.Errorf("parse result_summary: %w", jErr)
		}
	}
	if summary.FilePath == "" {
		return nil, fmt.Errorf("export artifact missing file_path")
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
