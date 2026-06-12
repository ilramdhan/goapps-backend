package iamnotifier

import (
	"context"
	"fmt"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	appfill "github.com/mutugading/goapps-backend/services/finance/internal/application/costfillassignment"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
)

// FillNotifier implements appfill.FillEventNotifier by dispatching fill-task
// lifecycle notifications via IAM RequestNotification. Supports both USER and
// DEPT assignee types — DEPT tasks are fanned out to all department members by IAM.
type FillNotifier struct {
	client iamclient.NotificationClient
}

// NewFillNotifier constructs the notifier.
func NewFillNotifier(client iamclient.NotificationClient) *FillNotifier {
	return &FillNotifier{client: client}
}

var _ appfill.FillEventNotifier = (*FillNotifier)(nil)

// NotifyTaskActivated fires when a fill task becomes ACTIVE (on creation or chain step).
func (n *FillNotifier) NotifyTaskActivated(ctx context.Context, task *domain.Task, requestNo string) error {
	title, body := fillActivatedMeta(task, requestNo)
	rules := n.fillerRules(task)
	return n.dispatch(ctx, "FILL_TASK_ACTIVATED", task.RequestID, task.RouteLevel,
		title, body, rules,
		iamv1.NotificationType_NOTIFICATION_TYPE_ASSIGNMENT,
		iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO,
	)
}

// NotifyApprovalPending fires when all params are filled and the approver is awaiting action.
func (n *FillNotifier) NotifyApprovalPending(ctx context.Context, task *domain.Task, requestNo string) error {
	ref := requestNoRef(requestNo)
	title := "Fill task awaiting your approval"
	body := fmt.Sprintf("Fill task for product request %s (level %d) is ready for your review.", ref, task.RouteLevel)
	rules := n.approverRules(task)
	return n.dispatch(ctx, "FILL_APPROVAL_PENDING", task.RequestID, task.RouteLevel,
		title, body, rules,
		iamv1.NotificationType_NOTIFICATION_TYPE_APPROVAL,
		iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO,
	)
}

// NotifyTaskRejected fires when an approver rejects the fill task.
// Routes to the task filler (ClaimedBy if set, otherwise FillerValue).
func (n *FillNotifier) NotifyTaskRejected(ctx context.Context, task *domain.Task, requestNo string) error {
	ref := requestNoRef(requestNo)
	title := "Your fill task was rejected"
	body := fmt.Sprintf("Fill task for product request %s (level %d) was rejected. Please revise and resubmit.", ref, task.RouteLevel)

	recipientID := task.ClaimedBy
	if recipientID == "" {
		recipientID = task.FillerValue
	}
	rules := []iamclient.RecipientRule{
		{RuleType: iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_USER_ID, Value: recipientID},
	}
	return n.dispatch(ctx, "FILL_TASK_REJECTED", task.RequestID, task.RouteLevel,
		title, body, rules,
		iamv1.NotificationType_NOTIFICATION_TYPE_ALERT,
		iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_WARNING,
	)
}

// NotifyAllApproved fires when all routing levels are approved and parameter fill is complete.
func (n *FillNotifier) NotifyAllApproved(ctx context.Context, requestID int64, requesterUserID, requestNo string) error {
	ref := requestNoRef(requestNo)
	title := "All fill tasks approved"
	body := fmt.Sprintf("All parameter fills for product request %s have been approved.", ref)
	rules := []iamclient.RecipientRule{
		{RuleType: iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_USER_ID, Value: requesterUserID},
	}
	return n.dispatch(ctx, "FILL_ALL_APPROVED", requestID, 0,
		title, body, rules,
		iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM,
		iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_SUCCESS,
	)
}

// NotifyOverdue fires from the SLA cron for tasks past their SLA deadline.
func (n *FillNotifier) NotifyOverdue(ctx context.Context, task *domain.Task) error {
	title := "Fill task SLA overdue"
	body := fmt.Sprintf("Fill task (level %d, request %d) has exceeded its SLA deadline.", task.RouteLevel, task.RequestID)
	rules := n.fillerRules(task)
	return n.dispatch(ctx, "FILL_TASK_OVERDUE", task.RequestID, task.RouteLevel,
		title, body, rules,
		iamv1.NotificationType_NOTIFICATION_TYPE_ALERT,
		iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_WARNING,
	)
}

