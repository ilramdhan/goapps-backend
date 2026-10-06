package costcalc

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
)

// PeriodLockChecker is the port the costcalc, mbbatch and mbpush handlers use
// to refuse ACTUAL writes for a locked period (design §5.5; plan-02 P1-T1).
// The postgres period lock repository implements it with a plain SELECT.
type PeriodLockChecker interface {
	IsLocked(ctx context.Context, period string, costType string) (bool, error)
}

// ErrPeriodLocked re-exports periodlock.ErrPeriodLocked for costcalc callers.
var ErrPeriodLocked = periodlock.ErrPeriodLocked

// CheckPeriodUnlocked returns ErrPeriodLocked when checker reports
// (period, ACTUAL) as locked. A nil checker (legacy wiring and tests) and any
// non-ACTUAL calc type always pass, so behavior without a lock is unchanged.
func CheckPeriodUnlocked(ctx context.Context, checker PeriodLockChecker, period string, calcType CalculationType) error {
	if checker == nil || calcType != CalcTypeActual {
		return nil
	}
	locked, err := checker.IsLocked(ctx, period, string(calcType))
	if err != nil {
		return err
	}
	if locked {
		return ErrPeriodLocked
	}
	return nil
}
