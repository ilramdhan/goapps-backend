package costfillassignment

import (
	"context"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// RejectTaskCommand carries the rejection.
type RejectTaskCommand struct {
	TaskID     int64
	ApproverID string
	Reason     string
}

// RejectTaskHandler rejects a fill task, recording the rejection event.
type RejectTaskHandler struct {
	repo domain.TaskRepository
}

// NewRejectTaskHandler constructs the handler.
func NewRejectTaskHandler(repo domain.TaskRepository) *RejectTaskHandler {
	return &RejectTaskHandler{repo: repo}
}

// Handle rejects the task and records an Approval row with REJECTED decision.
func (h *RejectTaskHandler) Handle(ctx context.Context, cmd RejectTaskCommand) error {
	if cmd.TaskID <= 0 {
		return fmt.Errorf("task ID must be > 0")
	}
	if cmd.ApproverID == "" {
		return fmt.Errorf("approver ID is required")
	}
	task, err := h.repo.GetByID(ctx, cmd.TaskID)
	if err != nil {
		return fmt.Errorf("get task %d: %w", cmd.TaskID, err)
	}
	if err = task.Reject(cmd.ApproverID, cmd.Reason); err != nil {
		return fmt.Errorf("reject task %d: %w", cmd.TaskID, err)
	}
	if err = h.repo.Save(ctx, task); err != nil {
		return fmt.Errorf("save task %d after reject: %w", cmd.TaskID, err)
	}
	approval := &domain.Approval{
		TaskID:    cmd.TaskID,
		Decision:  domain.DecisionRejected,
		DecidedBy: cmd.ApproverID,
		Note:      cmd.Reason,
		Trigger:   domain.TriggerInitial,
	}
	if err = h.repo.AddApproval(ctx, approval); err != nil {
		return fmt.Errorf("record rejection for task %d: %w", cmd.TaskID, err)
	}
	return nil
}
