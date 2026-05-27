package costcalc

import (
	"time"

	sharedcc "github.com/mutugading/goapps-backend/pkg/costcalc"
)

// JobStatus enumerates calc job lifecycle states.
type JobStatus string

// Job status constants.
const (
	JobStatusQueued        JobStatus = "QUEUED"
	JobStatusPlanning      JobStatus = "PLANNING"
	JobStatusProcessing    JobStatus = "PROCESSING"
	JobStatusSuccess       JobStatus = "SUCCESS"
	JobStatusPartialFailed JobStatus = "PARTIAL_FAILED"
	JobStatusFailed        JobStatus = "FAILED"
	JobStatusCancelled     JobStatus = "CANCELLED"
)

// JobScope is the shared scope enum (alias preserved for ergonomic call sites).
type JobScope = sharedcc.JobScope

// CalculationType is the shared calc-type enum (alias preserved for ergonomic call sites).
type CalculationType = sharedcc.CalculationType

// Re-exported scope + calc-type constants — keep existing callers compiling without an import rewrite.
const (
	ScopeAll           = sharedcc.ScopeAll
	ScopeFiltered      = sharedcc.ScopeFiltered
	ScopeSingleProduct = sharedcc.ScopeSingleProduct
	ScopeSingleRoute   = sharedcc.ScopeSingleRoute

	CalcTypeActual   = sharedcc.CalcTypeActual
	CalcTypeForecast = sharedcc.CalcTypeForecast
	CalcTypeSelling  = sharedcc.CalcTypeSelling
)

// Job is the calc job aggregate root.
type Job struct {
	id              int64
	code            string
	period          string
	calcType        CalculationType
	scope           JobScope
	productFilter   []byte
	status          JobStatus
	priority        int
	totalProducts   int
	totalChunks     int
	totalWaves      int
	processedChunks int
	successCount    int
	failedCount     int
	blockedCount    int
	errorSummary    []byte
	triggeredBy     string
	queuedAt        time.Time
	startedAt       *time.Time
	completedAt     *time.Time
	durationMs      int64
	createdBy       string
}

// NewJob constructs a fresh QUEUED job.
func NewJob(period string, calcType CalculationType, scope JobScope, filter []byte, triggeredBy, createdBy string) (*Job, error) {
	if len(period) != 6 {
		return nil, ErrInvalidPeriod
	}
	return &Job{
		period:        period,
		calcType:      calcType,
		scope:         scope,
		productFilter: filter,
		status:        JobStatusQueued,
		priority:      5,
		triggeredBy:   triggeredBy,
		createdBy:     createdBy,
		queuedAt:      time.Now(),
	}, nil
}

// HydrateJob reconstructs a Job from persisted state (repository use only).
func HydrateJob(
	id int64, code, period string, calcType CalculationType, scope JobScope, filter []byte,
	status JobStatus, priority, total, totalChunks, totalWaves, processedChunks, succ, fail, blocked int,
	errSummary []byte, triggeredBy string,
	queuedAt time.Time, startedAt, completedAt *time.Time, durationMs int64, createdBy string,
) *Job {
	return &Job{
		id: id, code: code, period: period, calcType: calcType, scope: scope, productFilter: filter,
		status: status, priority: priority, totalProducts: total, totalChunks: totalChunks, totalWaves: totalWaves,
		processedChunks: processedChunks, successCount: succ, failedCount: fail, blockedCount: blocked,
		errorSummary: errSummary, triggeredBy: triggeredBy,
		queuedAt: queuedAt, startedAt: startedAt, completedAt: completedAt, durationMs: durationMs, createdBy: createdBy,
	}
}

// AssignID is used by the repository immediately after INSERT.
func (j *Job) AssignID(id int64, code string) { j.id = id; j.code = code }

// ID returns the surrogate job ID.
func (j *Job) ID() int64 { return j.id }

// Code returns the generated job code.
func (j *Job) Code() string { return j.code }

// Period returns the YYYYMM period.
func (j *Job) Period() string { return j.period }

// CalcType returns the calculation type.
func (j *Job) CalcType() CalculationType { return j.calcType }

// Scope returns the job scope.
func (j *Job) Scope() JobScope { return j.scope }

