package orchestrator

// JobTriggeredEvent is published by the finance service to kick off
// non-SINGLE_PRODUCT calc jobs. The orchestrator picks it up, fetches the
// cal_job row, and drives planning + dispatch.
type JobTriggeredEvent struct {
	JobID int64 `json:"job_id"`
}

// ChunkCompletedEvent is published by the worker after processing one chunk.
type ChunkCompletedEvent struct {
	ChunkID      int64  `json:"chunk_id"`
	JobID        int64  `json:"job_id"`
	WaveNo       int    `json:"wave_no"`
	Status       string `json:"status"`
	SuccessCount int    `json:"success_count"`
	FailedCount  int    `json:"failed_count"`
	BlockedCount int    `json:"blocked_count"`
}
