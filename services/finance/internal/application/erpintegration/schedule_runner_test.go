package erpintegration

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// listingBatches adds a real ListByPeriod to memBatches.
type listingBatches struct{ *memBatches }

func (l listingBatches) ListByPeriod(_ context.Context, period string) ([]*domain.Batch, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []*domain.Batch
	for _, st := range l.batches {
		if st.Period == period {
			b, err := domain.ReconstituteBatch(st)
			if err != nil {
				return nil, err
			}
			out = append(out, b)
		}
	}
	return out, nil
}

// countingWriter fails the test if any Oracle write is attempted.
type countingWriter struct{ calls atomic.Int64 }

func (w *countingWriter) InsertBatch(context.Context, domain.PushBatch, []domain.PushRow) (domain.WriteResult, error) {
	w.calls.Add(1)
	return domain.WriteResult{}, nil
}

func (w *countingWriter) ValuateAdj(context.Context, int64) (domain.Summary, error) {
	w.calls.Add(1)
	return domain.Summary{}, nil
}

func (w *countingWriter) ApproveAdj(context.Context, int64, string) (domain.Summary, error) {
	w.calls.Add(1)
	return domain.Summary{}, nil
}

func (w *countingWriter) RestoreAdj(context.Context, int64) (domain.Summary, error) {
	w.calls.Add(1)
	return domain.Summary{}, nil
}

func (w *countingWriter) LockBatch(context.Context, int64) (domain.Summary, error) {
	w.calls.Add(1)
	return domain.Summary{}, nil
}

type runnerEnv struct {
	batches listingBatches
	jobs    *memJobs
	pub     *fakePublisher
	writer  *countingWriter
	locks   *fakeLocks
	prober  *fakeProber
	runner  *ScheduleRunner
}

func newRunnerEnv() *runnerEnv {
	e := &runnerEnv{
		batches: listingBatches{newMemBatches()}, jobs: newMemJobs(), pub: &fakePublisher{},
		writer: &countingWriter{}, locks: &fakeLocks{locked: true}, prober: &fakeProber{},
	}
	create := NewCreateBatchHandler(e.batches, e.locks, e.prober)
	// Push gates fully open with a live writer: the runner must still never
	// reach it.
	steps := NewStepTriggerHandler(e.jobs, e.batches, e.pub, 0).
		WithPushGates(true, WriterGate{Writer: e.writer, Mode: domain.WriterModeFake})
	e.runner = NewScheduleRunner(e.batches, create, steps)
	return e
}

func (e *runnerEnv) subtypes() []string {
	out := make([]string, 0, len(e.pub.calls))
	for _, c := range e.pub.calls {
		out = append(out, strings.SplitN(c, ":", 2)[0])
	}
	return out
}

func TestScheduleRunner_EnqueuesOnlyLoadDemand(t *testing.T) {
	ctx := context.Background()
	e := newRunnerEnv()
	res, err := e.runner.Run(ctx, "202609")
	require.NoError(t, err)
	assert.False(t, res.Skipped)
	assert.NotZero(t, res.BatchID)
	assert.NotEmpty(t, res.JobID)
	assert.Equal(t, []string{StepLoadDemand}, e.subtypes())
	assert.Equal(t, int64(0), e.writer.calls.Load(), "scheduled run makes 0 writer calls")

	st := e.batches.state(res.BatchID)
	assert.Equal(t, domain.ModeLive, st.Mode)
	assert.Equal(t, domain.StatusDraft, st.Status)
	assert.Equal(t, ScheduledRunActor, st.CreatedBy)
	require.Len(t, e.jobs.execs, 1)
	for _, ex := range e.jobs.execs {
		assert.True(t, IsScheduledRun(ex))
		assert.Equal(t, job.TypeErpIntegration, ex.JobType())
	}
	assert.Equal(t, int64(1), e.runner.RunCount())
}