// ProductFilter returns the raw product filter payload.
func (j *Job) ProductFilter() []byte { return j.productFilter }

// Status returns the current lifecycle status.
func (j *Job) Status() JobStatus { return j.status }

// Priority returns the dispatch priority.
func (j *Job) Priority() int { return j.priority }

// TotalProducts returns the planned product count.
func (j *Job) TotalProducts() int { return j.totalProducts }

// TotalChunks returns the planned chunk count.
func (j *Job) TotalChunks() int { return j.totalChunks }

// TotalWaves returns the planned wave count.
func (j *Job) TotalWaves() int { return j.totalWaves }

// ProcessedChunks returns the number of completed chunks.
func (j *Job) ProcessedChunks() int { return j.processedChunks }

// SuccessCount returns the success product count.
func (j *Job) SuccessCount() int { return j.successCount }

// FailedCount returns the failed product count.
func (j *Job) FailedCount() int { return j.failedCount }

// BlockedCount returns the blocked product count.
func (j *Job) BlockedCount() int { return j.blockedCount }

// ErrorSummary returns the persisted error summary blob.
func (j *Job) ErrorSummary() []byte { return j.errorSummary }

// TriggeredBy returns the trigger source label.
func (j *Job) TriggeredBy() string { return j.triggeredBy }

// QueuedAt returns the queue timestamp.
func (j *Job) QueuedAt() time.Time { return j.queuedAt }

// StartedAt returns the processing-start timestamp (nil if not started).
func (j *Job) StartedAt() *time.Time { return j.startedAt }

// CompletedAt returns the completion timestamp (nil if not completed).
func (j *Job) CompletedAt() *time.Time { return j.completedAt }

// DurationMs returns the recorded duration in milliseconds.
func (j *Job) DurationMs() int64 { return j.durationMs }

// CreatedBy returns the user who created the job.
func (j *Job) CreatedBy() string { return j.createdBy }

// MarkPlanning transitions QUEUED -> PLANNING.
func (j *Job) MarkPlanning() error {
	if j.status != JobStatusQueued {
		return ErrJobInvalidStatus
	}
	j.status = JobStatusPlanning
	return nil
}

// MarkProcessing transitions PLANNING -> PROCESSING, recording the start time.
func (j *Job) MarkProcessing() error {
	if j.status != JobStatusPlanning && j.status != JobStatusQueued {
		return ErrJobInvalidStatus
	}
	j.status = JobStatusProcessing
	now := time.Now()
	j.startedAt = &now
	return nil
}

// SetTotals records DAG planning output. Caller usually calls this in PLANNING.
func (j *Job) SetTotals(products, chunks, waves int) {
	j.totalProducts = products
	j.totalChunks = chunks
	j.totalWaves = waves
}

// MarkComplete transitions PROCESSING -> SUCCESS / PARTIAL_FAILED / FAILED based on counts.
func (j *Job) MarkComplete(succ, fail, blocked int) error {
	if j.status != JobStatusProcessing {
		return ErrJobInvalidStatus
	}
	j.successCount = succ
	j.failedCount = fail
	j.blockedCount = blocked
	switch {
	case fail == 0 && blocked == 0:
		j.status = JobStatusSuccess
	case succ == 0:
		j.status = JobStatusFailed
	default:
		j.status = JobStatusPartialFailed
	}
	now := time.Now()
	j.completedAt = &now
	if j.startedAt != nil {
		j.durationMs = now.Sub(*j.startedAt).Milliseconds()
	}
	return nil
}

// Cancel transitions any active state -> CANCELLED.
func (j *Job) Cancel() error {
	switch j.status {
	case JobStatusQueued, JobStatusPlanning, JobStatusProcessing:
		j.status = JobStatusCancelled
		now := time.Now()
		j.completedAt = &now
		if j.startedAt != nil {
			j.durationMs = now.Sub(*j.startedAt).Milliseconds()
		}
		return nil
	case JobStatusSuccess, JobStatusPartialFailed, JobStatusFailed, JobStatusCancelled:
		return ErrJobInvalidStatus
	default:
		return ErrJobInvalidStatus
	}
}
