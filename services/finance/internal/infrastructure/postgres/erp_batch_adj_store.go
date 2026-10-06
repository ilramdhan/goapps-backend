package postgres

// erp_batch_adj_store.go adds the ADJ execute reads/writes to the
// transaction-scoped batch store (plan-06 P5-T5; design Part 2 §9.4, §5.3
// C-8/C-9). PG side only: nothing here talks to Oracle.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

var _ erpintegration.AdjExecStore = (*erpBatchStore)(nil)

// ActiveForUpdate returns the LIVE VALUATED/RECONCILED/LOCKED batch of the
// period other than the locked batch, row-locked, or ErrBatchNotFound.
func (s *erpBatchStore) ActiveForUpdate(ctx context.Context, period string) (*erpintegration.Batch, error) {
	b, err := scanBatch(s.tx.QueryRowContext(ctx, `SELECT `+erpBatchColumns+` FROM cst_erp_int_batch
		 WHERE ceib_period = $1 AND ceib_mode = 'LIVE'
		   AND ceib_status IN ('VALUATED', 'RECONCILED', 'LOCKED')
		   AND ceib_batch_id <> $2
		 FOR UPDATE`, period, s.batchID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, erpintegration.ErrBatchNotFound
	}
	return b, err
}

// SaveOther persists another batch of the period in the same transaction
// (the SUPERSEDE of the prior active batch).
func (s *erpBatchStore) SaveOther(ctx context.Context, b *erpintegration.Batch, expected erpintegration.BatchStatus) error {
	if b == nil || b.ID() == s.batchID {
		return fmt.Errorf("erp batch store save other: use Save for the locked batch %d", s.batchID)
	}
	return saveBatch(ctx, s.tx, b, expected, erpintegration.Invalidation{})
}

// IsLatestBatch reports whether the locked batch has the highest seq of its
// period.
func (s *erpBatchStore) IsLatestBatch(ctx context.Context, period string) (bool, error) {
	var latest bool
	if err := s.tx.QueryRowContext(ctx, `SELECT NOT EXISTS (
		SELECT 1 FROM cst_erp_int_batch o
		 WHERE o.ceib_period = $1
		   AND o.ceib_seq > (SELECT b.ceib_seq FROM cst_erp_int_batch b WHERE b.ceib_batch_id = $2))`,
		period, s.batchID).Scan(&latest); err != nil {
		return false, fmt.Errorf("erp batch latest check: %w", err)
	}
	return latest, nil
}
