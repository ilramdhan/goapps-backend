// Package costcalc holds the cost-calculation domain: jobs, products,
// results, chunks, and formula evaluation contracts.
package costcalc

import "time"

// ChunkStatus enumerates chunk lifecycle states.
type ChunkStatus string

// Chunk status constants.
const (
	ChunkStatusQueued        ChunkStatus = "QUEUED"
	ChunkStatusDispatched    ChunkStatus = "DISPATCHED"
	ChunkStatusProcessing    ChunkStatus = "PROCESSING"
	ChunkStatusSuccess       ChunkStatus = "SUCCESS"
	ChunkStatusPartialFailed ChunkStatus = "PARTIAL_FAILED"
	ChunkStatusFailed        ChunkStatus = "FAILED"
)

// Chunk represents a batch of products processed together in a single transaction.
type Chunk struct {
	id           int64
	jobID        int64
	chunkNumber  int
	waveNo       int
	productIDs   []int64
	productCount int
	status       ChunkStatus
	workerID     string
	queuedAt     time.Time
	dispatchedAt *time.Time
	startedAt    *time.Time
	completedAt  *time.Time
	durationMs   int
	successCount int
	failedCount  int
	errorMessage string
	retryCount   int
	maxRetries   int
}

// NewChunk constructs a fresh QUEUED chunk.
func NewChunk(jobID int64, chunkNumber, waveNo int, productIDs []int64) *Chunk {
	return &Chunk{
		jobID:        jobID,
		chunkNumber:  chunkNumber,
		waveNo:       waveNo,
		productIDs:   productIDs,
		productCount: len(productIDs),
		status:       ChunkStatusQueued,
		queuedAt:     time.Now(),
		maxRetries:   3,
	}
}

// HydrateChunk reconstructs a Chunk from persisted state.
func HydrateChunk(id, jobID int64, chunkNumber, waveNo int, productIDs []int64, status ChunkStatus,
	workerID string, queuedAt time.Time, dispatchedAt, startedAt, completedAt *time.Time,
	durationMs, succ, fail int, errMsg string, retry, maxRetry int) *Chunk {
	return &Chunk{
		id: id, jobID: jobID, chunkNumber: chunkNumber, waveNo: waveNo,
		productIDs: productIDs, productCount: len(productIDs), status: status,
		workerID: workerID, queuedAt: queuedAt, dispatchedAt: dispatchedAt,
		startedAt: startedAt, completedAt: completedAt, durationMs: durationMs,
		successCount: succ, failedCount: fail, errorMessage: errMsg,
		retryCount: retry, maxRetries: maxRetry,
	}
}

// AssignID records the surrogate ID after INSERT.
func (c *Chunk) AssignID(id int64) { c.id = id }

// ID returns the surrogate chunk ID.
func (c *Chunk) ID() int64 { return c.id }

// JobID returns the parent job ID.
func (c *Chunk) JobID() int64 { return c.jobID }

// ChunkNumber returns the chunk sequence number within the job.
func (c *Chunk) ChunkNumber() int { return c.chunkNumber }

// WaveNo returns the wave the chunk belongs to.
func (c *Chunk) WaveNo() int { return c.waveNo }

// ProductIDs returns the product IDs packed into the chunk.
func (c *Chunk) ProductIDs() []int64 { return c.productIDs }

// ProductCount returns the number of products in the chunk.
func (c *Chunk) ProductCount() int { return c.productCount }

// Status returns the current chunk status.
func (c *Chunk) Status() ChunkStatus { return c.status }

// WorkerID returns the assigned worker identifier.
func (c *Chunk) WorkerID() string { return c.workerID }

// QueuedAt returns the queue timestamp.
func (c *Chunk) QueuedAt() time.Time { return c.queuedAt }

// DispatchedAt returns the dispatch timestamp (nil if not dispatched).
func (c *Chunk) DispatchedAt() *time.Time { return c.dispatchedAt }

// StartedAt returns the processing-start timestamp.
func (c *Chunk) StartedAt() *time.Time { return c.startedAt }

// CompletedAt returns the completion timestamp.
func (c *Chunk) CompletedAt() *time.Time { return c.completedAt }

// DurationMs returns the recorded duration in milliseconds.
func (c *Chunk) DurationMs() int { return c.durationMs }

// SuccessCount returns the number of products successfully processed.
func (c *Chunk) SuccessCount() int { return c.successCount }

// FailedCount returns the number of products that failed.
func (c *Chunk) FailedCount() int { return c.failedCount }

// ErrorMessage returns the recorded error message.
func (c *Chunk) ErrorMessage() string { return c.errorMessage }

// RetryCount returns the retry counter.
func (c *Chunk) RetryCount() int { return c.retryCount }

// MaxRetries returns the configured retry ceiling.
func (c *Chunk) MaxRetries() int { return c.maxRetries }

// Dispatch transitions QUEUED -> DISPATCHED.
func (c *Chunk) Dispatch() {
	c.status = ChunkStatusDispatched
	now := time.Now()
	c.dispatchedAt = &now
}

// Start transitions DISPATCHED -> PROCESSING with workerID recorded.
func (c *Chunk) Start(workerID string) {
	c.status = ChunkStatusProcessing
	c.workerID = workerID
	now := time.Now()
	c.startedAt = &now
}

// Complete records the final outcome.
func (c *Chunk) Complete(succ, fail int, errMsg string) {
	c.successCount = succ
	c.failedCount = fail
	c.errorMessage = errMsg
	switch {
	case fail == 0:
		c.status = ChunkStatusSuccess
	case succ == 0:
		c.status = ChunkStatusFailed
	default:
		c.status = ChunkStatusPartialFailed
	}
	now := time.Now()
	c.completedAt = &now
	if c.startedAt != nil {
		c.durationMs = int(now.Sub(*c.startedAt).Milliseconds())
	}
}

// IncRetry increments the retry counter and returns the new value.
func (c *Chunk) IncRetry() int { c.retryCount++; return c.retryCount }
