package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	appnotif "github.com/mutugading/goapps-backend/services/iam/internal/application/notification"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
)

// mockResolver implements UserResolver for RequestHandler tests.
type mockResolver struct{ mock.Mock }

func (m *mockResolver) GetByPermission(ctx context.Context, code string) ([]uuid.UUID, error) {
	args := m.Called(ctx, code)
	return args.Get(0).([]uuid.UUID), args.Error(1)
}
func (m *mockResolver) GetByDept(ctx context.Context, code string) ([]uuid.UUID, error) {
	args := m.Called(ctx, code)
	return args.Get(0).([]uuid.UUID), args.Error(1)
}
func (m *mockResolver) GetByRole(ctx context.Context, name string) ([]uuid.UUID, error) {
	args := m.Called(ctx, name)
	return args.Get(0).([]uuid.UUID), args.Error(1)
}
func (m *mockResolver) GetByUserID(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	args := m.Called(ctx, id)
	return args.Get(0).([]uuid.UUID), args.Error(1)
}

// mockCreator stubs the notifCreator interface used by RequestHandler.
type mockCreator struct{ mock.Mock }

func (m *mockCreator) Handle(ctx context.Context, cmd appnotif.CreateCommand) (*notification.Notification, error) {
	args := m.Called(ctx, cmd)
	if v := args.Get(0); v != nil {
		return v.(*notification.Notification), args.Error(1)
	}
	return nil, args.Error(1)
}

func baseCmd(rules []appnotif.RecipientRuleCmd) appnotif.RequestCommand {
	return appnotif.RequestCommand{
		Rules:         rules,
		Type:          notification.TypeAlert,
		Severity:      notification.SeverityInfo,
		Title:         "test notification",
		Body:          "body",
		ActionType:    notification.ActionNone,
		ActionPayload: "",
		SourceType:    "test",
		SourceID:      "src-1",
		ExpiresAt:     nil,
		CreatedBy:     "system",
	}
}

func TestRequestHandler_FansOutPerUser(t *testing.T) {
	t.Parallel()

	user1 := uuid.New()
	user2 := uuid.New()

	resolver := &mockResolver{}
	resolver.On("GetByRole", mock.Anything, "admin").Return([]uuid.UUID{user1, user2}, nil).Once()

	creator := &mockCreator{}
	// Expect one Create call per resolved user.
	n1, err := notification.NewNotification(user1,
		notification.TypeAlert, notification.SeverityInfo,
		"test notification", "body", notification.ActionNone, "", "test", "src-1", "system", nil)
	require.NoError(t, err)
	n2, err := notification.NewNotification(user2,
		notification.TypeAlert, notification.SeverityInfo,
		"test notification", "body", notification.ActionNone, "", "test", "src-1", "system", nil)
	require.NoError(t, err)

	creator.On("Handle", mock.Anything, mock.MatchedBy(func(cmd appnotif.CreateCommand) bool {
		return cmd.RecipientUserID == user1
	})).Return(n1, nil).Once()
	creator.On("Handle", mock.Anything, mock.MatchedBy(func(cmd appnotif.CreateCommand) bool {
		return cmd.RecipientUserID == user2
	})).Return(n2, nil).Once()

	h := appnotif.NewRequestHandler(creator, resolver, nil)
	cmd := baseCmd([]appnotif.RecipientRuleCmd{
		{RuleType: iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_ROLE, Value: "admin"},
	})

	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.Equal(t, 2, result.RecipientCount)
	assert.NotEqual(t, uuid.Nil, result.EventID)

	creator.AssertExpectations(t)
	resolver.AssertExpectations(t)
}

