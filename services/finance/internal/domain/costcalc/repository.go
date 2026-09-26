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
	// MarkQueuedAsSkipped marks all QUEUED (undispatched) chunks of a job as
	// SKIPPED, used when canceling a job mid-flight. This prevents the
	// orchestrator from dispatching waves for a cancelled job and leaves a
	// clear audit trail (the chunk row is SKIPPED, not vanished or FAILED).
	MarkQueuedAsSkipped(ctx context.Context, jobID int64) (affected int64, err error)
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
	// ListByProductIDsPeriodType returns the active result for each requested
	// product in one round-trip, keyed by product_sys_id. Products with no
	// result for the tuple are absent from the map (not an error) — a route
	// stage that has never been calculated is a legitimate state.
	ListByProductIDsPeriodType(ctx context.Context, productSysIDs []int64, period string, calcType CalculationType) (map[int64]*Result, error)
	MarkVerified(ctx context.Context, costID int64, by string) error
	MarkApproved(ctx context.Context, costID int64, by string) error
	// ListDistinctPeriods returns the distinct periods (YYYYMM) that have cost
	// results, ordered newest first.
	ListDistinctPeriods(ctx context.Context) ([]string, error)
}

// ResultListFilter is the filter for ListResults. Empty Period means "latest
// period present in cst_product_cost"; empty CalcType/Status means no filter
// (Status additionally excludes SUPERSEDED when unset).
type ResultListFilter struct {
	Period         string
	CalcType       CalculationType
	Status         string
	Search         string
	ProductTypeIDs []int32
	SortBy         string
	SortOrder      string
	Page           int
	PageSize       int
	ShadeCodes     []string
	RMGroupCodes   []string
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
	// ProductTypeID / ProductTypeCode come from the cost_product_master join;
	// zero/empty when the product row is missing.
	ProductTypeID   int32
	ProductTypeCode string
	// ItemCode / ItemName resolve cost_erp_item via the denormalized
	// cost_product_master.cpm_erp_item_code, falling back to the raw
	// cpm_erp_item_code when no cost_erp_item row matches.
	ItemCode string
	ItemName string
	// ShadeCode / ShadeName come from cost_product_master.
	ShadeCode string
	ShadeName string
	// PrimaryRMCode / PrimaryRMName / RMCount are derived from
	// cpc_rm_cost_detail: the RM entry with the highest contribution, and the
	// total entry count.
	//
	// Superseded by RMDetails, which carries every line (not just the top
	// contributor) — kept alongside it for existing consumers that only need
	// the top contributor, not marked as deprecated since this repository's
	// own code still populates it directly.
	PrimaryRMCode string
	PrimaryRMName string
	RMCount       int32
	// RMDetails is the full cpc_rm_cost_detail array, ordered by contribution
	// descending, with each line's ref_code/ref_name resolved to a display
	// name per its rm_type (GROUP -> cst_rm_group_head, ITEM -> cost_erp_item,
	// PRODUCT -> cost_product_master).
	RMDetails []RMDetailSummary
}

// RMDetailSummary is one resolved line of cpc_rm_cost_detail, ready for
// display (unlike the raw JSON, whose ref_code is an opaque
// "product:<sys_id>" string for PRODUCT-type lines).
type RMDetailSummary struct {
	RouteLevel   int32   `json:"route_level"`
	RMType       string  `json:"rm_type"`
	RefCode      string  `json:"ref_code"`
	RefName      string  `json:"ref_name"`
	ShadeCode    string  `json:"shade_code"`
	UnitCost     float64 `json:"unit_cost"`
	Ratio        float64 `json:"ratio"`
	Contribution float64 `json:"contribution"`
}

// AuditHistoryRepository persists AuditHistoryEntry rows.
type AuditHistoryRepository interface {
	Write(ctx context.Context, e *AuditHistoryEntry) error
}
