package erpintegration

// schedule_runner.go is the scheduled monthly ERP run (plan-06 P5-T11 steps
// 3-4; design Part 2 §9.3, §434; User decision 2026-09-29 U-2).
//
// A fire creates a LIVE batch for the target period and enqueues ONLY the
// read/compute chain LoadDemand -> Coverage -> Derive -> Validate. It stops
// at VALIDATED. Push, valuation, ADJ approve and restore are never enqueued
// (they need the human gates G3-G5); the runner has no writer and no path to
// TriggerPush.
//
// Steps are separate erp_integration jobs and only one may be active per
// period, so a fire enqueues load_demand; ContinueAfter enqueues the next
// read/compute step once the previous scheduled job has finished (called by
// the job executor after it marks the job SUCCESS).
//
// Skips (logged at INFO, counted, never retried — N-6):
//   - SkipNotLocked: (P, ACTUAL) is not cost-locked; a LIVE batch needs the
//     lock (G10), so there is nothing to compute yet.
//   - SkipPosted: the period has posted ADJ heads (V-10).
//   - SkipPushedOrLater: a LIVE batch is already PUSHED, VALUATED,
//     RECONCILED or LOCKED (ERP-frozen) for the period.
//   - SkipInFlight: a LIVE batch DRAFT..VALIDATED already exists; a human
//     is driving it.
//   - SkipActiveJob: an erp_integration job is already queued/processing.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// ScheduledRunActor is the created_by of scheduled batches and jobs. The
// executor uses it (IsScheduledRun) to decide whether to chain the next step.
const ScheduledRunActor = "system:erp_schedule"

// Scheduled-run skip reasons.
const (
	SkipNotLocked     = "not_locked"
	SkipPosted        = "posted"
	SkipPushedOrLater = "pushed_or_later"
	SkipInFlight      = "in_flight"
	SkipActiveJob     = "active_job"
)

// scheduledChain is the ordered read/compute chain. Push is deliberately
// absent: nothing after validate is ever scheduled.
var scheduledChain = []string{StepLoadDemand, StepCoverage, StepDerive, StepValidate}

// NextScheduledStep returns the step that follows step in the scheduled
// chain, or false after validate (and for any step outside the chain).
func NextScheduledStep(step string) (string, bool) {
	for i, s := range scheduledChain {
		if s == step && i+1 < len(scheduledChain) {
			return scheduledChain[i+1], true
		}
	}
	return "", false
}

// IsScheduledRun reports whether exec was enqueued by the scheduler.
func IsScheduledRun(exec *job.Execution) bool {
	return exec != nil && exec.JobType() == job.TypeErpIntegration && exec.CreatedBy() == ScheduledRunActor
}

// ScheduledRunResult reports one fire.
type ScheduledRunResult struct {
	Period     string
	Skipped    bool
	SkipReason string
	BatchID    int64
	JobID      string
}

// batchCreator is the CreateBatchHandler surface.
type batchCreator interface {
	Handle(ctx context.Context, cmd CreateBatchCommand) (*domain.Batch, error)
}

// stepEnqueuer is the StepTriggerHandler surface. Only Handle (never
// TriggerPush) is reachable, and Handle itself refuses push.
type stepEnqueuer interface {
	Handle(ctx context.Context, cmd StepTriggerCommand) (*job.Execution, error)
}

// ScheduleRunner runs the scheduled read/compute chain.
type ScheduleRunner struct {
	batches domain.BatchRepository
	create  batchCreator
	steps   stepEnqueuer

	mu    sync.Mutex
	skips map[string]int64
	runs  int64
}

// NewScheduleRunner builds the runner from the batch repository, the
// CreateBatch handler (its LIVE guards: lock, posted, in-flight) and the
// step trigger handler.
func NewScheduleRunner(batches domain.BatchRepository, create *CreateBatchHandler, steps *StepTriggerHandler) *ScheduleRunner {
	r := &ScheduleRunner{batches: batches, skips: map[string]int64{}}
	if create != nil {
		r.create = create
	}
	if steps != nil {
		r.steps = steps
	}
	return r
}

