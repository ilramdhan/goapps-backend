package worker

// ChunkMessage is the JSON payload the orchestrator publishes to the
// finance.cost.chunk queue. Mirrors orchestrator's ChunkSpec exactly; kept in
// lockstep manually since orchestrator can't import this package.
type ChunkMessage struct {
	JobID           int64   `json:"job_id"`
	JobCode         string  `json:"job_code"`
	ChunkID         int64   `json:"chunk_id"`
	ChunkNumber     int     `json:"chunk_number"`
	WaveNo          int     `json:"wave_no"`
	Period          string  `json:"period"`
	CalculationType string  `json:"calculation_type"` // "ACTUAL" | "FORECAST" | "SELLING"
	ProductIDs      []int64 `json:"product_ids"`
	Actor           string  `json:"actor"`
}

// ChunkCompletedEvent is the JSON payload the worker publishes to the
// finance.cost.chunk.completed queue after running a chunk.
type ChunkCompletedEvent struct {
	ChunkID      int64  `json:"chunk_id"`
	JobID        int64  `json:"job_id"`
	WaveNo       int    `json:"wave_no"`
	Status       string `json:"status"` // SUCCESS | PARTIAL_FAILED | FAILED
	SuccessCount int    `json:"success_count"`
	FailedCount  int    `json:"failed_count"`
	BlockedCount int    `json:"blocked_count"`
}
