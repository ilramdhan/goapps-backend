// Package rmcost provides application layer handlers for RM landed-cost calculation jobs.
package rmcost

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/oraclesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	"time"
)

// TriggerReason identifies why the calculation was requested. Maps to
// rmcost.HistoryTriggerReason on the worker side via the job params JSON.
type TriggerReason string

// Trigger reasons — mirror rmcost.HistoryTriggerReason values.
const (
	TriggerOracleSyncChain TriggerReason = "oracle-sync-chain"
	TriggerGroupUpdate     TriggerReason = "group-update"
	TriggerDetailChange    TriggerReason = "detail-change"
	TriggerManualUI        TriggerReason = "manual-ui"
)

// IsValid reports whether the trigger reason is one of the recognized values.
func (r TriggerReason) IsValid() bool {
	switch r {
	case TriggerOracleSyncChain, TriggerGroupUpdate, TriggerDetailChange, TriggerManualUI:
		return true
	default:
		return false
	}
}

// TriggerCommand requests a landed-cost calculation job. When GroupHeadID is nil,
// the worker iterates every active group for the period.
type TriggerCommand struct {
	Period      string
	GroupHeadID *uuid.UUID
	Reason      TriggerReason
	CreatedBy   string
}

// TriggerResult holds the queued job handle.
type TriggerResult struct {
	Execution *job.Execution
}

// JobPublisher publishes RM cost calculation job messages to the queue.
type JobPublisher interface {
	PublishRMCostCalculation(ctx context.Context, jobID, period string, groupHeadID *uuid.UUID, reason, createdBy string) error
}

// TriggerHandler creates a job_execution row and publishes the job to RabbitMQ.
type TriggerHandler struct {
	jobRepo   job.Repository
	publisher JobPublisher
}

// NewTriggerHandler builds a TriggerHandler.
func NewTriggerHandler(jobRepo job.Repository, publisher JobPublisher) *TriggerHandler {
	return &TriggerHandler{jobRepo: jobRepo, publisher: publisher}
}

// Handle validates input, enforces period duplicate protection, persists the job
// record, and publishes to the queue. On publish failure the job is marked FAILED
// so operators see the error instead of a permanently-QUEUED row.
func (h *TriggerHandler) Handle(ctx context.Context, cmd TriggerCommand) (*TriggerResult, error) {
	if h.publisher == nil {
		return nil, fmt.Errorf("message queue unavailable: RabbitMQ not connected")
	}
	if cmd.CreatedBy == "" {
		return nil, rmcost.ErrEmptyCreatedBy
	}
	reason := cmd.Reason
	if reason == "" {
		reason = TriggerManualUI
	}
	if !reason.IsValid() {
		return nil, fmt.Errorf("invalid trigger reason %q", cmd.Reason)
	}

	period := cmd.Period
	if period == "" {
		period = oraclesync.ResolvePeriod(time.Now())
	}
	if err := rmcost.ValidatePeriod(period); err != nil {
		return nil, err
	}

	hasActive, err := h.jobRepo.HasActiveJob(ctx, job.TypeRMCostCalculation, period)
	if err != nil {
		return nil, fmt.Errorf("check active job: %w", err)
	}
	if hasActive {
		return nil, job.ErrDuplicateActiveJob
	}

	subtype := "all"
	if cmd.GroupHeadID != nil {
		subtype = cmd.GroupHeadID.String()
	}
	exec, err := job.NewExecution(job.TypeRMCostCalculation, subtype, period, cmd.CreatedBy, 5, nil)
	if err != nil {
		return nil, fmt.Errorf("create execution: %w", err)
	}
	if err := h.jobRepo.Create(ctx, exec); err != nil {
		return nil, fmt.Errorf("persist job: %w", err)
	}

	if err := h.publisher.PublishRMCostCalculation(ctx, exec.ID().String(), period, cmd.GroupHeadID, string(reason), cmd.CreatedBy); err != nil {
		if failErr := exec.Fail("failed to publish to queue: " + err.Error()); failErr != nil {
			return nil, fmt.Errorf("fail job after publish error: %w (publish: %w)", failErr, err)
		}
		if updateErr := h.jobRepo.UpdateStatus(ctx, exec); updateErr != nil {
			return nil, fmt.Errorf("update failed status: %w (publish: %w)", updateErr, err)
		}
		return nil, fmt.Errorf("publish job: %w", err)
	}

	return &TriggerResult{Execution: exec}, nil
}
