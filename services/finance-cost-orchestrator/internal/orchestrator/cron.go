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

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

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
	if err := s.triggerJob(ctx, period); err != nil {
		log.Error().Err(err).Str("period", period).Msg("cron auto-trigger failed")
		return
	}
	log.Info().Str("period", period).Msg("cron auto-trigger published")
}

func (s *CronScheduler) triggerJob(ctx context.Context, period string) error {
	jobID, err := s.jobRepo.CreateAutoJob(ctx, period, "ACTUAL", "ALL", "CRON", "system")
	if err != nil {
		return fmt.Errorf("create cal_job: %w", err)
	}
	if err := s.pub.PublishJobTriggered(ctx, jobID); err != nil {
		return fmt.Errorf("publish job_triggered: %w", err)
	}
	return nil
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
