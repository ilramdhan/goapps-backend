package costfillassignment

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// SLANotifierJob checks for overdue fill tasks and triggers notifications.
type SLANotifierJob struct {
	repo             domain.TaskRepository
	notifier         Notifier
	reminderGapHours int
}

// NewSLANotifierJob constructs the SLA notifier.
// reminderGapHours: minimum hours between reminders for the same task.
func NewSLANotifierJob(repo domain.TaskRepository, notifier Notifier, reminderGapHours int) *SLANotifierJob {
	if reminderGapHours <= 0 {
		reminderGapHours = 4
	}
	return &SLANotifierJob{
		repo:             repo,
		notifier:         notifier,
		reminderGapHours: reminderGapHours,
	}
}

// Run finds all overdue tasks and sends notifications. Safe to call from a cron scheduler.
func (j *SLANotifierJob) Run() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	tasks, err := j.repo.ListOverdue(ctx, j.reminderGapHours)
	if err != nil {
		log.Error().Err(err).Msg("sla_notifier: list overdue tasks failed")
		return
	}
	if len(tasks) == 0 {
		return
	}
	log.Info().Int("count", len(tasks)).Msg("sla_notifier: processing overdue tasks")

	for _, t := range tasks {
		if notifyErr := j.notifier.NotifyOverdue(ctx, t.TaskID); notifyErr != nil {
			log.Warn().Err(notifyErr).Int64("task_id", t.TaskID).Msg("sla_notifier: notify failed")
			continue
		}
		if markErr := j.repo.MarkNotified(ctx, t.TaskID); markErr != nil {
			log.Warn().Err(markErr).Int64("task_id", t.TaskID).Msg("sla_notifier: mark notified failed")
		}
	}
	log.Info().Int("count", len(tasks)).Msg("sla_notifier: done")
}

// RunWithError is a testable version that returns the first notification error encountered.
func (j *SLANotifierJob) RunWithError(ctx context.Context) error {
	tasks, err := j.repo.ListOverdue(ctx, j.reminderGapHours)
	if err != nil {
		return fmt.Errorf("list overdue: %w", err)
	}
	for _, t := range tasks {
		if notifyErr := j.notifier.NotifyOverdue(ctx, t.TaskID); notifyErr != nil {
			return fmt.Errorf("notify task %d: %w", t.TaskID, notifyErr)
		}
		if markErr := j.repo.MarkNotified(ctx, t.TaskID); markErr != nil {
			return fmt.Errorf("mark notified task %d: %w", t.TaskID, markErr)
		}
	}
	return nil
}
