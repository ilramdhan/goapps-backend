package erpintegration

// adj_execute.go holds the ports of the ADJ execute steps (plan-06 P5-T5;
// design Part 2 §9.4; Part 1 §3.2 G4-G11, §S-R11): the W2 call-log keys, the
// one-shot preview consumption and the transaction-scoped store the steps
// read under the G11 lock. Oracle is changed only through OracleWriter's
// package calls (W2_*); nothing here issues SQL against Oracle.

import (
	"context"
	"errors"
	"time"
)

// W2 allowlist keys of the package calls. They mirror the oracle package
// keys and the chk_ceocl_statement_key values of migration 000554.
const (
	CallKeyW2ValuateAdj = "W2_VALUATE_ADJ"
	CallKeyW2ApproveAdj = "W2_APPROVE_ADJ"
	CallKeyW2RestoreAdj = "W2_RESTORE_ADJ"
	CallKeyW2LockBatch  = "W2_LOCK_BATCH"
)

// ErrPreviewConsumed is returned when the preview was already used (or is
// no longer OPEN when the executor tries to consume it): every execution
// needs a fresh preview, and a redelivered job never calls Oracle twice.
var ErrPreviewConsumed = errors.New("erpintegration: preview already consumed")

// PreviewConsumer flips a preview OPEN -> CONSUMED exactly once. Consume
// commits on its own (autocommit) so a later abort of the step still leaves
// the preview consumed; it returns ErrPreviewConsumed when no OPEN row was
// updated.
type PreviewConsumer interface {
	Consume(ctx context.Context, previewID, actor string, at time.Time) error
}

// AdjExecStore is the transaction-scoped store of the ADJ execute steps. The
// PG BatchTxRunner hands a BatchStore that also implements it.
type AdjExecStore interface {
	BatchStore
	// IsPeriodLocked is the G10 check inside the transaction (FOR SHARE).
	IsPeriodLocked(ctx context.Context, period, calcType string) (bool, error)
	// ListStdRows returns every std row of the locked batch ordered by key.
	ListStdRows(ctx context.Context) ([]StdRow, error)
	// ActiveForUpdate returns the LIVE VALUATED/RECONCILED/LOCKED batch of
	// the period other than the locked batch, row-locked (FOR UPDATE), or
	// ErrBatchNotFound.
	ActiveForUpdate(ctx context.Context, period string) (*Batch, error)
	// SaveOther persists another batch of the same period (the SUPERSEDE of
	// the prior active batch, design §5.3 / C-8) in the same transaction.
	SaveOther(ctx context.Context, b *Batch, expected BatchStatus) error
	// IsLatestBatch reports whether the locked batch has the highest seq of
	// its period (RESTORE_ADJ runs on the latest batch only, C-9).
	IsLatestBatch(ctx context.Context, period string) (bool, error)
}
