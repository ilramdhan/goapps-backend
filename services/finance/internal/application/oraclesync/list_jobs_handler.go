package oraclesync

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// ListJobsQuery holds the input for listing jobs.
type ListJobsQuery struct {
	Page     int
	PageSize int
	JobType  string
	Status   string
	Period   string
	Search   string
}

// ListJobsResult holds the output of listing jobs.
type ListJobsResult struct {
	Executions []*job.Execution
	Total      int64
}

// ListJobsHandler retrieves a paginated list of job executions.
type ListJobsHandler struct {
	jobRepo job.Repository
}

// NewListJobsHandler creates a new ListJobsHandler.
func NewListJobsHandler(jobRepo job.Repository) *ListJobsHandler {
	return &ListJobsHandler{jobRepo: jobRepo}
}

// Handle retrieves a paginated list of job executions.
func (h *ListJobsHandler) Handle(ctx context.Context, query ListJobsQuery) (*ListJobsResult, error) {
	filter := job.ListFilter{
		JobType:  query.JobType,
		Status:   query.Status,
		Period:   query.Period,
		Search:   query.Search,
		Page:     query.Page,
		PageSize: query.PageSize,
	}

	execs, total, err := h.jobRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}

	return &ListJobsResult{
		Executions: execs,
		Total:      total,
	}, nil
}
