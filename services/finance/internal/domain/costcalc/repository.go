package costcalc

import (
	"context"
	"time"
)

// JobFilter describes optional filters for ListJobs.
type JobFilter struct {
	Period      string
	CalcType    CalculationType
	Status      JobStatus
	TriggeredBy string
	Page        int
	PageSize    int
}

// JobProductFilter describes optional filters for ListJobProducts.
type JobProductFilter struct {
	Status   JobProductStatus
	Page     int
	PageSize int
}

// JobRepository persists Job aggregates.
type JobRepository interface {
	Create(ctx context.Context, j *Job) error
	GetByID(ctx context.Context, id int64) (*Job, error)
	List(ctx context.Context, f JobFilter) ([]*Job, int, error)
	UpdateStatus(ctx context.Context, id int64, status JobStatus) error
	UpdateTotals(ctx context.Context, id int64, total, totalChunks, totalWaves int) error
	UpdateProgress(ctx context.Context, id int64, processed, succ, fail, blocked int) error
	UpdateCompletion(ctx context.Context, id int64, status JobStatus, succ, fail, blocked int, durationMs int64, errSummary []byte) error
}

// ChunkRepository persists Chunk aggregates.
type ChunkRepository interface {
	Create(ctx context.Context, c *Chunk) error
	GetByID(ctx context.Context, id int64) (*Chunk, error)
	ListByJob(ctx context.Context, jobID int64, wave *int, status *ChunkStatus, page, pageSize int) ([]*Chunk, int, error)
	UpdateStatus(ctx context.Context, id int64, status ChunkStatus, workerID string) error
	UpdateResult(ctx context.Context, id int64, status ChunkStatus, succ, fail, durationMs int, errMsg string) error
	IncrementRetry(ctx context.Context, id int64) (int, error)
}

// JobProductRepository persists JobProduct aggregates.
type JobProductRepository interface {
	BulkCreate(ctx context.Context, items []*JobProduct) error
	GetByJobAndProduct(ctx context.Context, jobID, productSysID int64) (*JobProduct, error)
	ListByJob(ctx context.Context, jobID int64, f JobProductFilter) ([]*JobProduct, int, error)
	AssignChunk(ctx context.Context, jobID, productSysID, chunkID int64) error
	MarkSuccess(ctx context.Context, jobID, productSysID, costID int64, durationMs int, log []byte) error
	MarkFailed(ctx context.Context, jobID, productSysID int64, errMsg string, log []byte) error
	MarkBlocked(ctx context.Context, jobID, productSysID int64, reason string, log []byte) error
	MarkSkippedForJob(ctx context.Context, jobID int64) error
}

// ResultRepository persists Result aggregates.
type ResultRepository interface {
	// UpsertWithSupersede SUPERSEDEs the existing active row (if any) and inserts a new one
	// atomically. Returns (newCostID, prevVersion, prevTotal, prevCostID).
	UpsertWithSupersede(ctx context.Context, r *Result) (newCostID int64, prevVersion int, prevTotal float64, prevCostID int64, err error)
	GetActive(ctx context.Context, productSysID int64, period string, calcType CalculationType) (*Result, error)
	GetByID(ctx context.Context, id int64) (*Result, error)
	ListHistory(ctx context.Context, productSysID int64, calcType CalculationType, page, pageSize int) ([]*Result, int, error)
	// ListResults lists active cost results across products for a filter, with
	// product code/name resolved via join. Returns rows + total + resolved period.
	ListResults(ctx context.Context, f ResultListFilter) ([]*ResultSummary, int, string, error)
	MarkVerified(ctx context.Context, costID int64, by string) error
	MarkApproved(ctx context.Context, costID int64, by string) error
}

// ResultListFilter is the filter for ListResults. Empty Period means "latest
// period present in cst_product_cost"; empty CalcType/Status means no filter
// (Status additionally excludes SUPERSEDED when unset).
type ResultListFilter struct {
	Period   string
	CalcType CalculationType
	Status   string
	Search   string
	Page     int
	PageSize int
}

// ResultSummary is a flat, list-friendly projection of a cost result with the
// product code/name resolved (no UUIDs leak to the UI).
type ResultSummary struct {
	CostID       int64
	ProductSysID int64
	ProductCode  string
	ProductName  string
	Period       string
	CalcType     CalculationType
	RouteHeadID  int64
	Version      int
	CostPerUnit  float64
	TotalRMCost  float64
	TotalConv    float64
	TotalCost    float64
	UOMID        int
	CurrencyCode string
	Status       string
	JobID        int64
	CalculatedAt time.Time
	CalculatedBy string
}

// AuditHistoryRepository persists AuditHistoryEntry rows.
type AuditHistoryRepository interface {
	Write(ctx context.Context, e *AuditHistoryEntry) error
}
