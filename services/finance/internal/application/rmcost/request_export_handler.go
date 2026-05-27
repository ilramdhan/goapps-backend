package rmcost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// ExportJobPublisher abstracts the rabbitmq publisher dependency for testability.
type ExportJobPublisher interface {
	PublishRMCostExport(ctx context.Context, jobID, period, rmType, groupHeadID, search, requestingUserID, createdBy string) error
}

// RequestExportCommand carries the validated input for queueing an export.
type RequestExportCommand struct {
	Period           string
	RMType           string // "GROUP" / "ITEM" / "" (all)
	GroupHeadID      string // "" = no filter
	Search           string // "" = no filter
	RequestingUserID string // recipient for the EXPORT_READY notification
	CreatedBy        string // audit identity (typically "user:<uuid>" or username)
}

// RequestExportResult is the queue acknowledgement.
type RequestExportResult struct {
	Execution *job.Execution
}

// RequestExportHandler queues an asynchronous RM cost export job.
type RequestExportHandler struct {
	jobRepo   job.Repository
	publisher ExportJobPublisher
}

// NewRequestExportHandler constructs the handler.
func NewRequestExportHandler(jobRepo job.Repository, publisher ExportJobPublisher) *RequestExportHandler {
	return &RequestExportHandler{jobRepo: jobRepo, publisher: publisher}
}

// Handle creates a job_execution row and publishes the message to RabbitMQ.
func (h *RequestExportHandler) Handle(ctx context.Context, cmd RequestExportCommand) (*RequestExportResult, error) {
	if h.publisher == nil {
		return nil, fmt.Errorf("message queue unavailable: RabbitMQ not connected")
	}
	if cmd.Period == "" {
		return nil, fmt.Errorf("period is required")
	}
	if cmd.RequestingUserID == "" {
		return nil, fmt.Errorf("requesting user id is required")
	}
	if cmd.CreatedBy == "" {
		cmd.CreatedBy = cmd.RequestingUserID
	}

	// Persist params for traceability/debug.
	params := map[string]any{
		"period":             cmd.Period,
		"rm_type":            cmd.RMType,
		"group_head_id":      cmd.GroupHeadID,
		"search":             cmd.Search,
		"requesting_user_id": cmd.RequestingUserID,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("encode params: %w", err)
	}

	// Allow multiple concurrent export jobs for the same period — different
	// users may export with different filters. Skip the HasActiveJob check.
	exec, err := job.NewExecution(job.TypeRMCostExport, "xlsx", cmd.Period, cmd.CreatedBy, 5, paramsJSON)
	if err != nil {
		return nil, fmt.Errorf("create execution: %w", err)
	}
	if err := h.jobRepo.Create(ctx, exec); err != nil {
		return nil, fmt.Errorf("persist job: %w", err)
	}

	if err := h.publisher.PublishRMCostExport(
		ctx,
		exec.ID().String(),
		cmd.Period,
		cmd.RMType,
		cmd.GroupHeadID,
		cmd.Search,
		cmd.RequestingUserID,
		cmd.CreatedBy,
	); err != nil {
		// Mark job failed so it doesn't sit forever in QUEUED. Best-effort:
		// when the followup persist also fails we surface a joined error so
		// neither root cause is lost.
		if failErr := exec.Fail("failed to publish to queue: " + err.Error()); failErr == nil {
			if updErr := h.jobRepo.UpdateStatus(ctx, exec); updErr != nil {
				return nil, errors.Join(fmt.Errorf("publish job: %w", err), fmt.Errorf("persist failed: %w", updErr))
			}
		}
		return nil, fmt.Errorf("publish job: %w", err)
	}

	return &RequestExportResult{Execution: exec}, nil
}
