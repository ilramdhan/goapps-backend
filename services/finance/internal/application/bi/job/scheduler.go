package job

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"

	jobdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/job"
)

// log key constants for structured zerolog fields.
const (
	logKeyJobID   = "job_id"
	logKeyJobName = "job_name"
)

// CronTriggerer is the minimal interface the scheduler needs from TriggerHandler.
// Using an interface makes BiJobScheduler testable with a stub.
type CronTriggerer interface {
	CronTrigger(ctx context.Context, jobID uuid.UUID) (*jobdomain.Log, error)
}

// BiJobScheduler reads active bi_job rows that have a schedule_cron and fires
// them automatically via CronTrigger. It re-syncs the job list from the DB
// every syncInterval to pick up admin changes without a service restart.
//
// Overlap protection: if a job's previous run is still in-flight when the cron
// tick fires, the tick is skipped and a warning is logged. This prevents
// runaway accumulation when a job (e.g. ETL) takes longer than its interval.
type BiJobScheduler struct {
	repo         jobdomain.Repository
	trigger      CronTriggerer
	log          zerolog.Logger
	c            *cron.Cron
	entryIDs     map[string]cron.EntryID // jobID string -> cron entry ID
	entryCrons   map[string]string       // jobID string -> registered cron expression
	mu           sync.Mutex
	running      sync.Map // jobID string -> struct{} sentinel
	syncInterval time.Duration
}

// NewBiJobScheduler constructs a scheduler. syncInterval controls how often the
// job list is refreshed from the DB (recommended: 5 * time.Minute).
func NewBiJobScheduler(
	repo jobdomain.Repository,
	trigger CronTriggerer,
	logger zerolog.Logger,
	syncInterval time.Duration,
) *BiJobScheduler {
	return &BiJobScheduler{
		repo:         repo,
		trigger:      trigger,
		log:          logger.With().Str("component", "bi_job_scheduler").Logger(),
		c:            cron.New(),
		entryIDs:     make(map[string]cron.EntryID),
		entryCrons:   make(map[string]string),
		syncInterval: syncInterval,
	}
}

// Start loads all schedulable jobs from the DB, registers their cron entries,
// starts the cron engine, and blocks until ctx is cancelled.
// Call in a goroutine: go scheduler.Start(ctx).
func (s *BiJobScheduler) Start(ctx context.Context) {
	s.log.Info().Msg("BI job scheduler starting.")

	if err := s.syncJobs(ctx); err != nil {
		s.log.Error().Err(err).Msg("initial job schedule sync failed - will retry at next tick.")
	}

	s.c.Start()
	s.log.Info().Int("jobs_registered", len(s.entryIDs)).Msg("cron engine started.")

	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.log.Info().Msg("BI job scheduler stopping.")
			<-s.c.Stop().Done()
			s.log.Info().Msg("BI job scheduler stopped.")
			return
		case <-ticker.C:
			if err := s.syncJobs(ctx); err != nil {
				s.log.Error().Err(err).Msg("job schedule sync failed - retrying at next tick.")
			}
		}
	}
}

// syncJobs fetches all active jobs with a non-empty schedule_cron and reconciles
// the cron engine: adds new entries, removes stale ones.
// Locking ensures safe concurrent access between the sync ticker and cron goroutines.
func (s *BiJobScheduler) syncJobs(ctx context.Context) error {
	jobs, err := s.repo.List(ctx, false) // false = active only
	if err != nil {
		return err
	}

	// Build a set of schedulable job IDs from the DB.
	wanted := make(map[string]*jobdomain.Job, len(jobs))
	for _, j := range jobs {
		if j.ScheduleCron != "" {
			wanted[j.ID.String()] = j
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove entries for jobs no longer schedulable (deleted, deactivated, cron cleared).
	for idStr, entryID := range s.entryIDs {
		if _, ok := wanted[idStr]; !ok {
			s.c.Remove(entryID)
			delete(s.entryIDs, idStr)
			delete(s.entryCrons, idStr)
			s.log.Info().Str(logKeyJobID, idStr).Msg("unregistered cron entry (job removed or deactivated).")
		}
	}

	// Register new jobs, or re-register if the cron expression changed.
	// When schedule_cron is updated in the admin panel and the next 5-min sync
	// runs, the old entry is removed and a new one with the updated expression
	// is registered — no deactivate/reactivate cycle required.
	for idStr, j := range wanted {
		if existingCron, exists := s.entryCrons[idStr]; exists {
			if existingCron == j.ScheduleCron {
				continue // same cron, nothing to do
			}
			// Cron expression changed — remove old entry before re-registering.
			s.c.Remove(s.entryIDs[idStr])
			delete(s.entryIDs, idStr)
			delete(s.entryCrons, idStr)
			s.log.Info().
				Str(logKeyJobID, idStr).
				Str(logKeyJobName, j.Name).
				Str("old_cron", existingCron).
				Str("new_cron", j.ScheduleCron).
				Msg("cron expression changed — re-registering entry.")
		}

		// Capture loop variables for the closure.
		jobID := j.ID
		expr := j.ScheduleCron
		jobName := j.Name

		entryID, addErr := s.c.AddFunc(expr, func() {
			s.fire(ctx, jobID, jobName)
		})
		if addErr != nil {
			s.log.Error().Err(addErr).
				Str(logKeyJobID, idStr).
				Str(logKeyJobName, jobName).
				Str("cron", expr).
				Msg("failed to register cron entry - invalid expression.")
			continue
		}

		s.entryIDs[idStr] = entryID
		s.entryCrons[idStr] = expr
		s.log.Info().
			Str(logKeyJobID, idStr).
			Str(logKeyJobName, jobName).
			Str("cron", expr).
			Msg("registered cron entry.")
	}

	return nil
}

// fire is called by robfig/cron on each tick. It guards against overlapping
// runs of the same job and dispatches CronTrigger in a separate goroutine.
func (s *BiJobScheduler) fire(ctx context.Context, jobID uuid.UUID, jobName string) {
	idStr := jobID.String()

	// Overlap guard: skip this tick if the previous run is still in-flight.
	if _, loaded := s.running.LoadOrStore(idStr, struct{}{}); loaded {
		s.log.Warn().
			Str(logKeyJobID, idStr).
			Str(logKeyJobName, jobName).
			Msg("cron tick skipped - previous run still in progress.")
		return
	}

	go func() {
		defer s.running.Delete(idStr)

		s.log.Info().
			Str(logKeyJobID, idStr).
			Str(logKeyJobName, jobName).
			Msg("cron job started.")

		result, err := s.trigger.CronTrigger(ctx, jobID)
		if err != nil {
			s.log.Error().Err(err).
				Str(logKeyJobID, idStr).
				Str(logKeyJobName, jobName).
				Msg("cron job dispatch error.")
			return
		}

		s.log.Info().
			Str(logKeyJobID, idStr).
			Str(logKeyJobName, jobName).
			Str("status", result.Status).
			Int("rows_affected", result.RowsAffected).
			Int("duration_ms", result.DurationMs).
			Msg("cron job completed.")
	}()
}