// SkipCounts returns a copy of the skip counters by reason.
func (r *ScheduleRunner) SkipCounts() map[string]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int64, len(r.skips))
	for k, v := range r.skips {
		out[k] = v
	}
	return out
}

// RunCount returns how many fires enqueued a chain.
func (r *ScheduleRunner) RunCount() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runs
}

// Run performs one scheduled fire for period: skip checks, a LIVE batch,
// then the load_demand job. A skip returns Skipped=true and a nil error.
func (r *ScheduleRunner) Run(ctx context.Context, period string) (ScheduledRunResult, error) {
	res := ScheduledRunResult{Period: period}
	if r.batches == nil || r.create == nil || r.steps == nil {
		return res, fmt.Errorf("%w: scheduled run not wired", ErrScheduleNotConfigured)
	}
	if reason, err := r.frozenReason(ctx, period); err != nil {
		return res, err
	} else if reason != "" {
		return r.skip(res, reason), nil
	}
	b, err := r.create.Handle(ctx, CreateBatchCommand{Period: period, Mode: domain.ModeLive, Actor: ScheduledRunActor})
	if err != nil {
		if reason := skipReasonFor(err); reason != "" {
			return r.skip(res, reason), nil
		}
		return res, fmt.Errorf("scheduled run create batch %s: %w", period, err)
	}
	res.BatchID = b.ID()
	exec, err := r.steps.Handle(ctx, StepTriggerCommand{BatchID: b.ID(), Step: StepLoadDemand, Actor: ScheduledRunActor})
	if err != nil {
		if reason := skipReasonFor(err); reason != "" {
			return r.skip(res, reason), nil
		}
		return res, fmt.Errorf("scheduled run enqueue load_demand batch %d: %w", b.ID(), err)
	}
	res.JobID = exec.ID().String()
	r.mu.Lock()
	r.runs++
	r.mu.Unlock()
	log.Info().Str("period", period).Int64("batch_id", b.ID()).Str("job_id", res.JobID).
		Msg("erp scheduled run: load_demand enqueued")
	return res, nil
}

// ContinueAfter enqueues the step that follows completed for a scheduled
// batch. It returns ("", nil) after validate: the chain stops at VALIDATED.
// Call it only after the completed job is marked SUCCESS (one active
// erp_integration job per period).
func (r *ScheduleRunner) ContinueAfter(ctx context.Context, batchID int64, completed string) (string, error) {
	next, ok := NextScheduledStep(completed)
	if !ok {
		return "", nil
	}
	if r.steps == nil {
		return "", fmt.Errorf("%w: scheduled run not wired", ErrScheduleNotConfigured)
	}
	exec, err := r.steps.Handle(ctx, StepTriggerCommand{BatchID: batchID, Step: next, Actor: ScheduledRunActor})
	if err != nil {
		return "", fmt.Errorf("scheduled run enqueue %s batch %d: %w", next, batchID, err)
	}
	log.Info().Int64("batch_id", batchID).Str("step", next).Str("job_id", exec.ID().String()).
		Msg("erp scheduled run: next step enqueued")
	return next, nil
}

// frozenReason reports SkipPushedOrLater when a LIVE batch of the period is
// PUSHED or later. ListByPeriod is used so PUSHED (in-flight) is caught too.
func (r *ScheduleRunner) frozenReason(ctx context.Context, period string) (string, error) {
	list, err := r.batches.ListByPeriod(ctx, period)
	if err != nil {
		return "", fmt.Errorf("scheduled run list batches %s: %w", period, err)
	}
	for _, b := range list {
		if b.Mode() != domain.ModeLive {
			continue
		}
		if b.Status() == domain.StatusPushed || b.Status().IsActive() {
			return SkipPushedOrLater, nil
		}
	}
	return "", nil
}

func skipReasonFor(err error) string {
	switch {
	case errors.Is(err, domain.ErrPeriodNotLocked):
		return SkipNotLocked
	case errors.Is(err, domain.ErrPeriodPosted):
		return SkipPosted
	case errors.Is(err, domain.ErrActiveBatchExists):
		return SkipInFlight
	case errors.Is(err, job.ErrDuplicateActiveJob):
		return SkipActiveJob
	default:
		return ""
	}
}

