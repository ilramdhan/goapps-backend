// Package iamnotifier provides notification adapters that call the IAM
// RequestNotification gRPC endpoint instead of writing to the local
// Finance notification table.
package iamnotifier

import (
	"context"
	"fmt"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	cprapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
)

// CPRNotifier implements cprapp.CPRNotifier by calling IAM RequestNotification
// with the appropriate recipient rules for each event type.
type CPRNotifier struct {
	client iamclient.NotificationClient
}

// NewCPRNotifier constructs the notifier.
func NewCPRNotifier(client iamclient.NotificationClient) *CPRNotifier {
	return &CPRNotifier{client: client}
}

// NotifyEvent dispatches a CPR lifecycle event notification via IAM.
// Best-effort only — callers log and continue on error.
func (n *CPRNotifier) NotifyEvent(ctx context.Context, event cprapp.CPREvent) error {
	params := n.buildParams(event)
	if err := n.client.RequestNotification(ctx, params); err != nil {
		return fmt.Errorf("cpr notify %s: %w", event.EventType, err)
	}
	return nil
}

func (n *CPRNotifier) buildParams(event cprapp.CPREvent) iamclient.RequestNotificationParams {
	rules := make([]iamclient.RecipientRule, 0, len(event.Rules))
	for _, r := range event.Rules {
		rules = append(rules, iamclient.RecipientRule{
			RuleType: ruleTypeFrom(r.RuleType),
			Value:    r.Value,
		})
	}

	title, body, notifType, severity := cprEventMeta(event.EventType, event.RequestNo)

	return iamclient.RequestNotificationParams{
		EventType:      event.EventType,
		SourceService:  "finance",
		SourceType:     "cost_product_request",
		SourceID:       fmt.Sprintf("%d", event.RequestID),
		Rules:          rules,
		Type:           notifType,
		Severity:       severity,
		Title:          title,
		Body:           body,
		ActionType:     iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_NAVIGATE,
		ActionPayload:  fmt.Sprintf(`{"path":"/finance/product-requests/%d"}`, event.RequestID),
		IdempotencyKey: fmt.Sprintf("%s:%d", event.EventType, event.RequestID),
	}
}

// cprEventMeta returns the display text and notification metadata for each
// CPR event type.
func cprEventMeta(eventType, requestNo string) (title, body string, notifType iamv1.NotificationType, severity iamv1.NotificationSeverity) {
	ref := requestNo
	if ref == "" {
		ref = "a product request"
	}

	switch eventType {
	case "CPR_DRAFT_CREATED":
		return "New product request ready for submission",
			fmt.Sprintf("Product request %s has been created and is awaiting submission.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_ASSIGNMENT,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO
	case "CPR_SUBMITTED_REVIEWER":
		return "Product request submitted — review required",
			fmt.Sprintf("Product request %s has been submitted and requires your review.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_ASSIGNMENT,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO
	case "CPR_SUBMITTED_ACK":
		return "Your product request was submitted",
			fmt.Sprintf("Product request %s has been submitted and is pending review.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO
	case "CPR_UNDER_REVIEW":
		return "Product request is under review",
			fmt.Sprintf("Product request %s is now under review.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO
	case "CPR_FEASIBLE":
		return "Product request assessed as feasible",
			fmt.Sprintf("Product request %s has been assessed as feasible.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_APPROVAL,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_SUCCESS
	case "CPR_NOT_FEASIBLE":
		return "Product request assessed as not feasible",
			fmt.Sprintf("Product request %s has been assessed as not feasible.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_ALERT,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_WARNING
	case "CPR_REJECTED":
		return "Product request rejected",
			fmt.Sprintf("Product request %s has been rejected. Check the reject note for details.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_ALERT,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_WARNING
	case "CPR_PARAM_COMPLETE_REQUESTER":
		return "All parameter fills approved",
			fmt.Sprintf("All parameters for product request %s have been filled and approved.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_SUCCESS
	case "CPR_PARAM_COMPLETE_COSTING":
		return "Product request ready for costing",
			fmt.Sprintf("Product request %s has all parameters filled — costing can now be triggered.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_ASSIGNMENT,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO
	case "CPR_CLOSED":
		return "Product request closed",
			fmt.Sprintf("Product request %s has been closed.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO
	default:
		return fmt.Sprintf("Product request update: %s", eventType),
			fmt.Sprintf("Product request %s has been updated.", ref),
			iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM,
			iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO
	}
}

// ruleTypeFrom converts the string rule type used in CPRNotifRule to the
// iamv1 proto enum expected by RecipientRule.
func ruleTypeFrom(rt string) iamv1.RecipientRuleType {
	switch rt {
	case "BY_USER_ID":
		return iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_USER_ID
	case "BY_PERMISSION":
		return iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_PERMISSION
	case "BY_DEPT":
		return iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_DEPT
	case "BY_ROLE":
		return iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_ROLE
	default:
		return iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_UNSPECIFIED
	}
}
