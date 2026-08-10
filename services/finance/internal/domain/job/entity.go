package job

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Execution represents a job execution aggregate root.
type Execution struct {
	id                uuid.UUID
	code              Code
	jobType           Type
	subtype           string
	period            string
	status            Status
	priority          int
	params            json.RawMessage
	resultSummary     json.RawMessage
	errorMessage      string
	progress          int
	retryCount        int
	maxRetries        int
	queuedAt          time.Time
	startedAt         *time.Time
	completedAt       *time.Time
	createdBy         string
	cancelledBy       string
	cancelledAt       *time.Time
	logs              []*ExecutionLog
	parentJobID       *uuid.UUID
	totalChildren     int
	completedChildren int
	failedChildren    int
}

// NewExecution creates a new job execution.
func NewExecution(
	jobType Type,
	subtype string,
	period string,
	createdBy string,
	priority int,
	params json.RawMessage,
) (*Execution, error) {
	if jobType == "" {
		return nil, ErrEmptyJobType
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	if priority < 1 || priority > 10 {
		return nil, ErrInvalidPriority
	}

	return &Execution{
		id:         uuid.New(),
		jobType:    jobType,
		subtype:    subtype,
		period:     period,
		status:     StatusQueued,
		priority:   priority,
		params:     params,
		progress:   0,
		retryCount: 0,
		maxRetries: 3,
		queuedAt:   time.Now(),
		createdBy:  createdBy,
	}, nil
}

// NewParentExecution creates a new batch-tracking parent job execution. A
// parent has no product list or file output of its own — it exists purely to
// aggregate child job progress and fire exactly one completion notification
// once every child has finished. totalChildren must be positive.
func NewParentExecution(
	jobType Type,
	subtype string,
	period string,
	createdBy string,
	priority int,
	params json.RawMessage,
	totalChildren int,
) (*Execution, error) {
	if totalChildren < 1 {
		return nil, ErrInvalidTotalChildren
	}
	exec, err := NewExecution(jobType, subtype, period, createdBy, priority, params)
	if err != nil {
		return nil, err
	}
	exec.totalChildren = totalChildren
	return exec, nil
}

// NewChildExecution creates a new job execution that belongs to a batch
// fan-out, referencing its parent's job ID. Otherwise behaves exactly like a
// standalone job — same status lifecycle, same worker handling.
func NewChildExecution(
	jobType Type,
	subtype string,
	period string,
	createdBy string,
	priority int,
	params json.RawMessage,
	parentJobID uuid.UUID,
) (*Execution, error) {
	exec, err := NewExecution(jobType, subtype, period, createdBy, priority, params)
	if err != nil {
		return nil, err
	}
	exec.parentJobID = &parentJobID
	return exec, nil
}

// Reconstitute rebuilds an Execution from persistence data.
func Reconstitute(
	id uuid.UUID,
	code Code,
	jobType Type,
	subtype string,
	period string,
	status Status,
	priority int,
	params json.RawMessage,
	resultSummary json.RawMessage,
	errorMessage string,
	progress int,
	retryCount int,
	maxRetries int,
	queuedAt time.Time,
	startedAt *time.Time,
	completedAt *time.Time,
	createdBy string,
	cancelledBy string,
	cancelledAt *time.Time,
	logs []*ExecutionLog,
	parentJobID *uuid.UUID,
	totalChildren int,
	completedChildren int,
	failedChildren int,
) *Execution {
	return &Execution{
		id:                id,
		code:              code,
		jobType:           jobType,
		subtype:           subtype,
		period:            period,
		status:            status,
		priority:          priority,
		params:            params,
		resultSummary:     resultSummary,
		errorMessage:      errorMessage,
		progress:          progress,
		retryCount:        retryCount,
		maxRetries:        maxRetries,
		queuedAt:          queuedAt,
		startedAt:         startedAt,
		completedAt:       completedAt,
		createdBy:         createdBy,
		cancelledBy:       cancelledBy,
		cancelledAt:       cancelledAt,
		logs:              logs,
		parentJobID:       parentJobID,
		totalChildren:     totalChildren,
		completedChildren: completedChildren,
		failedChildren:    failedChildren,
	}
}

// Start transitions the job to processing state.
func (e *Execution) Start() error {
	if e.status != StatusQueued {
		return ErrInvalidStatus
	}
	e.status = StatusProcessing
	now := time.Now()
	e.startedAt = &now
	return nil
}

// Complete transitions the job to success state. A standalone job must be
// StatusProcessing (it went through Start()). A batch parent job is
// tracking-only and never dispatched to a worker, so it never leaves
// StatusQueued until its last child's completion drives it straight to
// success — StatusQueued is accepted here for that case only.
func (e *Execution) Complete(resultSummary json.RawMessage) error {
	if e.status != StatusProcessing && (!e.IsParent() || e.status != StatusQueued) {
		return ErrInvalidStatus
	}
	e.status = StatusSuccess
	e.resultSummary = resultSummary
	e.progress = 100
	now := time.Now()
	e.completedAt = &now
	return nil
}

// Fail transitions the job to failed state.
func (e *Execution) Fail(errorMessage string) error {
	if e.status.IsTerminal() {
		return ErrAlreadyCompleted
	}
	e.retryCount++
	e.status = StatusFailed
	e.errorMessage = errorMessage
	now := time.Now()
	e.completedAt = &now
	return nil
}

// Cancel transitions the job to canceled state.
//
//nolint:misspell // cancelledBy field and CancelledBy getter match proto/DB convention
func (e *Execution) Cancel(cancelledBy string) error {
	if e.status == StatusCancelled {
		return ErrAlreadyCancelled
	}
	if !e.status.IsActive() {
		return ErrNotCancellable
	}
	e.status = StatusCancelled
	e.cancelledBy = cancelledBy
	now := time.Now()
	e.cancelledAt = &now
	e.completedAt = &now
	return nil
}

// UpdateProgress updates the job's progress percentage.
func (e *Execution) UpdateProgress(progress int) {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	e.progress = progress
}

// SetCode sets the job code (usually after generation sequence is determined).
func (e *Execution) SetCode(code Code) {
	e.code = code
}

// CanRetry returns true if the job has not exceeded its maximum retries.
func (e *Execution) CanRetry() bool {
	return e.retryCount < e.maxRetries
}

// Getters.

// ID returns the job ID.
func (e *Execution) ID() uuid.UUID { return e.id }

// Code returns the job code.
func (e *Execution) Code() Code { return e.code }

// JobType returns the job type.
func (e *Execution) JobType() Type { return e.jobType }

// Subtype returns the job subtype.
func (e *Execution) Subtype() string { return e.subtype }

// Period returns the period.
func (e *Execution) Period() string { return e.period }

// Status returns the current status.
func (e *Execution) Status() Status { return e.status }

// Priority returns the priority.
func (e *Execution) Priority() int { return e.priority }

// Params returns the job parameters.
func (e *Execution) Params() json.RawMessage { return e.params }

// ResultSummary returns the result summary.
func (e *Execution) ResultSummary() json.RawMessage { return e.resultSummary }

// ErrorMessage returns the error message.
func (e *Execution) ErrorMessage() string { return e.errorMessage }

// Progress returns the progress percentage.
func (e *Execution) Progress() int { return e.progress }

// RetryCount returns the retry count.
func (e *Execution) RetryCount() int { return e.retryCount }

// MaxRetries returns the maximum retries.
func (e *Execution) MaxRetries() int { return e.maxRetries }

// QueuedAt returns the queued timestamp.
func (e *Execution) QueuedAt() time.Time { return e.queuedAt }

// StartedAt returns the started timestamp.
func (e *Execution) StartedAt() *time.Time { return e.startedAt }

// CompletedAt returns the completed timestamp.
func (e *Execution) CompletedAt() *time.Time { return e.completedAt }

// CreatedBy returns who created the job.
func (e *Execution) CreatedBy() string { return e.createdBy }

// CancelledBy returns who canceled the job. //nolint:misspell // matches proto field name
func (e *Execution) CancelledBy() string { return e.cancelledBy } //nolint:misspell // matches proto field

// CancelledAt returns the cancellation timestamp. //nolint:misspell // matches proto field name
func (e *Execution) CancelledAt() *time.Time { return e.cancelledAt } //nolint:misspell // matches proto field

// Logs returns the execution logs.
func (e *Execution) Logs() []*ExecutionLog { return e.logs }

// ParentJobID returns the parent job's ID, or nil for a standalone job or a
// parent job itself.
func (e *Execution) ParentJobID() *uuid.UUID { return e.parentJobID }

// TotalChildren returns the number of child jobs this parent expects.
func (e *Execution) TotalChildren() int { return e.totalChildren }

// CompletedChildren returns the number of child jobs that finished SUCCESS.
func (e *Execution) CompletedChildren() int { return e.completedChildren }

// FailedChildren returns the number of child jobs that finished FAILED.
func (e *Execution) FailedChildren() int { return e.failedChildren }

// IsParent returns true if this job is a batch-tracking parent (expects one
// or more children and is not itself a child of another job).
func (e *Execution) IsParent() bool { return e.totalChildren > 0 && e.parentJobID == nil }

// IsChild returns true if this job belongs to a batch fan-out.
func (e *Execution) IsChild() bool { return e.parentJobID != nil }

// IsBatchComplete reports whether every expected child has finished, i.e. the
// parent's completed+failed counters have reached its total. Meaningless (and
// always false) for a non-parent job.
func (e *Execution) IsBatchComplete() bool {
	return e.totalChildren > 0 && e.completedChildren+e.failedChildren >= e.totalChildren
}

// IncrementCompletedChildren records one more successfully finished child
// in-memory. The authoritative increment happens via a single atomic
// UPDATE ... RETURNING in the repository (see Repository.IncrementChildProgress);
// this method exists so callers holding a reconstituted parent can reflect the
// same change without a second round-trip, and for unit tests.
func (e *Execution) IncrementCompletedChildren() {
	e.completedChildren++
}

// IncrementFailedChildren records one more failed child in-memory. See
// IncrementCompletedChildren for the atomicity note.
func (e *Execution) IncrementFailedChildren() {
	e.failedChildren++
}

// ExecutionLog represents a single step log entry within a job execution.
type ExecutionLog struct {
	id          uuid.UUID
	jobID       uuid.UUID
	step        string
	status      LogStatus
	message     string
	metadata    json.RawMessage
	startedAt   time.Time
	completedAt *time.Time
	durationMs  *int
}

// NewExecutionLog creates a new execution log entry.
func NewExecutionLog(jobID uuid.UUID, step string, status LogStatus, message string, metadata json.RawMessage) *ExecutionLog {
	return &ExecutionLog{
		id:        uuid.New(),
		jobID:     jobID,
		step:      step,
		status:    status,
		message:   message,
		metadata:  metadata,
		startedAt: time.Now(),
	}
}

// ReconstituteLog rebuilds an ExecutionLog from persistence data.
func ReconstituteLog(
	id uuid.UUID,
	jobID uuid.UUID,
	step string,
	status LogStatus,
	message string,
	metadata json.RawMessage,
	startedAt time.Time,
	completedAt *time.Time,
	durationMs *int,
) *ExecutionLog {
	return &ExecutionLog{
		id:          id,
		jobID:       jobID,
		step:        step,
		status:      status,
		message:     message,
		metadata:    metadata,
		startedAt:   startedAt,
		completedAt: completedAt,
		durationMs:  durationMs,
	}
}

// MarkCompleted completes the log entry and calculates duration.
func (l *ExecutionLog) MarkCompleted(status LogStatus, message string) {
	now := time.Now()
	l.completedAt = &now
	l.status = status
	if message != "" {
		l.message = message
	}
	durationMs := int(now.Sub(l.startedAt).Milliseconds())
	l.durationMs = &durationMs
}

// Getters.

// ID returns the log ID.
func (l *ExecutionLog) ID() uuid.UUID { return l.id }

// JobID returns the parent job ID.
func (l *ExecutionLog) JobID() uuid.UUID { return l.jobID }

// Step returns the step name.
func (l *ExecutionLog) Step() string { return l.step }

// Status returns the log status.
func (l *ExecutionLog) Status() LogStatus { return l.status }

// Message returns the log message.
func (l *ExecutionLog) Message() string { return l.message }

// Metadata returns the metadata.
func (l *ExecutionLog) Metadata() json.RawMessage { return l.metadata }

// StartedAt returns the start time.
func (l *ExecutionLog) StartedAt() time.Time { return l.startedAt }

// CompletedAt returns the completion time.
func (l *ExecutionLog) CompletedAt() *time.Time { return l.completedAt }

// DurationMs returns the duration in milliseconds.
func (l *ExecutionLog) DurationMs() *int { return l.durationMs }