func TestScheduleRunner_ChainStopsAtValidate(t *testing.T) {
	ctx := context.Background()
	e := newRunnerEnv()
	now := time.Now()
	statusAfter := map[string]domain.BatchStatus{
		StepLoadDemand: domain.StatusDemandLoaded, StepCoverage: domain.StatusCovered,
		StepDerive: domain.StatusDerived, StepValidate: domain.StatusValidated,
	}
	id := e.batches.put(domain.BatchState{Period: "202609", Status: domain.StatusDraft})
	step := StepLoadDemand
	var enqueued []string
	for {
		// Simulate the step job completing.
		st := e.batches.state(id)
		st.Status, st.DemandLoadedAt = statusAfter[step], &now
		e.batches.put(st)
		next, err := e.runner.ContinueAfter(ctx, id, step)
		require.NoError(t, err)
		if next == "" {
			break
		}
		enqueued = append(enqueued, next)
		step = next
	}
	assert.Equal(t, []string{StepCoverage, StepDerive, StepValidate}, enqueued)
	assert.Equal(t, enqueued, e.subtypes(), "nothing after validate, never push")
	assert.Equal(t, int64(0), e.writer.calls.Load())

	for _, s := range []string{StepValidate, StepPush, "valuation", "approve", "restore", "unknown"} {
		next, ok := NextScheduledStep(s)
		assert.False(t, ok, s)
		assert.Empty(t, next)
	}
}

func TestScheduleRunner_Skips(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		setup  func(e *runnerEnv)
		reason string
	}{
		{"not_locked", func(e *runnerEnv) { e.locks.locked = false }, SkipNotLocked},
		{"posted", func(e *runnerEnv) { e.prober.posted = 3 }, SkipPosted},
		{"pushed", func(e *runnerEnv) {
			e.batches.put(domain.BatchState{Period: "202609", Status: domain.StatusPushed})
		}, SkipPushedOrLater},
		{"valuated", func(e *runnerEnv) {
			e.batches.put(domain.BatchState{Period: "202609", Status: domain.StatusValuated})
		}, SkipPushedOrLater},
		{"locked_batch", func(e *runnerEnv) {
			e.batches.put(domain.BatchState{Period: "202609", Status: domain.StatusLocked})
		}, SkipPushedOrLater},
		{"in_flight_validated", func(e *runnerEnv) {
			e.batches.put(domain.BatchState{Period: "202609", Status: domain.StatusValidated})
		}, SkipInFlight},
		{"active_job", func(e *runnerEnv) { e.jobs.active = true }, SkipActiveJob},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newRunnerEnv()
			tc.setup(e)
			before := len(e.batches.batches)
			res, err := e.runner.Run(ctx, "202609")
			require.NoError(t, err, "a skip is not an error (no retry)")
			assert.True(t, res.Skipped)
			assert.Equal(t, tc.reason, res.SkipReason)
			assert.Empty(t, e.pub.calls, "nothing published on skip")
			assert.Equal(t, int64(1), e.runner.SkipCounts()[tc.reason])
			assert.Equal(t, int64(0), e.runner.RunCount())
			assert.Equal(t, int64(0), e.writer.calls.Load())
			if tc.reason != SkipActiveJob {
				assert.Len(t, e.batches.batches, before, "no batch created on skip")
			}
		})
	}

	t.Run("shadow_batch_does_not_block", func(t *testing.T) {
		e := newRunnerEnv()
		e.batches.put(domain.BatchState{Period: "202609", Mode: domain.ModeShadow, Status: domain.StatusValidated})
		res, err := e.runner.Run(ctx, "202609")
		require.NoError(t, err)
		assert.False(t, res.Skipped)
	})
	t.Run("failed_and_superseded_do_not_block", func(t *testing.T) {
		e := newRunnerEnv()
		e.batches.put(domain.BatchState{Period: "202609", Status: domain.StatusFailed})
		e.batches.put(domain.BatchState{Period: "202609", Seq: 2, Status: domain.StatusSuperseded})
		res, err := e.runner.Run(ctx, "202609")
		require.NoError(t, err)
		assert.False(t, res.Skipped)
	})
	t.Run("lock_check_error_fails_closed", func(t *testing.T) {
		e := newRunnerEnv()
		e.locks.err = errors.New("db down")
		res, err := e.runner.Run(ctx, "202609")
		require.Error(t, err)
		assert.False(t, res.Skipped)
		assert.Empty(t, e.pub.calls)
	})
	t.Run("unwired_fails_closed", func(t *testing.T) {
		_, err := NewScheduleRunner(nil, nil, nil).Run(ctx, "202609")
		require.ErrorIs(t, err, ErrScheduleNotConfigured)
	})
}