func (r *ScheduleRunner) skip(res ScheduledRunResult, reason string) ScheduledRunResult {
	r.mu.Lock()
	r.skips[reason]++
	r.mu.Unlock()
	log.Info().Str("period", res.Period).Str("reason", reason).Msg("erp scheduled run skipped")
	res.Skipped, res.SkipReason = true, reason
	return res
}

// ErpScheduler fires the ScheduleRunner on the resolved schedule. It is
// meant for the finance worker (a singleton deployment); duplicate fires on
// several replicas are harmless because uix_ceib_inflight and the active-job
// index let only one batch and one job through.
type ErpScheduler struct {
	cfg    ScheduleConfig
	repo   domain.ScheduleSettingRepository
	runner *ScheduleRunner
	now    func() time.Time

	mu      sync.Mutex
	current ResolvedSchedule
	timer   *time.Timer
	stopped bool
	wg      sync.WaitGroup
}

// NewErpScheduler builds the scheduler; repo may be nil (config only).
func NewErpScheduler(cfg ScheduleConfig, repo domain.ScheduleSettingRepository, runner *ScheduleRunner) *ErpScheduler {
	return &ErpScheduler{cfg: cfg, repo: repo, runner: runner, now: time.Now}
}

// Start resolves the schedule and arms the next fire. It never fails: an
// invalid or disabled schedule logs and leaves the scheduler off.
func (s *ErpScheduler) Start(ctx context.Context) ResolvedSchedule {
	return s.Reload(ctx)
}

// Current returns the schedule in force.
func (s *ErpScheduler) Current() ResolvedSchedule {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// Reload re-resolves the schedule (after UpdateErpIntegrationSchedule) and
// re-arms the timer. It satisfies ScheduleReloader.
func (s *ErpScheduler) Reload(ctx context.Context) ResolvedSchedule {
	var setting *domain.ScheduleSetting
	if s.repo != nil {
		st, err := loadScheduleSetting(ctx, s.repo)
		if err != nil {
			log.Warn().Err(err).Msg("erp schedule: settings row unreadable; scheduler off (fail closed)")
			s.arm(ResolvedSchedule{Source: ScheduleSourceNone, Errors: []string{err.Error()}})
			return s.Current()
		}
		setting = st
	}
	res := ResolveSchedule(s.cfg, setting)
	switch {
	case len(res.Errors) > 0:
		log.Warn().Strs("errors", res.Errors).Str("source", res.Source).
			Msg("erp schedule invalid; scheduler off (fail closed)")
	case !res.Enabled:
		log.Info().Str("source", res.Source).Msg("erp schedule disabled")
	}
	s.arm(res)
	return s.Current()
}

// Stop cancels the timer and waits for an in-flight fire.
func (s *ErpScheduler) Stop() {
	s.mu.Lock()
	s.stopped = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *ErpScheduler) arm(res ResolvedSchedule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = res
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.stopped || !res.Enabled {
		return
	}
	now := s.now()
	next := res.Next(now)
	if next.IsZero() {
		log.Info().Str("source", res.Source).Msg("erp schedule has no future run")
		return
	}
	log.Info().Str("source", res.Source).Str("spec", res.Spec).Time("next_run", next).Msg("erp schedule armed")
	s.timer = time.AfterFunc(next.Sub(now), func() { s.fire(res) })
}

func (s *ErpScheduler) fire(res ResolvedSchedule) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	period := res.TargetPeriodFor(s.now())
	if s.runner == nil {
		log.Error().Str("period", period).Msg("erp scheduled run: runner not wired")
	} else if _, err := s.runner.Run(ctx, period); err != nil {
		log.Error().Err(err).Str("period", period).Msg("erp scheduled run failed")
	}
	// Re-arm from the same schedule (a Reload in between replaced it).
	s.mu.Lock()
	same := s.current.Source == res.Source && s.current.Spec == res.Spec && s.current.Timezone == res.Timezone
	s.mu.Unlock()
	if same {
		s.arm(res)
	}
}
