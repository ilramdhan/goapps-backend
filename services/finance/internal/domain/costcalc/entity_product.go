package costcalc

import "time"

// JobProductStatus enumerates per-product lifecycle inside a job.
type JobProductStatus string

// Job-product status constants.
const (
	JobProductStatusPending     JobProductStatus = "PENDING"
	JobProductStatusReady       JobProductStatus = "READY"
	JobProductStatusCalculating JobProductStatus = "CALCULATING"
	JobProductStatusSuccess     JobProductStatus = "SUCCESS"
	JobProductStatusFailed      JobProductStatus = "FAILED"
	JobProductStatusBlocked     JobProductStatus = "BLOCKED"
	JobProductStatusSkipped     JobProductStatus = "SKIPPED"
)

// JobProduct is the per-product execution record inside a calc job.
type JobProduct struct {
	id             int64
	jobID          int64
	chunkID        int64
	productSysID   int64
	routeHeadID    int64
	waveNo         int
	status         JobProductStatus
	blockReason    string
	startedAt      *time.Time
	completedAt    *time.Time
	durationMs     int
	costID         int64
	errorMessage   string
	calculationLog []byte
}

// NewJobProduct constructs a fresh PENDING row.
func NewJobProduct(jobID int64, productSysID, routeHeadID int64, waveNo int) *JobProduct {
	return &JobProduct{
		jobID: jobID, productSysID: productSysID, routeHeadID: routeHeadID, waveNo: waveNo,
		status: JobProductStatusPending,
	}
}

// HydrateJobProduct reconstructs from DB state.
func HydrateJobProduct(id, jobID, chunkID, productSysID, routeHeadID int64, waveNo int,
	status JobProductStatus, blockReason string, started, completed *time.Time, durMs int,
	costID int64, errMsg string, log []byte) *JobProduct {
	return &JobProduct{
		id: id, jobID: jobID, chunkID: chunkID, productSysID: productSysID, routeHeadID: routeHeadID,
		waveNo: waveNo, status: status, blockReason: blockReason,
		startedAt: started, completedAt: completed, durationMs: durMs,
		costID: costID, errorMessage: errMsg, calculationLog: log,
	}
}

// AssignID records the surrogate ID after INSERT.
func (p *JobProduct) AssignID(id int64) { p.id = id }

// AssignChunk records the assigned chunk after orchestrator packing.
func (p *JobProduct) AssignChunk(chunkID int64) { p.chunkID = chunkID }

// ID returns the surrogate ID.
func (p *JobProduct) ID() int64 { return p.id }

// JobID returns the parent job ID.
func (p *JobProduct) JobID() int64 { return p.jobID }

// ChunkID returns the assigned chunk ID.
func (p *JobProduct) ChunkID() int64 { return p.chunkID }

// ProductSysID returns the product surrogate key.
func (p *JobProduct) ProductSysID() int64 { return p.productSysID }

// RouteHeadID returns the costing route head ID used for this calc.
func (p *JobProduct) RouteHeadID() int64 { return p.routeHeadID }

// WaveNo returns the planning wave the product belongs to.
func (p *JobProduct) WaveNo() int { return p.waveNo }

// Status returns the current per-product status.
func (p *JobProduct) Status() JobProductStatus { return p.status }

// BlockReason returns the reason the product is blocked.
func (p *JobProduct) BlockReason() string { return p.blockReason }

// StartedAt returns the start timestamp.
func (p *JobProduct) StartedAt() *time.Time { return p.startedAt }

// CompletedAt returns the completion timestamp.
func (p *JobProduct) CompletedAt() *time.Time { return p.completedAt }

// DurationMs returns the recorded duration in milliseconds.
func (p *JobProduct) DurationMs() int { return p.durationMs }

// CostID returns the linked cost result ID, if any.
func (p *JobProduct) CostID() int64 { return p.costID }

// ErrorMessage returns the recorded error message.
func (p *JobProduct) ErrorMessage() string { return p.errorMessage }

// CalculationLog returns the JSON calculation trace blob.
func (p *JobProduct) CalculationLog() []byte { return p.calculationLog }

// MarkSuccess records a successful compute result.
func (p *JobProduct) MarkSuccess(costID int64, durMs int, log []byte) {
	p.status = JobProductStatusSuccess
	p.costID = costID
	p.durationMs = durMs
	p.calculationLog = log
	now := time.Now()
	p.completedAt = &now
}

// MarkFailed records a non-recoverable compute error.
func (p *JobProduct) MarkFailed(errMsg string, log []byte) {
	p.status = JobProductStatusFailed
	p.errorMessage = errMsg
	p.calculationLog = log
	now := time.Now()
	p.completedAt = &now
}

// MarkBlocked records a dependency / data-gap block (not retryable on its own).
func (p *JobProduct) MarkBlocked(reason string, log []byte) {
	p.status = JobProductStatusBlocked
	p.blockReason = reason
	p.calculationLog = log
	now := time.Now()
	p.completedAt = &now
}

// MarkSkipped records that the product was abandoned (e.g. job cancelled).
func (p *JobProduct) MarkSkipped() {
	p.status = JobProductStatusSkipped
	now := time.Now()
	p.completedAt = &now
}
