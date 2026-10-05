package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
)

// PeriodLockRepository implements periodlock.Repository and the
// costcalc.PeriodLockChecker port over cst_period_lock (migration 000550;
// design §4.3, §5.5; plan-02 P1-T2).
//
// Unlock is a soft unlock (cpl_unlocked_at/by/reason are set; the row stays),
// so a lock is in force iff cpl_unlocked_at IS NULL.
type PeriodLockRepository struct{ db *DB }

// NewPeriodLockRepository constructs the repository.
func NewPeriodLockRepository(db *DB) *PeriodLockRepository {
	return &PeriodLockRepository{db: db}
}

var (
	_ periodlock.Repository         = (*PeriodLockRepository)(nil)
	_ costcalcdom.PeriodLockChecker = (*PeriodLockRepository)(nil)
)

const isPeriodLockedSQL = `
		SELECT EXISTS (
			SELECT 1 FROM cst_period_lock
			 WHERE cpl_period = $1 AND cpl_calc_type = $2 AND cpl_unlocked_at IS NULL)`

// IsLocked is a plain SELECT (no row lock).
func (r *PeriodLockRepository) IsLocked(ctx context.Context, period, calcType string) (bool, error) {
	var locked bool
	if err := r.db.QueryRowContext(ctx, isPeriodLockedSQL, period, calcType).Scan(&locked); err != nil {
		return false, fmt.Errorf("period lock is-locked: %w", err)
	}
	return locked, nil
}

// isPeriodLockedForShareSQL takes a FOR SHARE row lock on the (period, type)
// lock row whatever its state, and reports whether it is in force. The state
// test is in the select list, not the WHERE: under READ COMMITTED a WHERE
// predicate is evaluated against the snapshot before the row lock, so a
// soft-unlocked row being re-locked by an uncommitted tx would be filtered out
// and never waited on. Locking the row itself serializes the caller against a
// concurrent Lock (re-lock) and Unlock alike; after the wait PostgreSQL
// re-reads the committed row version. No row = never locked = false.
const isPeriodLockedForShareSQL = `
		SELECT cpl_unlocked_at IS NULL FROM cst_period_lock
		 WHERE cpl_period = $1 AND cpl_calc_type = $2
		 FOR SHARE`

// IsPeriodLockedForShare runs the lock check inside tx with FOR SHARE, so the
// answer stays valid until tx ends (plan-02 P1-T2 step 2; used by the
// supersedePrevious guard, P1-T4, and the MB push in-tx re-check).
func IsPeriodLockedForShare(ctx context.Context, tx *sql.Tx, period, calcType string) (bool, error) {
	var locked bool
	err := tx.QueryRowContext(ctx, isPeriodLockedForShareSQL, period, calcType).Scan(&locked)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("period lock for share: %w", err)
	default:
		return locked, nil
	}
}

// IsLockedForShare is IsPeriodLockedForShare as a method.
func (r *PeriodLockRepository) IsLockedForShare(ctx context.Context, tx *sql.Tx, period, calcType string) (bool, error) {
	return IsPeriodLockedForShare(ctx, tx, period, calcType)
}

const selectPeriodLockSQL = `
		SELECT cpl_period, cpl_calc_type, cpl_locked_at, cpl_locked_by, cpl_reason,
		       cpl_erp_batch_id, cpl_unlocked_at, COALESCE(cpl_unlocked_by, ''),
		       COALESCE(cpl_unlock_reason, '')
		  FROM cst_period_lock
		 WHERE cpl_period = $1 AND cpl_calc_type = $2`

// Get returns the lock row or periodlock.ErrLockNotFound.
func (r *PeriodLockRepository) Get(ctx context.Context, period, calcType string) (*periodlock.PeriodLock, error) {
	var (
		p, ct, lockedBy, reason, unlockedBy, unlockReason string
		lockedAt                                          time.Time
		batch                                             sql.NullInt64
		unlockedAt                                        sql.NullTime
	)
	err := r.db.QueryRowContext(ctx, selectPeriodLockSQL, period, calcType).Scan(
		&p, &ct, &lockedAt, &lockedBy, &reason, &batch, &unlockedAt, &unlockedBy, &unlockReason)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, periodlock.ErrLockNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("period lock get: %w", err)
	}
	var batchID *int64
	if batch.Valid {
		v := batch.Int64
		batchID = &v
	}
	var unl *time.Time
	if unlockedAt.Valid {
		v := unlockedAt.Time
		unl = &v
	}
	return periodlock.Reconstruct(p, ct, lockedAt, lockedBy, reason, batchID, unl, unlockedBy, unlockReason), nil
}

