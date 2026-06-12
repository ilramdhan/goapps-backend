package costfillassignment

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// CreateAllTasksHandler implements FillTaskCreator — resolves configs for all route
// levels and bulk-inserts Task rows in a single call.
type CreateAllTasksHandler struct {
	configRepo domain.ConfigRepository
	taskRepo   domain.TaskRepository
	notifier   FillEventNotifier // optional; fires NotifyTaskActivated after insert
}

// NewCreateAllTasksHandler constructs the handler.
func NewCreateAllTasksHandler(configRepo domain.ConfigRepository, taskRepo domain.TaskRepository) *CreateAllTasksHandler {
	return &CreateAllTasksHandler{configRepo: configRepo, taskRepo: taskRepo}
}

// WithNotifier attaches a fill-event notifier so that fillers are notified when
// tasks first become ACTIVE (i.e., immediately on creation for regular levels).
func (h *CreateAllTasksHandler) WithNotifier(n FillEventNotifier) *CreateAllTasksHandler {
	h.notifier = n
	return h
}

// CreateForRequest resolves config for each level, builds Task objects, bulk-inserts them,
// and fires NotifyTaskActivated for each task when a notifier is configured.
// Returns domain.ErrConfigNotFound (wrapped) if any level has no active global config.
func (h *CreateAllTasksHandler) CreateForRequest(ctx context.Context, requestID, productSysID, routeHeadID int64, routeLevels []int32, perLevelTotals map[int32]int32, requestNo string) error {
	resolver := NewConfigResolverAdapter(h.configRepo)
	tasks := make([]*domain.Task, 0, len(routeLevels))
	for _, level := range routeLevels {
		rc, err := resolver.Resolve(ctx, productSysID, requestID, level)
		if err != nil {
			return fmt.Errorf("resolve config for level %d: %w", level, err)
		}
		tasks = append(tasks, domain.NewTask(requestID, routeHeadID, rc, perLevelTotals[level]))
	}
	if err := h.taskRepo.BulkInsert(ctx, tasks); err != nil {
		return fmt.Errorf("bulk insert tasks for request %d: %w", requestID, err)
	}
	if h.notifier != nil {
		for _, t := range tasks {
			if notifyErr := h.notifier.NotifyTaskActivated(ctx, t, requestNo); notifyErr != nil {
				log.Warn().Err(notifyErr).Int64("task_id", t.TaskID).
					Msg("CreateAllTasksHandler: NotifyTaskActivated failed (non-fatal)")
			}
		}
	}
	return nil
}

var _ FillTaskCreator = (*CreateAllTasksHandler)(nil)
