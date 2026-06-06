package costfillassignment

import (
	"context"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// CreateAllTasksHandler implements FillTaskCreator — resolves configs for all route
// levels and bulk-inserts Task rows in a single call.
type CreateAllTasksHandler struct {
	configRepo domain.ConfigRepository
	taskRepo   domain.TaskRepository
}

// NewCreateAllTasksHandler constructs the handler.
func NewCreateAllTasksHandler(configRepo domain.ConfigRepository, taskRepo domain.TaskRepository) *CreateAllTasksHandler {
	return &CreateAllTasksHandler{configRepo: configRepo, taskRepo: taskRepo}
}

// CreateForRequest resolves config for each level, builds Task objects, and bulk-inserts them.
// Returns domain.ErrConfigNotFound (wrapped) if any level has no active global config.
func (h *CreateAllTasksHandler) CreateForRequest(ctx context.Context, requestID, productSysID, routeHeadID int64, routeLevels []int32, totalParams int32) error {
	resolver := NewConfigResolverAdapter(h.configRepo)
	tasks := make([]*domain.Task, 0, len(routeLevels))
	for _, level := range routeLevels {
		rc, err := resolver.Resolve(ctx, productSysID, requestID, level)
		if err != nil {
			return fmt.Errorf("resolve config for level %d: %w", level, err)
		}
		tasks = append(tasks, domain.NewTask(requestID, routeHeadID, rc, totalParams))
	}
	if err := h.taskRepo.BulkInsert(ctx, tasks); err != nil {
		return fmt.Errorf("bulk insert tasks for request %d: %w", requestID, err)
	}
	return nil
}

var _ FillTaskCreator = (*CreateAllTasksHandler)(nil)
