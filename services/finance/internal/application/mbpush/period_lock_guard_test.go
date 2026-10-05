package mbpush

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// Period-lock guard on MB push execute (plan-02 P1-T3, AC-09). The in-tx
// FOR SHARE re-check is covered by the finance period-lock integration suite.

type stubPeriodLock struct {
	locked map[string]bool
	err    error
}

func (s *stubPeriodLock) IsLocked(_ context.Context, period, costType string) (bool, error) {
	return s.locked[period+"/"+costType], s.err
}

// countingHeadReader records calls and fails, so reaching it proves the guard
// let the request through without opening a transaction.
type countingHeadReader struct{ calls int }

var errHeadsReached = errors.New("heads reached")

func (c *countingHeadReader) ListValidated(context.Context) ([]MBHeadCandidate, error) {
	c.calls++
	return nil, errHeadsReached
}

func TestExecute_LockedActual_RefusedBeforeAnyReadOrWrite(t *testing.T) {
	t.Parallel()
	heads := &countingHeadReader{}
	h := NewExecuteHandler(nil, heads, nil, nil, nil,
		WithPeriodLock(&stubPeriodLock{locked: map[string]bool{"202608/ACTUAL": true}}))
	_, err := h.Execute(context.Background(), "202608", []string{"MBH-1"}, "u")
	require.ErrorIs(t, err, costcalcdom.ErrPeriodLocked)
	assert.Zero(t, heads.calls)
}

func TestExecute_UnlockedOrNilChecker_Proceeds(t *testing.T) {
	t.Parallel()
	for name, lock := range map[string]costcalcdom.PeriodLockChecker{
		"other-period-locked": &stubPeriodLock{locked: map[string]bool{"202607/ACTUAL": true}},
		"nil":                 nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			heads := &countingHeadReader{}
			h := NewExecuteHandler(nil, heads, nil, nil, nil, WithPeriodLock(lock))
			_, err := h.Execute(context.Background(), "202608", []string{"MBH-1"}, "u")
			require.ErrorIs(t, err, errHeadsReached)
			assert.Equal(t, 1, heads.calls)
		})
	}
}

func TestExecute_LockCheckerError_Propagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("db down")
	heads := &countingHeadReader{}
	h := NewExecuteHandler(nil, heads, nil, nil, nil, WithPeriodLock(&stubPeriodLock{err: boom}))
	_, err := h.Execute(context.Background(), "202608", nil, "u")
	require.ErrorIs(t, err, boom)
	assert.Zero(t, heads.calls)
}

func TestCheckLockInTx_NilCheckerIsNoop(t *testing.T) {
	t.Parallel()
	h := NewExecuteHandler(nil, nil, nil, nil, nil)
	require.NoError(t, h.checkLockInTx(context.Background(), nil, "202608"))
}
