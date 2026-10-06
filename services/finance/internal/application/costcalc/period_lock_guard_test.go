package costcalc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// Period-lock guard tests (plan-02 P1-T3, AC-09). A locked ACTUAL request must
// return ErrPeriodLocked with zero repository writes; FORECAST / SELLING and a
// nil checker proceed exactly as before.

// stubPeriodLock answers IsLocked from a fixed (period, type) set.
type stubPeriodLock struct {
	locked map[string]bool // key: period+"/"+type
	err    error
	calls  int
}

func (s *stubPeriodLock) IsLocked(_ context.Context, period, costType string) (bool, error) {
	s.calls++
	return s.locked[period+"/"+costType], s.err
}

func lockedActual(period string) *stubPeriodLock {
	return &stubPeriodLock{locked: map[string]bool{period + "/ACTUAL": true}}
}

// spyJobRepo counts every write; reads are unused on these paths.
type spyJobRepo struct {
	costcalcdom.JobRepository
	creates, updates int
}

func (s *spyJobRepo) Create(_ context.Context, j *costcalcdom.Job) error {
	s.creates++
	j.AssignID(int64(s.creates), "JOB")
	return nil
}

func (s *spyJobRepo) UpdateStatus(context.Context, int64, costcalcdom.JobStatus) error {
	s.updates++
	return nil
}

type spyPublisher struct{ published []int64 }

func (s *spyPublisher) PublishJobTriggered(_ context.Context, id int64) error {
	s.published = append(s.published, id)
	return nil
}

func newLockTriggerHandler(jobs *spyJobRepo, pub *spyPublisher, lock costcalcdom.PeriodLockChecker) *TriggerJobHandler {
	svc := NewService(jobs, nil, nil, nil, nil, nil, nil, nil, pub)
	return NewTriggerJobHandler(svc, WithTriggerPeriodLock(lock))
}

func TestTriggerJob_LockedActual_RefusedBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	for _, scope := range []costcalcdom.JobScope{costcalcdom.ScopeAll, costcalcdom.ScopeSingleProduct, costcalcdom.ScopeFiltered} {
		t.Run(string(scope), func(t *testing.T) {
			t.Parallel()
			jobs, pub := &spyJobRepo{}, &spyPublisher{}
			h := newLockTriggerHandler(jobs, pub, lockedActual("202608"))
			_, err := h.Handle(context.Background(), TriggerCommand{
				Period: "202608", CalcType: costcalcdom.CalcTypeActual, Scope: scope,
				ProductSysID: 7, Actor: "u", TriggeredBy: "TEST",
			})
			require.ErrorIs(t, err, costcalcdom.ErrPeriodLocked)
			assert.Zero(t, jobs.creates)
			assert.Zero(t, jobs.updates)
			assert.Empty(t, pub.published)
		})
	}
}

func TestTriggerJob_LockedPeriod_ForecastAndSellingProceed(t *testing.T) {
	t.Parallel()
	for _, ct := range []costcalcdom.CalculationType{costcalcdom.CalcTypeForecast, costcalcdom.CalcTypeSelling} {
		t.Run(string(ct), func(t *testing.T) {
			t.Parallel()
			jobs, pub := &spyJobRepo{}, &spyPublisher{}
			lock := lockedActual("202608")
			h := newLockTriggerHandler(jobs, pub, lock)
			job, err := h.Handle(context.Background(), TriggerCommand{
				Period: "202608", CalcType: ct, Scope: costcalcdom.ScopeAll, Actor: "u", TriggeredBy: "TEST",
			})
			require.NoError(t, err)
			require.NotNil(t, job)
			assert.Equal(t, 1, jobs.creates)
			assert.Len(t, pub.published, 1)
			assert.Zero(t, lock.calls, "non-ACTUAL never consults the lock")
		})
	}
}

func TestTriggerJob_UnlockedActualAndNilChecker_Proceed(t *testing.T) {
	t.Parallel()
	for name, lock := range map[string]costcalcdom.PeriodLockChecker{
		"unlocked": lockedActual("202607"),
		"nil":      nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			jobs, pub := &spyJobRepo{}, &spyPublisher{}
			h := newLockTriggerHandler(jobs, pub, lock)
			_, err := h.Handle(context.Background(), TriggerCommand{
				Period: "202608", CalcType: costcalcdom.CalcTypeActual, Scope: costcalcdom.ScopeAll, Actor: "u", TriggeredBy: "TEST",
			})
			require.NoError(t, err)
			assert.Equal(t, 1, jobs.creates)
		})
	}
}

func TestTriggerJob_LockCheckerError_Propagates(t *testing.T) {
	t.Parallel()
	jobs, pub := &spyJobRepo{}, &spyPublisher{}
	boom := errors.New("db down")
	h := newLockTriggerHandler(jobs, pub, &stubPeriodLock{err: boom})
	_, err := h.Handle(context.Background(), TriggerCommand{
		Period: "202608", CalcType: costcalcdom.CalcTypeActual, Scope: costcalcdom.ScopeAll, Actor: "u", TriggeredBy: "TEST",
	})
	require.ErrorIs(t, err, boom)
	assert.Zero(t, jobs.creates)
}

