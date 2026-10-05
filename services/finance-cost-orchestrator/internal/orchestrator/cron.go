// Package orchestrator cron auto-trigger for the monthly ALL-scope calc job.
//
// S8e.6 of the Phase C Calc Engine plan: on the 5th day of each month at
// 02:00 Asia/Jakarta, insert a QUEUED cal_job row for the previous month and
// publish a JobTriggeredEvent so the existing coordinator picks it up.
package orchestrator

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

// cronCalcType / cronScope are the fixed shape of the monthly auto-trigger.
const (
	cronCalcType = "ACTUAL"
	cronScope    = "ALL"
)

// skippedLockedTotal counts cron fires skipped because (period, ACTUAL) is
// locked in cst_period_lock (design §5.5, N-6; plan-02 P1-T5).
var skippedLockedTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "orchestrator_skipped_locked_total",
	Help: "Cron auto-trigger fires skipped because the target ACTUAL period is locked.",
})

// rmqJobPublisher is the small surface the cron needs from the rmq package.
type rmqJobPublisher interface {
	PublishJobTriggered(ctx context.Context, jobID int64) error
}

// CronScheduler runs the monthly auto-trigger.
type CronScheduler struct {
	jobRepo *JobRepo
	pub     rmqJobPublisher
	cronExp string
	tz      *time.Location
	cron    *cron.Cron
}

// NewCronScheduler constructs the scheduler. cronExp default is the 6-field
// expression "0 0 2 5 * *" (second minute hour day month dow) — tanggal 5 at
// 02:00 in the given timezone.
func NewCronScheduler(db *sql.DB, pub rmqJobPublisher, cronExp string, tz string) (*CronScheduler, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", tz, err)
	}
	c := cron.New(cron.WithLocation(loc), cron.WithSeconds())
	return &CronScheduler{
		jobRepo: NewJobRepo(db),
		pub:     pub,
		cronExp: cronExp,
		tz:      loc,
		cron:    c,
	}, nil
}

// Start registers the cron entry and runs the scheduler. Returns the next
// scheduled fire time so the caller can log it. Non-blocking.
func (s *CronScheduler) Start() (time.Time, error) {
	entryID, err := s.cron.AddFunc(s.cronExp, s.fire)
	if err != nil {
		return time.Time{}, fmt.Errorf("add cron entry %q: %w", s.cronExp, err)
	}
	s.cron.Start()
	next := s.cron.Entry(entryID).Next
	return next, nil
}

// Stop gracefully halts the scheduler. Blocks until in-flight fires complete.
func (s *CronScheduler) Stop() {
	if s.cron != nil {
		stopCtx := s.cron.Stop()
		<-stopCtx.Done()
	}
}

// fire is the cron tick callback. Inserts a QUEUED cal_job for the previous
// month and publishes the JobTriggeredEvent.
func (s *CronScheduler) fire() {
	now := time.Now().In(s.tz)
	period := previousPeriodYYYYMM(now)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	published, err := s.triggerJob(ctx, period)
	if err != nil {
		log.Error().Err(err).Str("period", period).Msg("cron auto-trigger failed")
		return
	}
	if published {
		log.Info().Str("period", period).Msg("cron auto-trigger published")
	}
}

// triggerJob creates and publishes the monthly ACTUAL job. A locked period is
// skipped: logged at INFO, counted in orchestrator_skipped_locked_total, and
// reported as published=false with a nil error so nothing retries (N-6). A lock-check error fails closed
// (no job, error logged by fire); with no lock row the job is created exactly
// as before.
func (s *CronScheduler) triggerJob(ctx context.Context, period string) (bool, error) {
	locked, err := s.jobRepo.IsPeriodLocked(ctx, period, cronCalcType)
	if err != nil {
		return false, fmt.Errorf("check period lock: %w", err)
	}
	if locked {
		skippedLockedTotal.Inc()
		log.Info().Str("period", period).Str("calc_type", cronCalcType).Str("scope", cronScope).
			Msg("cron auto-trigger skipped: period locked")
		return false, nil
	}
	jobID, err := s.jobRepo.CreateAutoJob(ctx, period, cronCalcType, cronScope, "CRON", "system")
	if err != nil {
		return false, fmt.Errorf("create cal_job: %w", err)
	}
	if err := s.pub.PublishJobTriggered(ctx, jobID); err != nil {
		return false, fmt.Errorf("publish job_triggered: %w", err)
	}
	return true, nil
}

// previousPeriodYYYYMM returns YYYYMM for the month BEFORE now.
// Examples (Asia/Jakarta): now=2026-05-05 02:00 → "202604";
// now=2026-01-05 → "202512".
func previousPeriodYYYYMM(now time.Time) string {
	y, m, _ := now.Date()
	pm := m - 1
	if pm < time.January {
		pm = time.December
		y--
	}
	return fmt.Sprintf("%04d%02d", y, int(pm))
}
