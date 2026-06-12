// Package fillnotifier wires the costfillassignment.Notifier seam to the
// costnotification.Emitter so that fill-task events produce persisted
// in-app notifications.
package fillnotifier

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	appfill "github.com/mutugading/goapps-backend/services/finance/internal/application/costfillassignment"
	costnotif "github.com/mutugading/goapps-backend/services/finance/internal/application/costnotification"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
	notifDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costnotification"
)

// Impl implements application/costfillassignment.Notifier using the costnotification emitter.
type Impl struct {
	taskRepo domain.TaskRepository
	emitter  *costnotif.Emitter
}

// New constructs the notifier implementation.
func New(taskRepo domain.TaskRepository, emitter *costnotif.Emitter) *Impl {
	return &Impl{taskRepo: taskRepo, emitter: emitter}
}

var _ appfill.Notifier = (*Impl)(nil)

// NotifyOverdue sends an SLA overdue notification to the task's assigned filler.
func (n *Impl) NotifyOverdue(ctx context.Context, taskID int64) error {
	task, err := n.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task for sla notify: %w", err)
	}
	if task.FillerType != "USER" {
		// Department-assigned tasks: skip individual notify (no single user to address).
		log.Info().Int64("task_id", taskID).Str("filler_type", task.FillerType).
			Msg("sla overdue: dept-assigned task, skipping individual notify")
		return nil
	}
	payload := fmt.Sprintf(
		`{"taskId":%d,"requestId":%d,"routeLevel":%d}`,
		taskID, task.RequestID, task.RouteLevel,
	)
	_, err = n.emitter.Emit(ctx, notifDomain.NewInput{
		RecipientUserID: task.FillerValue,
		TriggerType:     notifDomain.TriggerSLAOverdue,
		RequestID:       task.RequestID,
		Payload:         payload,
	})
	if err != nil {
		return fmt.Errorf("emit sla overdue for task %d: %w", taskID, err)
	}
	return nil
}
