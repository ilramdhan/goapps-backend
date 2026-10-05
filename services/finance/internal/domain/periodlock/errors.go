package periodlock

import "errors"

// Sentinel errors for the period lock domain (design Part 1 §5.5).
var (
	// ErrPeriodLocked is returned by every guarded ACTUAL operation (costcalc
	// trigger/verify/approve, mbbatch recompute, mbpush execute,
	// supersedePrevious) while (period, ACTUAL) is locked. Delivery maps it to
	// gRPC FailedPrecondition (P6).
	ErrPeriodLocked = errors.New("period is locked for ACTUAL costing")

	// ErrUnlockPostedPeriod is returned when an unlock is requested for a
	// period whose ERP ADJ is already posted (read-only probe).
	ErrUnlockPostedPeriod = errors.New("period cannot be unlocked: ERP ADJ is posted")

	// ErrUnlockBatchLocked is returned when an unlock is requested for a
	// period that has an ERP integration batch in status LOCKED (ERP freeze).
	ErrUnlockBatchLocked = errors.New("period cannot be unlocked: an ERP batch for the period is LOCKED")

	// ErrAlreadyLocked is returned when locking a period that is already locked.
	ErrAlreadyLocked = errors.New("period is already locked")

	// ErrNotLocked is returned when unlocking a period that is not locked.
	ErrNotLocked = errors.New("period is not locked")

	// ErrLockNotFound is returned when no lock row exists for (period, calc type).
	ErrLockNotFound = errors.New("period lock not found")

	// ErrInvalidPeriod is returned for a period that is not YYYYMM (month 01-12).
	ErrInvalidPeriod = errors.New("period must be YYYYMM")

	// ErrInvalidCalcType is returned for a calc type other than ACTUAL (Q7:
	// FORECAST and SELLING are never locked; widening is deferred to P11).
	ErrInvalidCalcType = errors.New("only ACTUAL periods can be locked")

	// ErrUserRequired is returned when the acting user is empty.
	ErrUserRequired = errors.New("user is required")

	// ErrReasonRequired is returned when the lock/unlock reason is empty.
	ErrReasonRequired = errors.New("reason is required")
)
