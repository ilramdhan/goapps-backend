package rmcost

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
)

// ExecuteCommand is the worker-side input parsed from a queue message. It mirrors
// CalculateCommand but sources its JobID/Period/GroupHeadID from the message
// payload rather than an HTTP caller.
type ExecuteCommand struct {
	JobID         uuid.UUID
	Period        string
	GroupHeadID   *uuid.UUID
	TriggerReason rmcost.HistoryTriggerReason
	CalculatedBy  string
}

// ExecuteHandler drives the job lifecycle around CalculateHandler: it marks the
// job as processing, invokes the calculation, then persists the terminal status.
// Lives in the application layer so the worker entrypoint stays thin.
type ExecuteHandler struct {
	jobRepo   job.Repository
	calculate *CalculateHandler
	logger    zerolog.Logger
}

// NewExecuteHandler builds an ExecuteHandler.
func NewExecuteHandler(jobRepo job.Repository, calculate *CalculateHandler, logger zerolog.Logger) *ExecuteHandler {
	return &ExecuteHandler{jobRepo: jobRepo, calculate: calculate, logger: logger}
}

// Execute runs a full rm-cost calculation job by ID, updating job status along
// the way. Returns nil if the job completed (or was already terminal), else the
// underlying error so the consumer can nack to DLQ.
func (h *ExecuteHandler) Execute(ctx context.Context, cmd ExecuteCommand) error {
	exec, err := h.jobRepo.GetByID(ctx, cmd.JobID)
	if err != nil {
		return fmt.Errorf("get job %s: %w", cmd.JobID, err)
	}

	if exec.Status().IsTerminal() {
		h.logger.Info().
			Str("job_id", cmd.JobID.String()).
			Str("status", exec.Status().String()).
			Msg("Skipping rm cost job — already terminal")
		return nil
	}

	if err := exec.Start(); err != nil {
		return fmt.Errorf("start job %s: %w", cmd.JobID, err)
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		return fmt.Errorf("update status to processing: %w", err)
	}

	h.logger.Info().
		Str("job_id", cmd.JobID.String()).
		Str("period", cmd.Period).
		Msg("Starting RM cost calculation job")

	calcID := cmd.JobID
	calcCmd := CalculateCommand{
		Period:        cmd.Period,
		GroupHeadID:   cmd.GroupHeadID,
		JobID:         &calcID,
		TriggerReason: cmd.TriggerReason,
		CalculatedBy:  cmd.CalculatedBy,
	}

	result, calcErr := h.calculate.Handle(ctx, calcCmd)
	if calcErr != nil {
		return h.failJob(ctx, exec, calcErr)
	}

	return h.completeJob(ctx, exec, result)
}

func (h *ExecuteHandler) completeJob(ctx context.Context, exec *job.Execution, result *CalculateResult) error {
	summary, err := json.Marshal(map[string]any{
		"period":    result.Period,
		"processed": result.Processed,
		"skipped":   result.Skipped,
	})
	if err != nil {
		return fmt.Errorf("marshal rm cost summary: %w", err)
	}
	if err := exec.Complete(summary); err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		return fmt.Errorf("update status to success: %w", err)
	}
	h.logger.Info().
		Str("job_id", exec.ID().String()).
		Int("processed", result.Processed).
		Int("skipped", result.Skipped).
		Msg("RM cost calculation job completed")
	return nil
}

func (h *ExecuteHandler) failJob(ctx context.Context, exec *job.Execution, calcErr error) error {
	if failErr := exec.Fail(calcErr.Error()); failErr != nil {
		h.logger.Error().Err(failErr).Msg("Failed to transition rm cost job to failed state")
		return fmt.Errorf("fail job: %w (original: %w)", failErr, calcErr)
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Error().Err(err).Msg("Failed to persist rm cost job failure status")
		return fmt.Errorf("update status to failed: %w (original: %w)", err, calcErr)
	}
	h.logger.Error().Err(calcErr).
		Str("job_id", exec.ID().String()).
		Msg("RM cost calculation job failed")
	return calcErr
}
