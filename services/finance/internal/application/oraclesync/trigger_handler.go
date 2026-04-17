package oraclesync

import (
	"context"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// TriggerCommand holds the input for triggering a sync job.
type TriggerCommand struct {
	Period    string
	CreatedBy string
}

// TriggerResult holds the output of triggering a sync job.
type TriggerResult struct {
	Execution *job.Execution
}

// TriggerHandler creates and enqueues a new sync job.
type TriggerHandler struct {
	jobRepo   job.Repository
	publisher JobPublisher
}

// JobPublisher publishes job messages to the message queue.
type JobPublisher interface {
	PublishOracleSync(ctx context.Context, jobID string, period string, createdBy string) error
}

// NewTriggerHandler creates a new TriggerHandler.
func NewTriggerHandler(jobRepo job.Repository, publisher JobPublisher) *TriggerHandler {
	return &TriggerHandler{
		jobRepo:   jobRepo,
		publisher: publisher,
	}
}

// Handle creates a job execution and publishes it to the queue.
func (h *TriggerHandler) Handle(ctx context.Context, cmd TriggerCommand) (*TriggerResult, error) {
	if h.publisher == nil {
		return nil, fmt.Errorf("message queue unavailable: RabbitMQ not connected")
	}

	// Resolve period if not provided.
	period := cmd.Period
	if period == "" {
		period = ResolvePeriod(time.Now())
	}

	// Check for duplicate active job.
	hasActive, err := h.jobRepo.HasActiveJob(ctx, job.TypeOracleSync, period)
	if err != nil {
		return nil, fmt.Errorf("check active job: %w", err)
	}
	if hasActive {
		return nil, job.ErrDuplicateActiveJob
	}

	// Create job execution.
	exec, err := job.NewExecution(job.TypeOracleSync, "item_cons_stk_po", period, cmd.CreatedBy, 5, nil)
	if err != nil {
		return nil, fmt.Errorf("create execution: %w", err)
	}

	if err := h.jobRepo.Create(ctx, exec); err != nil {
		return nil, fmt.Errorf("persist job: %w", err)
	}

	// Publish to RabbitMQ.
	if err := h.publisher.PublishOracleSync(ctx, exec.ID().String(), period, cmd.CreatedBy); err != nil {
		// Job is persisted but not published — mark as failed.
		if failErr := exec.Fail("failed to publish to queue: " + err.Error()); failErr != nil {
			return nil, fmt.Errorf("fail job after publish error: %w (publish: %v)", failErr, err)
		}
		if updateErr := h.jobRepo.UpdateStatus(ctx, exec); updateErr != nil {
			return nil, fmt.Errorf("update failed status: %w (publish: %v)", updateErr, err)
		}
		return nil, fmt.Errorf("publish job: %w", err)
	}

	return &TriggerResult{Execution: exec}, nil
}
