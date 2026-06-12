package costfillassignment

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// Notifier sends SLA/overdue notifications. Implementation provided externally.
type Notifier interface {
	NotifyOverdue(ctx context.Context, taskID int64) error
}

// FillEventNotifier dispatches notifications for fill task lifecycle events.
// Supports both USER and DEPT assignee types. Replaces the combined
// Notifier+CompletionNotifier for new code; old interfaces are kept for
// backward compatibility during migration.
// All methods are best-effort — implementations log and return nil on failure.
type FillEventNotifier interface {
	// NotifyTaskActivated fires when a fill task becomes ACTIVE (on creation).
	NotifyTaskActivated(ctx context.Context, task *domain.Task, requestNo string) error
	// NotifyApprovalPending fires when all params are filled and approver exists.
	NotifyApprovalPending(ctx context.Context, task *domain.Task, requestNo string) error
	// NotifyTaskRejected fires when an approver rejects. Routes to task.ClaimedBy.
	NotifyTaskRejected(ctx context.Context, task *domain.Task, requestNo string) error
	// NotifyAllApproved fires when all routing levels are approved.
	NotifyAllApproved(ctx context.Context, requestID int64, requesterUserID, requestNo string) error
	// NotifyOverdue fires from the SLA cron for tasks past their SLA deadline.
	NotifyOverdue(ctx context.Context, task *domain.Task) error
	// NotifyReminderFill fires from the reminder cron for ACTIVE/FILLING tasks.
	NotifyReminderFill(ctx context.Context, task *domain.Task) error
	// NotifyReminderApproval fires from the reminder cron for APPROVAL_PENDING tasks.
	NotifyReminderApproval(ctx context.Context, task *domain.Task) error
}

// CompletionGate checks if all fill tasks for a request are approved and fires next state.
type CompletionGate interface {
	CheckAndAdvance(ctx context.Context, requestID int64, approvedLevel int32) error
}

// FillTaskCreator creates fill tasks for all route levels of a request.
// It is called by MarkParameterPending BEFORE the state transition.
// Returns ErrConfigNotFound (wrapped) if a level has no global config.
// perLevelTotals maps route level → total applicable params for that level's products.
type FillTaskCreator interface {
	CreateForRequest(ctx context.Context, requestID, productSysID, routeHeadID int64, routeLevels []int32, perLevelTotals map[int32]int32, requestNo string) error
}

// CPRCompleter triggers the PARAMETER_COMPLETE state transition on the CPR aggregate.
// Implemented by costproductrequest.TransitionHandler and provided via main.go wiring.
// The returned requesterUserID and requestNo are used for notification after completion.
type CPRCompleter interface {
	MarkParameterComplete(ctx context.Context, requestID int64, actor string) (requesterUserID, requestNo string, err error)
}

// CompletionNotifier sends in-app notifications during the completion chain.
// Decoupled from the costnotification package to preserve clean architecture layers.
type CompletionNotifier interface {
	NotifyFiller(ctx context.Context, taskID int64, recipientUserID, requestNo string) error
	NotifyComplete(ctx context.Context, requestID int64, requesterUserID, requestNo string) error
}

// RequestNoProvider resolves a CPR request number from its numeric ID.
// Used by SubmitFillHandler to fetch requestNo for the approval-pending notification.
type RequestNoProvider interface {
	GetRequestNo(ctx context.Context, requestID int64) (string, error)
}