func TestRequestHandler_DeduplicatesAcrossRules(t *testing.T) {
	t.Parallel()

	sharedUser := uuid.New()
	uniqueUser := uuid.New()

	resolver := &mockResolver{}
	// Rule 1: BY_PERMISSION returns sharedUser + uniqueUser.
	resolver.On("GetByPermission", mock.Anything, "finance.master.uom.view").
		Return([]uuid.UUID{sharedUser, uniqueUser}, nil).Once()
	// Rule 2: BY_USER_ID returns sharedUser again (should be deduplicated).
	resolver.On("GetByUserID", mock.Anything, sharedUser).
		Return([]uuid.UUID{sharedUser}, nil).Once()

	creator := &mockCreator{}
	// After deduplication, only 2 unique users → 2 create calls (not 3).
	n1, err := notification.NewNotification(sharedUser,
		notification.TypeAlert, notification.SeverityInfo,
		"test notification", "body", notification.ActionNone, "", "test", "src-1", "system", nil)
	require.NoError(t, err)
	n2, err := notification.NewNotification(uniqueUser,
		notification.TypeAlert, notification.SeverityInfo,
		"test notification", "body", notification.ActionNone, "", "test", "src-1", "system", nil)
	require.NoError(t, err)

	creator.On("Handle", mock.Anything, mock.MatchedBy(func(cmd appnotif.CreateCommand) bool {
		return cmd.RecipientUserID == sharedUser
	})).Return(n1, nil).Once()
	creator.On("Handle", mock.Anything, mock.MatchedBy(func(cmd appnotif.CreateCommand) bool {
		return cmd.RecipientUserID == uniqueUser
	})).Return(n2, nil).Once()

	h := appnotif.NewRequestHandler(creator, resolver, nil)
	cmd := baseCmd([]appnotif.RecipientRuleCmd{
		{RuleType: iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_PERMISSION, Value: "finance.master.uom.view"},
		{RuleType: iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_USER_ID, Value: sharedUser.String()},
	})

	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)
	// 2 unique users despite 3 total (sharedUser appeared in both rules).
	assert.Equal(t, 2, result.RecipientCount)

	creator.AssertExpectations(t)
	resolver.AssertExpectations(t)
}

func TestRequestHandler_NoRulesReturnsZero(t *testing.T) {
	t.Parallel()

	creator := &mockCreator{}
	resolver := &mockResolver{}

	h := appnotif.NewRequestHandler(creator, resolver, nil)
	result, err := h.Handle(context.Background(), baseCmd(nil))
	require.NoError(t, err)
	assert.Equal(t, 0, result.RecipientCount)
	assert.NotEqual(t, uuid.Nil, result.EventID)

	creator.AssertNumberOfCalls(t, "Handle", 0)
}

func TestRequestHandler_EmailDispatchedWhenSet(t *testing.T) {
	t.Parallel()

	user := uuid.New()
	resolver := &mockResolver{}
	resolver.On("GetByUserID", mock.Anything, user).Return([]uuid.UUID{user}, nil).Once()

	n, err := notification.NewNotification(user,
		notification.TypeAlert, notification.SeverityInfo,
		"test notification", "body", notification.ActionNone, "", "test", "src-1", "system", nil)
	require.NoError(t, err)

	creator := &mockCreator{}
	creator.On("Handle", mock.Anything, mock.MatchedBy(func(cmd appnotif.CreateCommand) bool {
		return cmd.RecipientUserID == user
	})).Return(n, nil).Once()

	dispatched := make(chan *notification.Notification, 1)
	emailDisp := appnotif.EmailDispatcherFunc(func(_ context.Context, notif *notification.Notification) {
		dispatched <- notif
	})

	h := appnotif.NewRequestHandler(creator, resolver, emailDisp)
	cmd := baseCmd([]appnotif.RecipientRuleCmd{
		{RuleType: iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_USER_ID, Value: user.String()},
	})

	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.Equal(t, 1, result.RecipientCount)

	select {
	case got := <-dispatched:
		assert.Equal(t, n.ID(), got.ID())
	case <-time.After(2 * time.Second):
		t.Fatal("email dispatcher was not called")
	}

	creator.AssertExpectations(t)
	resolver.AssertExpectations(t)
}