// NotifyReminderFill fires from the reminder cron for ACTIVE/FILLING tasks.
func (n *FillNotifier) NotifyReminderFill(ctx context.Context, task *domain.Task) error {
	title := "Reminder: fill task pending"
	body := fmt.Sprintf("Fill task (level %d, request %d) is still awaiting your input.", task.RouteLevel, task.RequestID)
	rules := n.fillerRules(task)
	return n.dispatch(ctx, "FILL_REMINDER_FILL", task.RequestID, task.RouteLevel,
		title, body, rules,
		iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM,
		iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO,
	)
}

// NotifyReminderApproval fires from the reminder cron for APPROVAL_PENDING tasks.
func (n *FillNotifier) NotifyReminderApproval(ctx context.Context, task *domain.Task) error {
	title := "Reminder: fill task awaiting approval"
	body := fmt.Sprintf("Fill task (level %d, request %d) is still waiting for your approval.", task.RouteLevel, task.RequestID)
	rules := n.approverRules(task)
	return n.dispatch(ctx, "FILL_REMINDER_APPROVAL", task.RequestID, task.RouteLevel,
		title, body, rules,
		iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM,
		iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO,
	)
}

// fillerRules returns the recipient rules targeting the task's filler.
// USER filler → BY_USER_ID; DEPT filler → BY_DEPT (IAM fans out to all members).
func (n *FillNotifier) fillerRules(task *domain.Task) []iamclient.RecipientRule {
	return actorRules(task.FillerType, task.FillerValue)
}

// approverRules returns the recipient rules targeting the task's approver.
func (n *FillNotifier) approverRules(task *domain.Task) []iamclient.RecipientRule {
	return actorRules(task.ApproverType, task.ApproverValue)
}

// actorRules builds a RecipientRule slice for the given actor type and value.
func actorRules(actorType, actorValue string) []iamclient.RecipientRule {
	switch actorType {
	case domain.ActorUser:
		return []iamclient.RecipientRule{
			{RuleType: iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_USER_ID, Value: actorValue},
		}
	case domain.ActorDept:
		return []iamclient.RecipientRule{
			{RuleType: iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_DEPT, Value: actorValue},
		}
	default:
		return nil
	}
}

// dispatch calls IAM RequestNotification with the supplied parameters.
func (n *FillNotifier) dispatch(
	ctx context.Context,
	eventType string,
	requestID int64,
	routeLevel int32,
	title, body string,
	rules []iamclient.RecipientRule,
	notifType iamv1.NotificationType,
	severity iamv1.NotificationSeverity,
) error {
	if len(rules) == 0 {
		return nil // no recipient — nothing to dispatch
	}
	idempotencyKey := fmt.Sprintf("%s:%d:%d", eventType, requestID, routeLevel)
	params := iamclient.RequestNotificationParams{
		EventType:      eventType,
		SourceService:  "finance",
		SourceType:     "cost_fill_task",
		SourceID:       fmt.Sprintf("%d", requestID),
		Rules:          rules,
		Type:           notifType,
		Severity:       severity,
		Title:          title,
		Body:           body,
		ActionType:     iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_NAVIGATE,
		ActionPayload:  fmt.Sprintf(`{"path":"/finance/product-requests/%d"}`, requestID),
		IdempotencyKey: idempotencyKey,
	}
	if err := n.client.RequestNotification(ctx, params); err != nil {
		return fmt.Errorf("fill notify %s request %d level %d: %w", eventType, requestID, routeLevel, err)
	}
	return nil
}

// fillActivatedMeta returns title and body for a task-activated event.
func fillActivatedMeta(task *domain.Task, requestNo string) (title, body string) {
	ref := requestNoRef(requestNo)
	title = "New fill task assigned to you"
	body = fmt.Sprintf("You have been assigned a fill task for product request %s (level %d).", ref, task.RouteLevel)
	return title, body
}

// requestNoRef returns requestNo if non-empty, otherwise a generic reference string.
func requestNoRef(requestNo string) string {
	if requestNo != "" {
		return requestNo
	}
	return "a product request"
}
