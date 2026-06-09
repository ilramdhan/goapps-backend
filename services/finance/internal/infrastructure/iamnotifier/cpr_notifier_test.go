package iamnotifier_test

import (
	"context"
	"testing"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	cprapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamnotifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockNotifClient struct{ mock.Mock }

func (m *mockNotifClient) Create(_ context.Context, _ iamclient.CreateNotificationParams) error {
	return nil
}
func (m *mockNotifClient) RequestNotification(ctx context.Context, p iamclient.RequestNotificationParams) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}
func (m *mockNotifClient) Close() error { return nil }

func TestCPRNotifier_NotifyEvent_DraftCreated(t *testing.T) {
	mc := &mockNotifClient{}
	mc.On("RequestNotification", mock.Anything, mock.MatchedBy(func(p iamclient.RequestNotificationParams) bool {
		return p.EventType == "CPR_DRAFT_CREATED" &&
			len(p.Rules) == 1 &&
			p.Rules[0].RuleType == iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_PERMISSION &&
			p.Rules[0].Value == "finance.product.request.submit" &&
			p.Type == iamv1.NotificationType_NOTIFICATION_TYPE_ASSIGNMENT
	})).Return(nil)

	n := iamnotifier.NewCPRNotifier(mc)
	err := n.NotifyEvent(context.Background(), cprapp.CPREvent{
		EventType: "CPR_DRAFT_CREATED",
		RequestID: 42,
		RequestNo: "CPR-042",
		Rules: []cprapp.CPRNotifRule{
			{RuleType: "BY_PERMISSION", Value: "finance.product.request.submit"},
		},
	})

	assert.NoError(t, err)
	mc.AssertExpectations(t)
}
