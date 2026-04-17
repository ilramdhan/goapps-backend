package oraclesync

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// CancelJobCommand holds the input for cancelling a job.
type CancelJobCommand struct {
	JobID       string
	CancelledBy string
}

// CancelJobHandler cancels a queued or processing job.
type CancelJobHandler struct {
	jobRepo job.Repository
}

// NewCancelJobHandler creates a new CancelJobHandler.
func NewCancelJobHandler(jobRepo job.Repository) *CancelJobHandler {
	return &CancelJobHandler{jobRepo: jobRepo}
}

// Handle cancels a job execution.
func (h *CancelJobHandler) Handle(ctx context.Context, cmd CancelJobCommand) (*job.Execution, error) {
	id, err := uuid.Parse(cmd.JobID)
	if err != nil {
		return nil, fmt.Errorf("parse job ID: %w", err)
	}

	exec, err := h.jobRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}

	if err := exec.Cancel(cmd.CancelledBy); err != nil {
		return nil, err
	}

	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		return nil, fmt.Errorf("update cancel status: %w", err)
	}

	return exec, nil
}