type memScheduleRepo struct {
	row   *domain.ScheduleSetting
	err   error
	saves int
}

func (m *memScheduleRepo) GetSchedule(context.Context) (domain.ScheduleSetting, error) {
	if m.err != nil {
		return domain.ScheduleSetting{}, m.err
	}
	if m.row == nil {
		return domain.ScheduleSetting{}, domain.ErrScheduleSettingNotFound
	}
	return *m.row, nil
}

func (m *memScheduleRepo) SaveSchedule(_ context.Context, s domain.ScheduleSetting, actor string) (domain.ScheduleSetting, error) {
	m.saves++
	s.UpdatedBy = actor
	m.row = &s
	return s, nil
}

func TestErpScheduler_FailClosedAndReload(t *testing.T) {
	ctx := context.Background()
	repo := &memScheduleRepo{}
	cfg := ScheduleConfig{Enabled: true, Cron: "0 0 2 5 * *", Timezone: "Asia/Jakarta"}
	s := NewErpScheduler(cfg, repo, newRunnerEnv().runner)
	defer s.Stop()

	r := s.Start(ctx)
	assert.True(t, r.Enabled)
	assert.Equal(t, ScheduleSourceConfigCron, r.Source)

	repo.row = &domain.ScheduleSetting{Enabled: true, Mode: domain.ScheduleCron, Cron: "bad"}
	r = s.Reload(ctx)
	assert.False(t, r.Enabled, "invalid row -> off")
	assert.NotEmpty(t, r.Errors)

	repo.err = errors.New("pg down")
	r = s.Reload(ctx)
	assert.False(t, r.Enabled, "unreadable row -> off")

	off := NewErpScheduler(ScheduleConfig{Cron: "0 0 2 5 * *"}, nil, nil)
	assert.False(t, off.Start(ctx).Enabled, "master switch off by default")
	off.Stop()
}

func TestErpScheduler_FiresRunner(t *testing.T) {
	e := newRunnerEnv()
	loc, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err)
	// A SPECIFIC_DATE two hours ahead; the fake clock starts 50ms before it
	// and moves past it after the first read, so the one-shot fires once.
	fireAt := time.Now().In(loc).Add(2 * time.Hour).Truncate(time.Minute)
	repo := &memScheduleRepo{row: &domain.ScheduleSetting{
		Enabled: true, Mode: domain.ScheduleSpecificDate,
		RunDate: fireAt.Format(time.DateOnly), RunTime: fireAt.Format("15:04"), Timezone: "Asia/Jakarta",
	}}
	s := NewErpScheduler(ScheduleConfig{Enabled: true}, repo, e.runner)
	defer s.Stop()
	var reads atomic.Int64
	s.now = func() time.Time {
		if reads.Add(1) == 1 {
			return fireAt.Add(-50 * time.Millisecond)
		}
		return fireAt.Add(time.Second)
	}
	r := s.Start(context.Background())
	require.True(t, r.Enabled, "%v", r.Errors)
	require.Eventually(t, func() bool { return e.runner.RunCount() == 1 }, 3*time.Second, 20*time.Millisecond)
	assert.Equal(t, []string{StepLoadDemand}, e.subtypes())
	assert.Equal(t, int64(0), e.writer.calls.Load())
	for _, ex := range e.jobs.execs {
		assert.Equal(t, s.Current().TargetPeriodFor(fireAt.Add(time.Second)), ex.Period())
	}
}
