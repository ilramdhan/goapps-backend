package job

import (
	"context"

	"github.com/google/uuid"
)

// ListFilter holds criteria for listing job executions.
type ListFilter struct {
	JobType  string
	Status   string
	Period   string
	Search   string
	Page     int
	PageSize int
}

// Repository defines the persistence contract for job executions.
type Repository interface {
	// Create persists a new job execution and assigns a sequential code.
	Create(ctx context.Context, exec *Execution) error

	// GetByID retrieves a job execution by its ID, including logs.
	GetByID(ctx context.Context, id uuid.UUID) (*Execution, error)

	// GetByCode retrieves a job execution by its code.
	GetByCode(ctx context.Context, code string) (*Execution, error)

	// List retrieves a paginated list of job executions.
	List(ctx context.Context, filter ListFilter) ([]*Execution, int64, error)

	// UpdateStatus atomically updates a job execution's status fields.
	UpdateStatus(ctx context.Context, exec *Execution) error

	// UpdateProgress atomically updates a job execution's progress.
	UpdateProgress(ctx context.Context, id uuid.UUID, progress int) error

	// AddLog persists a new log entry for a job execution.
	AddLog(ctx context.Context, log *ExecutionLog) error

	// UpdateLog updates an existing log entry.
	UpdateLog(ctx context.Context, log *ExecutionLog) error

	// HasActiveJob checks if an active job exists for the given type and period.
	HasActiveJob(ctx context.Context, jobType Type, period string) (bool, error)

	// GetNextSequence returns the next sequential number for job code generation.
	GetNextSequence(ctx context.Context, jobType Type, period string) (int, error)
}
