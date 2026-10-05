package erpintegration

import "context"

// BatchRepository persists cst_erp_int_batch (plan-04 P3-T2; design §4.2).
type BatchRepository interface {
	// Create inserts a new DRAFT batch, allocating ceib_seq = max(seq)+1 for
	// the period (serialized per period). It returns the stored batch with
	// id and seq set. A clash with uix_ceib_inflight / uix_ceib_active (a LIVE
	// batch is already in flight or active for the period) returns
	// ErrActiveBatchExists.
	Create(ctx context.Context, b *Batch) (*Batch, error)
	// GetByID returns the batch, or ErrBatchNotFound.
	GetByID(ctx context.Context, id int64) (*Batch, error)
	// ListByPeriod returns every batch of the period, newest seq first.
	ListByPeriod(ctx context.Context, period string) ([]*Batch, error)
	// FindActive returns the LIVE VALUATED/RECONCILED/LOCKED batch of the
	// period, or ErrBatchNotFound.
	FindActive(ctx context.Context, period string) (*Batch, error)
	// FindInFlight returns the LIVE DRAFT..PUSHED batch of the period, or
	// ErrBatchNotFound.
	FindInFlight(ctx context.Context, period string) (*Batch, error)
	// Save persists b with optimistic concurrency: the row is updated only
	// while its stored status still equals expected; otherwise
	// ErrStaleBatchStatus (or ErrBatchNotFound). The invalidation returned by
	// Batch.Transition is applied (batch-owned rows deleted) in the same
	// transaction.
	Save(ctx context.Context, b *Batch, expected BatchStatus, inv Invalidation) error
}

// DemandRepository persists cst_erp_adj_demand (replace-per-batch).
type DemandRepository interface {
	// Replace deletes the batch's demand rows and inserts lines, in one
	// transaction (re-run idempotency). It returns the number inserted.
	Replace(ctx context.Context, batchID int64, lines []DemandLine) (int64, error)
	// List returns the batch's demand rows ordered by the uk_ced key.
	List(ctx context.Context, batchID int64) ([]DemandLine, error)
	// Count returns the number of demand rows of the batch.
	Count(ctx context.Context, batchID int64) (int64, error)
}

// CoverageRepository persists cst_erp_coverage (replace-per-batch).
type CoverageRepository interface {
	// Replace deletes the batch's coverage rows and inserts lines, in one
	// transaction. It returns the number inserted.
	Replace(ctx context.Context, batchID int64, lines []CoverageLine) (int64, error)
	// List returns the batch's coverage rows ordered by (item, shade). An
	// empty statuses slice means every status.
	List(ctx context.Context, batchID int64, statuses ...CoverageStatus) ([]CoverageLine, error)
	// GetByID returns one row of the batch by cec_id, or ErrCoverageLineNotFound.
	GetByID(ctx context.Context, batchID, cecID int64) (CoverageLine, error)
	// Counts returns the per-status row count of the batch.
	Counts(ctx context.Context, batchID int64) (CoverageCounts, error)
}

// BatchStore is the transaction-scoped view of one batch used by a step. All
// calls share the transaction opened by BatchTxRunner.RunLocked, so the
// artifact replacement and the status update commit or roll back together.
type BatchStore interface {
	// GetForUpdate loads the batch row with SELECT … FOR UPDATE.
	GetForUpdate(ctx context.Context) (*Batch, error)
	// Save is BatchRepository.Save inside the transaction. Artifacts already
	// replaced in this transaction (ReplaceDemand / ReplaceCoverage) are not
	// deleted again by the invalidation, so call order does not matter.
	Save(ctx context.Context, b *Batch, expected BatchStatus, inv Invalidation) error
	// ReplaceDemand is DemandRepository.Replace inside the transaction.
	ReplaceDemand(ctx context.Context, lines []DemandLine) (int64, error)
	// ReplaceCoverage is CoverageRepository.Replace inside the transaction.
	ReplaceCoverage(ctx context.Context, lines []CoverageLine) (int64, error)
	// ListDemand is DemandRepository.List inside the transaction.
	ListDemand(ctx context.Context) ([]DemandLine, error)
}

// BatchTxRunner runs a step against one batch under the G11 single-runner
// guard (design §3.2): it opens a PG transaction, takes
// pg_try_advisory_xact_lock(hashtext('erp:'||batch_id)) and runs fn. If the
// lock is held elsewhere it returns ErrConcurrentRun without calling fn. The
// lock is released when the transaction ends; fn's error rolls it back.
type BatchTxRunner interface {
	RunLocked(ctx context.Context, batchID int64, fn func(ctx context.Context, s BatchStore) error) error
}
