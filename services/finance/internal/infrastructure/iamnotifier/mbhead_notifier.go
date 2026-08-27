package iamnotifier

import (
	"context"
	"fmt"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	mbheadapp "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
)

// MBHeadNotifier implements mbheadapp.Notifier by calling IAM
// RequestNotification with the recipient rules carried on each event.
type MBHeadNotifier struct {
	client iamclient.NotificationClient
}

// NewMBHeadNotifier constructs the notifier.
func NewMBHeadNotifier(client iamclient.NotificationClient) *MBHeadNotifier {
	return &MBHeadNotifier{client: client}
}

// NotifyEvent dispatches an MB recipe lifecycle event notification via IAM.
// Best-effort only — callers log and continue on error.
func (n *MBHeadNotifier) NotifyEvent(ctx context.Context, event mbheadapp.Event) error {
	if len(event.Rules) == 0 {
		return nil // no recipient rule — nothing to dispatch
	}
	if err := n.client.RequestNotification(ctx, n.buildParams(event)); err != nil {
		return fmt.Errorf("mbhead notify %s: %w", event.EventType, err)
	}
	return nil
}

func (n *MBHeadNotifier) buildParams(event mbheadapp.Event) iamclient.RequestNotificationParams {
	rules := make([]iamclient.RecipientRule, 0, len(event.Rules))
	for _, r := range event.Rules {
		rules = append(rules, iamclient.RecipientRule{
			RuleType: ruleTypeFrom(r.RuleType),
			Value:    r.Value,
		})
	}

	title, body, notifType, severity := mbHeadEventMeta(event)

	return iamclient.RequestNotificationParams{
		EventType:     event.EventType,
		SourceService: "finance",
		SourceType:    "mb_head",
		SourceID:      event.MbhID.String(),
		Rules:         rules,
		Type:          notifType,
		Severity:      severity,
		Title:         title,
		Body:          body,
		ActionType:    iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_NAVIGATE,
		ActionPayload: fmt.Sprintf(`{"path":"/finance/mb-recipe/%s"}`, event.MbhID.String()),
		// 🔴 The key is unique per (head, event, transition, version). Including the
		// transition and the version is what lets a head legitimately cycle through the
		// same state twice — submit, reject, resubmit — and notify each time, while a
		// retry of the SAME dispatch stays a duplicate and is suppressed.
		IdempotencyKey: fmt.Sprintf("%s:%s:%s:%s:%d",
			event.EventType, event.MbhID.String(), event.FromState, event.ToState, event.Version),
	}
}

// mbHeadEventMeta returns the display text and notification metadata for each MB
// recipe event type.
func mbHeadEventMeta(event mbheadapp.Event) (title, body string, notifType iamv1.NotificationType, severity iamv1.NotificationSeverity) {
	ref := event.MBCosting
	if ref == "" {
		ref = "an MB recipe"
	}

	switch event.EventType {
	case mbheadapp.EventSubmitted:
		return "MB recipe submitted for approval",
			fmt.Sprintf("MB recipe %s has been submitted and is waiting for your approval.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_APPROVAL,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO
	case mbheadapp.EventReturnedToDraft:
		return "MB recipe returned to draft",
			fmt.Sprintf("MB recipe %s was rejected and is back in draft for rework.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_APPROVAL,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_WARNING
	case mbheadapp.EventUnlockRequested:
		return "MB recipe unlock requested",
			fmt.Sprintf("An unlock has been requested for MB recipe %s and is waiting for a decision.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_APPROVAL,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO
	case mbheadapp.EventUnlockGranted:
		return "MB recipe unlock granted",
			fmt.Sprintf("Your unlock request for MB recipe %s was granted; it is now editable.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_APPROVAL,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO
	case mbheadapp.EventUnlockRejected:
		return "MB recipe unlock rejected",
			fmt.Sprintf("Your unlock request for MB recipe %s was rejected; it stays locked.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_APPROVAL,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_WARNING
	default:
		return fmt.Sprintf("MB recipe update: %s", event.EventType),
			fmt.Sprintf("MB recipe %s has been updated.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO
	}
}
