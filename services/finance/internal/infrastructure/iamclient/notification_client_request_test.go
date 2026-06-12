package iamclient_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
)

func TestNopClient_RequestNotification_ReturnsNil(t *testing.T) {
	c := iamclient.NewNopClient()
	err := c.RequestNotification(context.Background(), iamclient.RequestNotificationParams{
		EventType:     "TEST_EVENT",
		SourceService: "finance",
		Rules: []iamclient.RecipientRule{
			{
				RuleType: iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_USER_ID,
				Value:    "00000000-0000-0000-0000-000000000001",
			},
		},
		Type:       iamv1.NotificationType_NOTIFICATION_TYPE_SYSTEM,
		Severity:   iamv1.NotificationSeverity_NOTIFICATION_SEVERITY_INFO,
		Title:      "Test notification",
		ActionType: iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_NONE,
	})
	require.NoError(t, err)
}
