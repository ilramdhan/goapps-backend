package mbbatch

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// Period-lock guard on the MB_BATCH recompute entry (plan-02 P1-T3, AC-09).

type stubPeriodLock struct {
	locked map[string]bool
	err    error
}

func (s *stubPeriodLock) IsLocked(_ context.Context, period, costType string) (bool, error) {
	return s.locked[period+"/"+costType], s.err
}

// spyJobRepo counts Create calls and fails them, so a guard that lets the
// request through is observable without running the batch.
type spyJobRepo struct {
	costcalcdom.JobRepository
	creates int
}

var errCreateReached = errors.New("create reached")

func (s *spyJobRepo) Create(context.Context, *costcalcdom.Job) error {
	s.creates++
	return errCreateReached
}

func TestTriggerHandler_LockedActual_RefusedBeforeJobRow(t *testing.T) {
	t.Parallel()
	jobs := &spyJobRepo{}
	h := NewTriggerHandler(nil, jobs, WithPeriodLock(&stubPeriodLock{locked: map[string]bool{"202608/ACTUAL": true}}))
	_, err := h.Handle(context.Background(), "202608", "u")
	require.ErrorIs(t, err, costcalcdom.ErrPeriodLocked)
	assert.Zero(t, jobs.creates)
}

func TestTriggerHandler_UnlockedOrNilChecker_Proceeds(t *testing.T) {
	t.Parallel()
	for name, lock := range map[string]costcalcdom.PeriodLockChecker{
		"other-period-locked": &stubPeriodLock{locked: map[string]bool{"202607/ACTUAL": true}},
		"nil":                 nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			jobs := &spyJobRepo{}
			h := NewTriggerHandler(nil, jobs, WithPeriodLock(lock))
			_, err := h.Handle(context.Background(), "202608", "u")
			require.ErrorIs(t, err, errCreateReached)
			assert.Equal(t, 1, jobs.creates)
		})
	}
}

func TestTriggerHandler_LockCheckerError_Propagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("db down")
	jobs := &spyJobRepo{}
	h := NewTriggerHandler(nil, jobs, WithPeriodLock(&stubPeriodLock{err: boom}))
	_, err := h.Handle(context.Background(), "202608", "u")
	require.ErrorIs(t, err, boom)
	assert.Zero(t, jobs.creates)
}
