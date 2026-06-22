// Package costimportjob tracks the lifecycle of bulk import operations.
package costimportjob

import "time"

// Status constants for CostImportJob.
const (
	StatusPending = "PENDING"
	StatusRunning = "RUNNING"
	StatusDone    = "DONE"
	StatusFailed  = "FAILED"
	StatusPartial = "PARTIAL"
)

// Entity constants for CostImportJob.
const (
	EntityProductType              = "product_type"
	EntityParameter                = "parameter"
	EntityProductMaster            = "product_master"
	EntityCAPP                     = "capp"
	EntityCPP                      = "cpp"
	EntityBulkProductRouting       = "bulk_product_routing"
	EntityBulkProductRoutingExport = "bulk_product_routing_export"
)

// CostImportJob tracks the lifecycle of a bulk import operation.
type CostImportJob struct {
	jobID            int64
	entity           string
	status           string
	totalRows        int
	processed        int
	success          int
	failed           int
	skipped          int
	fileKey          string
	errorFile        string
	errorDetail      string
	createdAt        time.Time
	createdBy        string
	requestingUserID string
	startedAt        *time.Time
	completedAt      *time.Time
	parentJobID      *int64
}

// NewJob creates a new PENDING import job.
// requestingUserID is the UUID of the user who initiated the import; used for
// result notifications. Empty string is safe (notification will be skipped).
func NewJob(entity, fileKey, createdBy, requestingUserID string) *CostImportJob {
	return &CostImportJob{
		entity:           entity,
		status:           StatusPending,
		fileKey:          fileKey,
		createdAt:        time.Now().UTC(),
		createdBy:        createdBy,
		requestingUserID: requestingUserID,
	}
}

// Reconstruct rebuilds a job from persistence.
func Reconstruct(
	jobID int64, entity, status string,
	totalRows, processed, success, failed, skipped int,
	fileKey, errorFile, errorDetail string,
	createdAt time.Time, createdBy, requestingUserID string,
	startedAt, completedAt *time.Time,
	parentJobID *int64,
) *CostImportJob {
	return &CostImportJob{
		jobID: jobID, entity: entity, status: status,
		totalRows: totalRows, processed: processed,
		success: success, failed: failed, skipped: skipped,
		fileKey: fileKey, errorFile: errorFile, errorDetail: errorDetail,
		createdAt: createdAt, createdBy: createdBy,
		requestingUserID: requestingUserID,
		startedAt:        startedAt, completedAt: completedAt,
		parentJobID: parentJobID,
	}
}

// JobID returns the job ID.
func (j *CostImportJob) JobID() int64 { return j.jobID }

// Entity returns the entity type being imported.
func (j *CostImportJob) Entity() string { return j.entity }

// Status returns the current job status.
func (j *CostImportJob) Status() string { return j.status }

// TotalRows returns the total expected row count.
func (j *CostImportJob) TotalRows() int { return j.totalRows }

// Processed returns the number of rows processed so far.
func (j *CostImportJob) Processed() int { return j.processed }

// Success returns the number of successfully imported rows.
func (j *CostImportJob) Success() int { return j.success }

// Failed returns the number of failed rows.
func (j *CostImportJob) Failed() int { return j.failed }

// Skipped returns the number of skipped rows.
func (j *CostImportJob) Skipped() int { return j.skipped }

// FileKey returns the storage key for the uploaded import file.
func (j *CostImportJob) FileKey() string { return j.fileKey }

// ErrorFile returns the storage key for the generated error report file.
func (j *CostImportJob) ErrorFile() string { return j.errorFile }

// ErrorDetail returns a description of a fatal error, if any.
func (j *CostImportJob) ErrorDetail() string { return j.errorDetail }

// CreatedAt returns the time the job was created.
func (j *CostImportJob) CreatedAt() time.Time { return j.createdAt }

// CreatedBy returns the identifier of the user who created the job.
func (j *CostImportJob) CreatedBy() string { return j.createdBy }

// RequestingUserID returns the UUID of the user who initiated the import.
// Used to route completion notifications. May be empty for legacy jobs.
func (j *CostImportJob) RequestingUserID() string { return j.requestingUserID }

// StartedAt returns the time the job started processing, or nil if not yet started.
func (j *CostImportJob) StartedAt() *time.Time { return j.startedAt }

// CompletedAt returns the time the job completed, or nil if still in progress.
func (j *CostImportJob) CompletedAt() *time.Time { return j.completedAt }

// ParentJobID returns the parent job ID for child jobs, or nil for root jobs.
func (j *CostImportJob) ParentJobID() *int64 { return j.parentJobID }

// SetJobID is called by the repository after INSERT to assign the generated ID.
func (j *CostImportJob) SetJobID(id int64) { j.jobID = id }

// SetTotalRows sets the total expected row count.
func (j *CostImportJob) SetTotalRows(n int) { j.totalRows = n }

// MarkRunning transitions the job to RUNNING and records the start time.
func (j *CostImportJob) MarkRunning() {
	now := time.Now().UTC()
	j.status = StatusRunning
	j.startedAt = &now
}

// UpdateProgress records batch progress counters.
func (j *CostImportJob) UpdateProgress(processed, success, failed, skipped int) {
	j.processed = processed
	j.success = success
	j.failed = failed
	j.skipped = skipped
}

// MarkDone finalizes the job. If any rows failed the status is set to PARTIAL;
// otherwise it is set to DONE.
func (j *CostImportJob) MarkDone(errorFile string) {
	now := time.Now().UTC()
	j.completedAt = &now
	j.errorFile = errorFile
	if j.failed > 0 {
		j.status = StatusPartial
	} else {
		j.status = StatusDone
	}
}

// MarkFailed records a fatal error and transitions the job to FAILED.
func (j *CostImportJob) MarkFailed(detail string) {
	now := time.Now().UTC()
	j.status = StatusFailed
	j.completedAt = &now
	j.errorDetail = detail
}
