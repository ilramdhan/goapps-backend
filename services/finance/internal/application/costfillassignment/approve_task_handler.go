// Package costfillassignment provides application-layer handlers for the fill-assignment feature.
package costfillassignment

import (
	"context"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// ApproveTaskCommand carries the approval.
type ApproveTaskCommand struct {
	TaskID     int64
	RequestID  int64
	ApproverID string
	Note       string
}

// ApproveTaskHandler approves a fill task and records the approval event.
type ApproveTaskHandler struct {
	repo domain.TaskRepository
	gate CompletionGate
}

// NewApproveTaskHandler constructs the handler.
func NewApproveTaskHandler(repo domain.TaskRepository, gate CompletionGate) *ApproveTaskHandler {
	return &ApproveTaskHandler{repo: repo, gate: gate}
}

// Handle approves the task, records an Approval row, and checks the completion gate.
func (h *ApproveTaskHandler) Handle(ctx context.Context, cmd ApproveTaskCommand) error {
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
	if err = task.Approve(cmd.ApproverID); err != nil {
		return fmt.Errorf("approve task %d: %w", cmd.TaskID, err)
	}
	if err = h.repo.Save(ctx, task); err != nil {
		return fmt.Errorf("save task %d after approve: %w", cmd.TaskID, err)
	}
	approval := &domain.Approval{
		TaskID:    cmd.TaskID,
		Decision:  domain.DecisionApproved,
		DecidedBy: cmd.ApproverID,
		Note:      cmd.Note,
		Trigger:   domain.TriggerInitial,
	}
	if err = h.repo.AddApproval(ctx, approval); err != nil {
		return fmt.Errorf("record approval for task %d: %w", cmd.TaskID, err)
	}
	if gateErr := h.gate.CheckAndAdvance(ctx, cmd.RequestID, task.RouteLevel); gateErr != nil {
		return fmt.Errorf("completion gate after approve: %w", gateErr)
	}
	return nil
}