// lockResultRepo serves GetByID from a fixed row and records transitions.
type lockResultRepo struct {
	stubResultRepo
	row    *costcalcdom.Result
	getErr error
}

func (s *lockResultRepo) GetByID(context.Context, int64) (*costcalcdom.Result, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.row, nil
}

func lockTestRow(period string, ct costcalcdom.CalculationType) *costcalcdom.Result {
	return costcalcdom.NewResult(
		1, period, ct, 1, 1,
		10, 6, 4, 10, 0, "USD",
		nil, nil, nil, nil, "h", 1, "t",
		0, 0, 0, 0, 0, 0, 0,
	)
}

type transitionCase struct {
	name string
	run  func(repo *lockResultRepo, lock costcalcdom.PeriodLockChecker) error
	done func(repo *lockResultRepo) []int64
}

func transitionCases() []transitionCase {
	return []transitionCase{
		{
			name: "verify",
			run: func(repo *lockResultRepo, lock costcalcdom.PeriodLockChecker) error {
				svc := NewService(nil, nil, nil, repo, nil, nil, nil, nil, nil)
				return NewVerifyCostHandler(svc, WithVerifyPeriodLock(lock)).Handle(context.Background(), VerifyCostCommand{CostID: 5, Actor: "v"})
			},
			done: func(repo *lockResultRepo) []int64 { return repo.verified },
		},
		{
			name: "approve",
			run: func(repo *lockResultRepo, lock costcalcdom.PeriodLockChecker) error {
				svc := NewService(nil, nil, nil, repo, nil, nil, nil, nil, nil)
				return NewApproveCostHandler(svc, WithApprovePeriodLock(lock)).Handle(context.Background(), ApproveCostCommand{CostID: 5, Actor: "a"})
			},
			done: func(repo *lockResultRepo) []int64 { return repo.approved },
		},
	}
}

func TestVerifyApprove_LockedActual_RefusedWithNoWrite(t *testing.T) {
	t.Parallel()
	for _, tc := range transitionCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &lockResultRepo{row: lockTestRow("202608", costcalcdom.CalcTypeActual)}
			err := tc.run(repo, lockedActual("202608"))
			require.ErrorIs(t, err, costcalcdom.ErrPeriodLocked)
			assert.Empty(t, tc.done(repo))
		})
	}
}

func TestVerifyApprove_LockedPeriod_ForecastSellingAndUnlockedProceed(t *testing.T) {
	t.Parallel()
	rows := map[string]*costcalcdom.Result{
		"forecast":        lockTestRow("202608", costcalcdom.CalcTypeForecast),
		"selling":         lockTestRow("202608", costcalcdom.CalcTypeSelling),
		"actual-unlocked": lockTestRow("202607", costcalcdom.CalcTypeActual),
	}
	for _, tc := range transitionCases() {
		for name, row := range rows {
			t.Run(tc.name+"/"+name, func(t *testing.T) {
				t.Parallel()
				repo := &lockResultRepo{row: row}
				require.NoError(t, tc.run(repo, lockedActual("202608")))
				assert.Equal(t, []int64{5}, tc.done(repo))
			})
		}
	}
}

func TestVerifyApprove_NilChecker_SkipsLookup(t *testing.T) {
	t.Parallel()
	for _, tc := range transitionCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// getErr would surface if the guard looked the row up.
			repo := &lockResultRepo{getErr: errors.New("must not be called")}
			require.NoError(t, tc.run(repo, nil))
			assert.Equal(t, []int64{5}, tc.done(repo))
		})
	}
}

func TestVerifyApprove_LookupErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range transitionCases() {
		t.Run(tc.name+"/not-found-falls-through", func(t *testing.T) {
			t.Parallel()
			repo := &lockResultRepo{getErr: costcalcdom.ErrCostNotFound}
			require.NoError(t, tc.run(repo, lockedActual("202608")))
			assert.Equal(t, []int64{5}, tc.done(repo))
		})
		t.Run(tc.name+"/db-error-propagates", func(t *testing.T) {
			t.Parallel()
			boom := errors.New("db down")
			repo := &lockResultRepo{getErr: boom}
			require.ErrorIs(t, tc.run(repo, lockedActual("202608")), boom)
			assert.Empty(t, tc.done(repo))
		})
		t.Run(tc.name+"/checker-error-propagates", func(t *testing.T) {
			t.Parallel()
			boom := errors.New("lock down")
			repo := &lockResultRepo{row: lockTestRow("202608", costcalcdom.CalcTypeActual)}
			require.ErrorIs(t, tc.run(repo, &stubPeriodLock{err: boom}), boom)
			assert.Empty(t, tc.done(repo))
		})
	}
}
