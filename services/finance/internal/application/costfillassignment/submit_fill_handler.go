package costfillassignment

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// SubmitFillCommand carries the submit request.
type SubmitFillCommand struct {
	TaskID    int64
	RequestID int64
	UserID    string
}

// SubmitFillHandler transitions a FILLING task to APPROVAL_PENDING or APPROVED.
type SubmitFillHandler struct {
	repo          domain.TaskRepository
	gate          CompletionGate
	fillNotifier  FillEventNotifier  // optional; fires NotifyApprovalPending when task reaches APPROVAL_PENDING
	reqNoProvider RequestNoProvider  // optional; resolves request_no for the notification
}

// NewSubmitFillHandler constructs the handler.
func NewSubmitFillHandler(repo domain.TaskRepository, gate CompletionGate) *SubmitFillHandler {
	return &SubmitFillHandler{repo: repo, gate: gate}
}

// WithFillNotifier attaches a FillEventNotifier. Returns receiver for chaining.
func (h *SubmitFillHandler) WithFillNotifier(fn FillEventNotifier) *SubmitFillHandler {
	h.fillNotifier = fn
	return h
}

// WithRequestNoProvider attaches a RequestNoProvider used to resolve request_no
// for the approval-pending notification. Returns receiver for chaining.
func (h *SubmitFillHandler) WithRequestNoProvider(p RequestNoProvider) *SubmitFillHandler {
	h.reqNoProvider = p
	return h
}

// Handle loads the task, calls Submit() on the domain object, saves it, and if auto-approved checks the gate.
func (h *SubmitFillHandler) Handle(ctx context.Context, cmd SubmitFillCommand) error {
	if cmd.TaskID <= 0 {
		return fmt.Errorf("task ID must be > 0")
	}
	task, err := h.repo.GetByID(ctx, cmd.TaskID)
	if err != nil {
		return fmt.Errorf("get task %d: %w", cmd.TaskID, err)
	}
	if err = task.Submit(); err != nil {
		return fmt.Errorf("submit task %d: %w", cmd.TaskID, err)
	}
	if err = h.repo.Save(ctx, task); err != nil {
		return fmt.Errorf("save task %d after submit: %w", cmd.TaskID, err)
	}
	// Notify approver when task enters APPROVAL_PENDING.
	if task.Status() == domain.StatusApprovalPending && h.fillNotifier != nil {
		h.notifyApprovalPending(ctx, task, cmd.RequestID)
	}
	// If the task auto-approved (no approver), check if all tasks are done.
	if task.Status() == domain.StatusApproved {
		if gateErr := h.gate.CheckAndAdvance(ctx, cmd.RequestID, task.RouteLevel); gateErr != nil {
			return fmt.Errorf("completion gate after auto-approve: %w", gateErr)
		}
	}
	return nil
}

// notifyApprovalPending fires NotifyApprovalPending in a best-effort, non-fatal manner.
func (h *SubmitFillHandler) notifyApprovalPending(ctx context.Context, task *domain.Task, requestID int64) {
	requestNo := ""
	if h.reqNoProvider != nil {
		if no, lookupErr := h.reqNoProvider.GetRequestNo(ctx, requestID); lookupErr == nil {
			requestNo = no
		} else {
			log.Warn().Err(lookupErr).Int64("request_id", requestID).
				Msg("SubmitFillHandler: requestNo lookup failed (non-fatal, notification will have empty ref)")
		}
	}
	if notifyErr := h.fillNotifier.NotifyApprovalPending(ctx, task, requestNo); notifyErr != nil {
		log.Warn().Err(notifyErr).Int64("task_id", task.TaskID).
			Msg("SubmitFillHandler: NotifyApprovalPending failed (non-fatal)")
	}
}