// HasLockedErpBatch reports whether any batch for the period is LOCKED.
func (r *PeriodLockRepository) HasLockedErpBatch(ctx context.Context, period string) (bool, error) {
	const q = `SELECT EXISTS (
		SELECT 1 FROM cst_erp_int_batch WHERE ceib_period = $1 AND ceib_status = 'LOCKED')`
	var has bool
	if err := r.db.QueryRowContext(ctx, q, period).Scan(&has); err != nil {
		return false, fmt.Errorf("period lock locked-batch probe: %w", err)
	}
	return has, nil
}

// lockSQL inserts a new lock, or re-locks a soft-unlocked row. The WHERE on
// the conflict branch makes it a no-op (0 rows) when the row is still
// locked, which maps to ErrAlreadyLocked.
const lockSQL = `
		INSERT INTO cst_period_lock (cpl_period, cpl_calc_type, cpl_locked_at, cpl_locked_by, cpl_reason)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (cpl_period, cpl_calc_type) DO UPDATE
		   SET cpl_locked_at     = EXCLUDED.cpl_locked_at,
		       cpl_locked_by     = EXCLUDED.cpl_locked_by,
		       cpl_reason        = EXCLUDED.cpl_reason,
		       cpl_erp_batch_id  = NULL,
		       cpl_unlocked_at   = NULL,
		       cpl_unlocked_by   = NULL,
		       cpl_unlock_reason = NULL
		 WHERE cst_period_lock.cpl_unlocked_at IS NOT NULL`

// lockTableSQL serializes Lock against every in-flight guarded ACTUAL write
// (plan-02 P1-T6, AC-09 race). Every guard reads the lock row with
// SELECT ... FOR SHARE, which takes a ROW SHARE table lock even when no row
// matches. EXCLUSIVE conflicts with ROW SHARE, so Lock waits for in-flight
// supersede / push transactions to finish, and new ones wait for Lock to
// commit and then see the committed row. Without it a FIRST lock of a period
// (no row yet, so nothing for FOR SHARE to lock) could commit while an ACTUAL
// write that passed the check commits after it. Plain SELECTs (ACCESS SHARE)
// are not blocked.
const lockTableSQL = `LOCK TABLE cst_period_lock IN EXCLUSIVE MODE`

// Lock persists a locked PeriodLock.
func (r *PeriodLockRepository) Lock(ctx context.Context, l *periodlock.PeriodLock) error {
	if l == nil || !l.IsLocked() {
		return fmt.Errorf("period lock: %w", periodlock.ErrNotLocked)
	}
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, lockTableSQL); err != nil {
			return fmt.Errorf("period lock table lock: %w", err)
		}
		res, err := tx.ExecContext(ctx, lockSQL, l.Period(), l.CalcType(), l.LockedAt(), l.LockedBy(), l.Reason())
		if err != nil {
			return fmt.Errorf("period lock insert: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("period lock rows affected: %w", err)
		}
		if n == 0 {
			return periodlock.ErrAlreadyLocked
		}
		return nil
	})
}

const unlockSQL = `
		UPDATE cst_period_lock
		   SET cpl_unlocked_at = $3, cpl_unlocked_by = $4, cpl_unlock_reason = $5
		 WHERE cpl_period = $1 AND cpl_calc_type = $2 AND cpl_unlocked_at IS NULL`

const flagRepushSQL = `
		UPDATE cst_erp_int_batch
		   SET ceib_needs_repush = TRUE, updated_at = NOW(), updated_by = $2
		 WHERE ceib_period = $1
		   AND ceib_mode = 'LIVE'
		   AND ceib_status IN ('PUSHED', 'VALUATED', 'RECONCILED')
		   AND ceib_needs_repush = FALSE`

// Unlock soft-unlocks the row and flags ceib_needs_repush on affected
// batches, in one transaction.
func (r *PeriodLockRepository) Unlock(ctx context.Context, l *periodlock.PeriodLock) (flagged int64, err error) {
	if l == nil || l.IsLocked() || l.UnlockedAt() == nil {
		return 0, fmt.Errorf("period unlock: entity is not unlocked")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("period unlock begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback() //nolint:errcheck // rollback after a failure; the original error wins
		}
	}()

	res, err := tx.ExecContext(ctx, unlockSQL, l.Period(), l.CalcType(), *l.UnlockedAt(), l.UnlockedBy(), l.UnlockReason())
	if err != nil {
		return 0, fmt.Errorf("period unlock update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("period unlock rows affected: %w", err)
	}
	if n == 0 {
		err = periodlock.ErrNotLocked
		return 0, err
	}
	res, err = tx.ExecContext(ctx, flagRepushSQL, l.Period(), l.UnlockedBy())
	if err != nil {
		return 0, fmt.Errorf("period unlock flag repush: %w", err)
	}
	if flagged, err = res.RowsAffected(); err != nil {
		return 0, fmt.Errorf("period unlock repush rows affected: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("period unlock commit: %w", err)
	}
	return flagged, nil
}
