package costfillassignment

import (
	"context"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// ListTasksQuery specifies which request's tasks to list.
type ListTasksQuery struct {
	RequestID int64
}

// ListTasksResult contains fill tasks and their approval histories.
type ListTasksResult struct {
	Tasks     []*domain.Task
	Approvals map[int64][]*domain.Approval // keyed by TaskID
}

// ListTasksHandler returns fill tasks for a CPR with their approval histories.
type ListTasksHandler struct {
	repo domain.TaskRepository
}

// NewListTasksHandler constructs the handler.
func NewListTasksHandler(repo domain.TaskRepository) *ListTasksHandler {
	return &ListTasksHandler{repo: repo}
}

// Handle fetches tasks and their approval logs for a request.
func (h *ListTasksHandler) Handle(ctx context.Context, q ListTasksQuery) (*ListTasksResult, error) {
	if q.RequestID <= 0 {
		return nil, fmt.Errorf("request ID must be > 0")
	}
	tasks, err := h.repo.ListByRequest(ctx, q.RequestID)
	if err != nil {
		return nil, fmt.Errorf("list tasks for request %d: %w", q.RequestID, err)
	}
	approvals := make(map[int64][]*domain.Approval, len(tasks))
	for _, t := range tasks {
		history, aErr := h.repo.ListApprovals(ctx, t.TaskID)
		if aErr != nil {
			return nil, fmt.Errorf("list approvals for task %d: %w", t.TaskID, aErr)
		}
		approvals[t.TaskID] = history
	}
	return &ListTasksResult{Tasks: tasks, Approvals: approvals}, nil
}
