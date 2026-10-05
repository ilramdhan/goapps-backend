package periodlock

import "context"

// Repository persists cst_period_lock.
//
// The transactional FOR SHARE check used by supersedePrevious (P1-T4) lives
// in the postgres package (IsPeriodLockedForShare) because it takes a *sql.Tx.
type Repository interface {
	// Get returns the lock row for (period, calcType), or ErrLockNotFound.
	Get(ctx context.Context, period, calcType string) (*PeriodLock, error)
	// IsLocked is a plain SELECT: true when a row exists with no unlock.
	IsLocked(ctx context.Context, period, calcType string) (bool, error)
	// HasLockedErpBatch reports whether any ERP integration batch for the
	// period is in status LOCKED (design §5.5: unlock is refused).
	HasLockedErpBatch(ctx context.Context, period string) (bool, error)
	// Lock inserts the lock, or re-locks a soft-unlocked row. It returns
	// ErrAlreadyLocked when the period is already locked (race-safe).
	Lock(ctx context.Context, lock *PeriodLock) error
	// Unlock persists the soft unlock of a locked row and, in the same
	// transaction, flags ceib_needs_repush on LIVE batches of the period in
	// PUSHED / VALUATED / RECONCILED (design §5.5). It returns ErrNotLocked
	// when the row is not locked (race-safe) and the number of batches flagged.
	Unlock(ctx context.Context, lock *PeriodLock) (repushFlagged int64, err error)
}

// AdjPostedProbe answers whether the ERP ADJ for a period is posted
// (ADJH_POST_STATUS set on any head). The real implementation is the
// read-only Oracle probe (P3-T3); a static fake is used until then.
type AdjPostedProbe interface {
	IsAdjPosted(ctx context.Context, period string) (bool, error)
}
