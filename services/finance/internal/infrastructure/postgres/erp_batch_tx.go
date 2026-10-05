package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// ErpBatchTxRunner implements erpintegration.BatchTxRunner: one PG
// transaction per step, guarded by the G11 single-runner advisory lock
// (design §3.2; plan-04 P3-T2 step 4).
type ErpBatchTxRunner struct{ db *DB }

// NewErpBatchTxRunner constructs the runner.
func NewErpBatchTxRunner(db *DB) *ErpBatchTxRunner {
	return &ErpBatchTxRunner{db: db}
}

var _ erpintegration.BatchTxRunner = (*ErpBatchTxRunner)(nil)

// erpBatchAdvisoryLockSQL is the G11 lock. The key text is exactly
// 'erp:' || batch_id (design §3.2); it is released at transaction end.
const erpBatchAdvisoryLockSQL = `SELECT pg_try_advisory_xact_lock(hashtext('erp:' || $1::bigint::text))`

// TryLockErpBatch takes the G11 advisory lock inside tx. It returns false
// when another session holds it.
func TryLockErpBatch(ctx context.Context, tx *sql.Tx, batchID int64) (bool, error) {
	var got bool
	if err := tx.QueryRowContext(ctx, erpBatchAdvisoryLockSQL, batchID).Scan(&got); err != nil {
		return false, fmt.Errorf("erp batch advisory lock: %w", err)
	}
	return got, nil
}

// RunLocked opens a transaction, takes the G11 lock for batchID and runs fn
// with a transaction-scoped store. ErrConcurrentRun when the lock is held.
func (r *ErpBatchTxRunner) RunLocked(ctx context.Context, batchID int64, fn func(ctx context.Context, s erpintegration.BatchStore) error) error {
	if batchID <= 0 {
		return fmt.Errorf("erp batch run: %w: id %d", erpintegration.ErrBatchNotFound, batchID)
	}
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		got, err := TryLockErpBatch(ctx, tx, batchID)
		if err != nil {
			return err
		}
		if !got {
			return fmt.Errorf("%w: batch %d", erpintegration.ErrConcurrentRun, batchID)
		}
		return fn(ctx, &erpBatchStore{tx: tx, batchID: batchID})
	})
}

// erpBatchStore is the transaction-scoped erpintegration.BatchStore.
type erpBatchStore struct {
	tx               *sql.Tx
	batchID          int64
	replacedDemand   bool
	replacedCoverage bool
	replacedStd      bool
}

var _ erpintegration.BatchStore = (*erpBatchStore)(nil)

func (s *erpBatchStore) GetForUpdate(ctx context.Context) (*erpintegration.Batch, error) {
	return getBatch(ctx, s.tx, s.batchID, true)
}

func (s *erpBatchStore) Save(ctx context.Context, b *erpintegration.Batch, expected erpintegration.BatchStatus, inv erpintegration.Invalidation) error {
	if b == nil || b.ID() != s.batchID {
		return fmt.Errorf("erp batch store save: batch does not match the locked batch %d", s.batchID)
	}
	// Rows replaced earlier in this tx are the new artifacts: keep them.
	if s.replacedDemand {
		inv.Demand = false
	}
	if s.replacedCoverage {
		inv.Coverage = false
	}
	if s.replacedStd {
		inv.StdRows = false
	}
	return saveBatch(ctx, s.tx, b, expected, inv)
}

func (s *erpBatchStore) ReplaceDemand(ctx context.Context, lines []erpintegration.DemandLine) (int64, error) {
	n, err := replaceDemand(ctx, s.tx, s.batchID, lines)
	if err != nil {
		return 0, err
	}
	s.replacedDemand = true
	return n, nil
}

func (s *erpBatchStore) ReplaceCoverage(ctx context.Context, lines []erpintegration.CoverageLine) (int64, error) {
	n, err := replaceCoverage(ctx, s.tx, s.batchID, lines)
	if err != nil {
		return 0, err
	}
	s.replacedCoverage = true
	return n, nil
}

func (s *erpBatchStore) ListDemand(ctx context.Context) ([]erpintegration.DemandLine, error) {
	return listDemand(ctx, s.tx, s.batchID)
}
