package fillnotifier

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	appfill "github.com/mutugading/goapps-backend/services/finance/internal/application/costfillassignment"
	costnotif "github.com/mutugading/goapps-backend/services/finance/internal/application/costnotification"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
	notifDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costnotification"
)

// ReminderJob runs periodic scans for fill tasks that need reminder notifications:
//
//   - PENDING_FILL     — ACTIVE/FILLING tasks whose filler has not yet submitted.
//   - PENDING_APPROVAL — APPROVAL_PENDING tasks whose approver has not yet decided.
//
// It is intended to be called from a cron scheduler (e.g. every hour).
type ReminderJob struct {
	taskRepo         domain.TaskRepository
	emitter          *costnotif.Emitter
	fillNotifier     appfill.FillEventNotifier // preferred; supports USER + DEPT
	reminderGapHours int
}

// NewReminderJob constructs the job.
// reminderGapHours is the minimum gap between repeated reminders for the same task.
func NewReminderJob(taskRepo domain.TaskRepository, emitter *costnotif.Emitter, reminderGapHours int) *ReminderJob {
	if reminderGapHours <= 0 {
		reminderGapHours = 4
	}
	return &ReminderJob{taskRepo: taskRepo, emitter: emitter, reminderGapHours: reminderGapHours}
}

// WithFillNotifier attaches a FillEventNotifier. Returns receiver for chaining.
func (j *ReminderJob) WithFillNotifier(fn appfill.FillEventNotifier) *ReminderJob {
	j.fillNotifier = fn
	return j
}

// Run is the cron entry point. It scans all pending-fill and pending-approval tasks
// and emits a reminder notification to each responsible party.
func (j *ReminderJob) Run() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	j.runPendingFill(ctx)
	j.runPendingApproval(ctx)
}

func (j *ReminderJob) runPendingFill(ctx context.Context) {
	tasks, err := j.taskRepo.ListPendingFill(ctx, j.reminderGapHours)
	if err != nil {
		log.Error().Err(err).Msg("reminder_job: list pending fill failed")
		return
	}
	for _, t := range tasks {
		if notifyErr := j.notifyPendingFill(ctx, t); notifyErr != nil {
			log.Warn().Err(notifyErr).Int64("task_id", t.TaskID).Msg("reminder_job: pending fill notify failed")
			continue
		}
		if markErr := j.taskRepo.MarkNotified(ctx, t.TaskID); markErr != nil {
			log.Warn().Err(markErr).Int64("task_id", t.TaskID).Msg("reminder_job: mark notified failed")
		}
	}
	if len(tasks) > 0 {
		log.Info().Int("count", len(tasks)).Msg("reminder_job: pending fill reminders sent")
	}
}

func (j *ReminderJob) runPendingApproval(ctx context.Context) {
	tasks, err := j.taskRepo.ListPendingApproval(ctx, j.reminderGapHours)
	if err != nil {
		log.Error().Err(err).Msg("reminder_job: list pending approval failed")
		return
	}
	for _, t := range tasks {
		if notifyErr := j.notifyPendingApproval(ctx, t); notifyErr != nil {
			log.Warn().Err(notifyErr).Int64("task_id", t.TaskID).Msg("reminder_job: pending approval notify failed")
			continue
		}
		if markErr := j.taskRepo.MarkNotified(ctx, t.TaskID); markErr != nil {
			log.Warn().Err(markErr).Int64("task_id", t.TaskID).Msg("reminder_job: mark notified failed")
		}
	}
	if len(tasks) > 0 {
		log.Info().Int("count", len(tasks)).Msg("reminder_job: pending approval reminders sent")
	}
}

func (j *ReminderJob) notifyPendingFill(ctx context.Context, t *domain.Task) error {
	if j.fillNotifier != nil {
		return j.fillNotifier.NotifyReminderFill(ctx, t)
	}
	if t.FillerType != domain.ActorUser {
		// Department-assigned: no single user to notify (legacy path only).
		return nil
	}
	payload := fmt.Sprintf(
		`{"taskId":%d,"requestId":%d,"routeLevel":%d,"status":%q}`,
		t.TaskID, t.RequestID, t.RouteLevel, t.Status(),
	)
	_, err := j.emitter.Emit(ctx, notifDomain.NewInput{
		RecipientUserID: t.FillerValue,
		TriggerType:     notifDomain.TriggerPendingFill,
		RequestID:       t.RequestID,
		Payload:         payload,
	})
	return err
}

func (j *ReminderJob) notifyPendingApproval(ctx context.Context, t *domain.Task) error {
	if j.fillNotifier != nil {
		return j.fillNotifier.NotifyReminderApproval(ctx, t)
	}
	if t.ApproverType != domain.ActorUser {
		// Department approver: no single user to notify (legacy path only).
		return nil
	}
	payload := fmt.Sprintf(
		`{"taskId":%d,"requestId":%d,"routeLevel":%d}`,
		t.TaskID, t.RequestID, t.RouteLevel,
	)
	_, err := j.emitter.Emit(ctx, notifDomain.NewInput{
		RecipientUserID: t.ApproverValue,
		TriggerType:     notifDomain.TriggerPendingApproval,
		RequestID:       t.RequestID,
		Payload:         payload,
	})
	return err
}
