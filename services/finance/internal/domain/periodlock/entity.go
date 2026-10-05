// Package periodlock is the cost-freeze domain for (period, ACTUAL)
// (design Part 1 §4.3, §5.5; plan-02 P1-T1).
//
// A lock is one row of cst_period_lock (migration 000550). It is locked while
// UnlockedAt is nil. An unlock is a soft unlock: the row stays and the
// cpl_unlocked_* columns are set, so the P1-T4 guard (`cpl_unlocked_at IS
// NULL`) and the audit history both hold. A re-lock of a soft-unlocked row
// reuses the same primary key.
package periodlock

import (
	"regexp"
	"strings"
	"time"
)

// CalcTypeActual is the only lockable calculation type (Q7). It equals
// costcalc.CalcTypeActual; it is repeated here to keep this domain free of
// imports from other domains.
const CalcTypeActual = "ACTUAL"

// maxUserLen mirrors cpl_locked_by / cpl_unlocked_by VARCHAR(64).
const maxUserLen = 64

var periodRe = regexp.MustCompile(`^[0-9]{4}(0[1-9]|1[0-2])$`)

// ValidatePeriod checks the YYYYMM format enforced by chk_cpl_period.
func ValidatePeriod(period string) error {
	if !periodRe.MatchString(period) {
		return ErrInvalidPeriod
	}
	return nil
}

// ValidateCalcType accepts ACTUAL only (chk_cpl_calc_type).
func ValidateCalcType(calcType string) error {
	if calcType != CalcTypeActual {
		return ErrInvalidCalcType
	}
	return nil
}

// PeriodLock is the aggregate for one (period, calc type) cost freeze.
type PeriodLock struct {
	period       string
	calcType     string
	lockedAt     time.Time
	lockedBy     string
	reason       string
	erpBatchID   *int64
	unlockedAt   *time.Time
	unlockedBy   string
	unlockReason string
}

// NewPeriodLock creates a new, locked PeriodLock.
func NewPeriodLock(period, calcType, user, reason string, now time.Time) (*PeriodLock, error) {
	if err := ValidatePeriod(period); err != nil {
		return nil, err
	}
	if err := ValidateCalcType(calcType); err != nil {
		return nil, err
	}
	user, reason, err := validateActor(user, reason)
	if err != nil {
		return nil, err
	}
	return &PeriodLock{
		period:   period,
		calcType: calcType,
		lockedAt: now,
		lockedBy: user,
		reason:   reason,
	}, nil
}

// Reconstruct rebuilds a PeriodLock from persistence without validation.
func Reconstruct(
	period, calcType string, lockedAt time.Time, lockedBy, reason string,
	erpBatchID *int64, unlockedAt *time.Time, unlockedBy, unlockReason string,
) *PeriodLock {
	return &PeriodLock{
		period: period, calcType: calcType,
		lockedAt: lockedAt, lockedBy: lockedBy, reason: reason,
		erpBatchID: erpBatchID,
		unlockedAt: unlockedAt, unlockedBy: unlockedBy, unlockReason: unlockReason,
	}
}

func validateActor(user, reason string) (string, string, error) {
	user = strings.TrimSpace(user)
	reason = strings.TrimSpace(reason)
	if user == "" {
		return "", "", ErrUserRequired
	}
	if len(user) > maxUserLen {
		user = user[:maxUserLen]
	}
	if reason == "" {
		return "", "", ErrReasonRequired
	}
	return user, reason, nil
}

// Lock re-locks a soft-unlocked period. It returns ErrAlreadyLocked when the
// period is still locked. The ERP batch link is cleared: a new LOCK_BATCH
// sets it again.
func (l *PeriodLock) Lock(user, reason string, now time.Time) error {
	if l.IsLocked() {
		return ErrAlreadyLocked
	}
	user, reason, err := validateActor(user, reason)
	if err != nil {
		return err
	}
	l.lockedAt = now
	l.lockedBy = user
	l.reason = reason
	l.erpBatchID = nil
	l.unlockedAt = nil
	l.unlockedBy = ""
	l.unlockReason = ""
	return nil
}

// Unlock soft-unlocks the period. It is refused with ErrUnlockPostedPeriod
// when the ERP ADJ for the period is posted, and with ErrNotLocked when the
// period is not locked.
func (l *PeriodLock) Unlock(user, reason string, adjPosted bool, now time.Time) error {
	if !l.IsLocked() {
		return ErrNotLocked
	}
	user, reason, err := validateActor(user, reason)
	if err != nil {
		return err
	}
	if adjPosted {
		return ErrUnlockPostedPeriod
	}
	t := now
	l.unlockedAt = &t
	l.unlockedBy = user
	l.unlockReason = reason
	return nil
}

// IsLocked reports whether the lock is in force.
func (l *PeriodLock) IsLocked() bool { return l.unlockedAt == nil }

// Period returns YYYYMM.
func (l *PeriodLock) Period() string { return l.period }

// CalcType returns the calculation type (ACTUAL).
func (l *PeriodLock) CalcType() string { return l.calcType }

// LockedAt returns when the (latest) lock was taken.
func (l *PeriodLock) LockedAt() time.Time { return l.lockedAt }

// LockedBy returns who took the (latest) lock.
func (l *PeriodLock) LockedBy() string { return l.lockedBy }

// Reason returns the lock reason.
func (l *PeriodLock) Reason() string { return l.reason }

// ErpBatchID returns the ERP batch set at LOCK_BATCH, or nil.
func (l *PeriodLock) ErpBatchID() *int64 { return l.erpBatchID }

// UnlockedAt returns when the lock was released, or nil while locked.
func (l *PeriodLock) UnlockedAt() *time.Time { return l.unlockedAt }

// UnlockedBy returns who released the lock ("" while locked).
func (l *PeriodLock) UnlockedBy() string { return l.unlockedBy }

// UnlockReason returns the unlock reason ("" while locked).
func (l *PeriodLock) UnlockReason() string { return l.unlockReason }
