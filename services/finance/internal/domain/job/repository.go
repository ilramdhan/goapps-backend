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
	// ExcludeChildren, when true, restricts the list to standalone jobs and
	// batch-tracking parents only (jex_parent_job_id IS NULL) — used by
	// "recent exports" style listings where a batch's individual children
	// should not each surface as their own history row.
	ExcludeChildren bool
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

	// CreateChildren persists N child job executions in one batch/transaction,
	// each referencing parentJobID. Every child is assigned its own sequential
	// code exactly like a standalone Create. Returns an error (and persists
	// nothing) if any child fails to insert.
	CreateChildren(ctx context.Context, execs []*Execution) error

	// IncrementChildProgress atomically increments a parent job's
	// completed-children or failed-children counter by 1 (success=true bumps
	// completed, success=false bumps failed) and reports whether the batch is
	// now fully done (completed+failed == total). Implemented as a single
	// UPDATE ... RETURNING (or equivalent) so concurrent children finishing at
	// the same time cannot race a read-then-write and double-count or miss the
	// completion transition.
	IncrementChildProgress(ctx context.Context, parentJobID uuid.UUID, success bool) (batchComplete bool, err error)

	// ListChildren returns every child job execution belonging to the given
	// parent job, ordered by creation time ascending. Returns an empty slice
	// (not an error) when the parent has no children or does not exist —
	// callers that need parent-existence semantics must check the parent
	// separately via GetByID.
	ListChildren(ctx context.Context, parentJobID uuid.UUID) ([]*Execution, error)
}
