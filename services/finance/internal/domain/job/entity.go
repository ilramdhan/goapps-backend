package job

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Execution represents a job execution aggregate root.
type Execution struct {
	id            uuid.UUID
	code          Code
	jobType       Type
	subtype       string
	period        string
	status        Status
	priority      int
	params        json.RawMessage
	resultSummary json.RawMessage
	errorMessage  string
	progress      int
	retryCount    int
	maxRetries    int
	queuedAt      time.Time
	startedAt     *time.Time
	completedAt   *time.Time
	createdBy     string
	cancelledBy   string
	cancelledAt   *time.Time
	logs          []*ExecutionLog
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
) *Execution {
	return &Execution{
		id:            id,
		code:          code,
		jobType:       jobType,
		subtype:       subtype,
		period:        period,
		status:        status,
		priority:      priority,
		params:        params,
		resultSummary: resultSummary,
		errorMessage:  errorMessage,
		progress:      progress,
		retryCount:    retryCount,
		maxRetries:    maxRetries,
		queuedAt:      queuedAt,
		startedAt:     startedAt,
		completedAt:   completedAt,
		createdBy:     createdBy,
		cancelledBy:   cancelledBy,
		cancelledAt:   cancelledAt,
		logs:          logs,
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

// Complete transitions the job to success state.
func (e *Execution) Complete(resultSummary json.RawMessage) error {
	if e.status != StatusProcessing {
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

// Cancel transitions the job to cancelled state.
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

// CancelledBy returns who cancelled the job.
func (e *Execution) CancelledBy() string { return e.cancelledBy }

// CancelledAt returns the cancellation timestamp.
func (e *Execution) CancelledAt() *time.Time { return e.cancelledAt }

// Logs returns the execution logs.
func (e *Execution) Logs() []*ExecutionLog { return e.logs }

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
