package oraclesync

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// GetJobQuery holds the input for retrieving a job.
type GetJobQuery struct {
	JobID string
}

// GetJobHandler retrieves a single job execution by ID.
type GetJobHandler struct {
	jobRepo job.Repository
}

// NewGetJobHandler creates a new GetJobHandler.
func NewGetJobHandler(jobRepo job.Repository) *GetJobHandler {
	return &GetJobHandler{jobRepo: jobRepo}
}

// Handle retrieves a job execution with its logs.
func (h *GetJobHandler) Handle(ctx context.Context, query GetJobQuery) (*job.Execution, error) {
	id, err := uuid.Parse(query.JobID)
	if err != nil {
		return nil, fmt.Errorf("parse job ID: %w", err)
	}

	exec, err := h.jobRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}

	return exec, nil
}
