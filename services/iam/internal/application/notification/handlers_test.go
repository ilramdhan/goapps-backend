package notification_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appnotif "github.com/mutugading/goapps-backend/services/iam/internal/application/notification"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
	notifinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/notification"
)

// repoMock implements notification.Repository for handler tests.
type repoMock struct{ mock.Mock }

func (m *repoMock) Create(ctx context.Context, n *notification.Notification) error {
	return m.Called(ctx, n).Error(0)
}
func (m *repoMock) GetByID(ctx context.Context, id uuid.UUID) (*notification.Notification, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*notification.Notification), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *repoMock) ListByRecipient(ctx context.Context, r uuid.UUID, f notification.ListFilter) ([]*notification.Notification, int64, error) {
	args := m.Called(ctx, r, f)
	return args.Get(0).([]*notification.Notification), args.Get(1).(int64), args.Error(2)
}
func (m *repoMock) CountUnread(ctx context.Context, r uuid.UUID) (int64, error) {
	args := m.Called(ctx, r)
	return args.Get(0).(int64), args.Error(1)
}
func (m *repoMock) MarkAsRead(ctx context.Context, r, n uuid.UUID, at time.Time) error {
	return m.Called(ctx, r, n, at).Error(0)
}
func (m *repoMock) MarkAllAsRead(ctx context.Context, r uuid.UUID, at time.Time) (int64, error) {
	args := m.Called(ctx, r, at)
	return args.Get(0).(int64), args.Error(1)
}
func (m *repoMock) Archive(ctx context.Context, r, n uuid.UUID, at time.Time) error {
	return m.Called(ctx, r, n, at).Error(0)
}
func (m *repoMock) Delete(ctx context.Context, r, n uuid.UUID) error {
	return m.Called(ctx, r, n).Error(0)
}
func (m *repoMock) DeleteExpired(ctx context.Context, t time.Time) (int64, error) {
	args := m.Called(ctx, t)
	return args.Get(0).(int64), args.Error(1)
}

func TestCreateHandler_HappyPathPublishes(t *testing.T) {
	t.Parallel()
	repo := &repoMock{}
	repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
	b := notifinfra.NewBroadcaster()
	user := uuid.New()
	ch, unsub := b.Subscribe(user)
	defer unsub()

	h := appnotif.NewCreateHandler(repo, b)
	got, err := h.Handle(context.Background(), appnotif.CreateCommand{
		RecipientUserID: user,
		Type:            notification.TypeAlert,
		Severity:        notification.SeverityInfo,
		Title:           "hi",
		ActionType:      notification.ActionNone,
		CreatedBy:       "system",
	})
	require.NoError(t, err)
	require.NotNil(t, got)

	select {
	case n := <-ch:
		assert.Equal(t, got.ID(), n.ID(), "broadcaster must deliver the just-created notification")
	case <-time.After(time.Second):
		t.Fatal("broadcaster did not publish")
	}
}

func TestCreateHandler_RepoFailureReturnsError(t *testing.T) {
	t.Parallel()
	repo := &repoMock{}
	repo.On("Create", mock.Anything, mock.Anything).Return(errors.New("db down")).Once()
	h := appnotif.NewCreateHandler(repo, notifinfra.NewBroadcaster())
	_, err := h.Handle(context.Background(), appnotif.CreateCommand{
		RecipientUserID: uuid.New(),
		Type:            notification.TypeAlert,
		Severity:        notification.SeverityInfo,
		Title:           "hi",
		ActionType:      notification.ActionNone,
	})
	require.Error(t, err)
}

func TestGetHandler_OwnershipCheck(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	stranger := uuid.New()
	n, err := notification.NewNotification(owner,
		notification.TypeAlert, notification.SeverityInfo,
		"t", "", notification.ActionNone, "", "", "", "system", nil)
	require.NoError(t, err)

	repo := &repoMock{}
	repo.On("GetByID", mock.Anything, n.ID()).Return(n, nil).Twice()

	h := appnotif.NewGetHandler(repo)
	// Owner can read
	got, err := h.Handle(context.Background(), owner, n.ID())
	require.NoError(t, err)
	assert.Equal(t, n.ID(), got.ID())

	// Stranger is forbidden
	_, err = h.Handle(context.Background(), stranger, n.ID())
	require.Error(t, err)
	assert.True(t, errors.Is(err, notification.ErrForbidden))
}

func TestListHandler_DefaultsAndFiltering(t *testing.T) {
	t.Parallel()
	user := uuid.New()
	repo := &repoMock{}
	expected := notification.ListFilter{
		Status:   notification.StatusUnread,
		Type:     "",
		Page:     1,
		PageSize: 10,
		SortDesc: true,
	}
	repo.On("ListByRecipient", mock.Anything, user, expected).
		Return([]*notification.Notification{}, int64(0), nil).Once()

	h := appnotif.NewListHandler(repo)
	res, err := h.Handle(context.Background(), appnotif.ListQuery{
		RecipientUserID: user,
		Status:          notification.StatusUnread,
		// Page / PageSize / SortOrder left default to exercise normalization
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Page)
	assert.Equal(t, 10, res.PageSize)
	repo.AssertExpectations(t)
}

func TestUnreadCountHandler(t *testing.T) {
	t.Parallel()
	user := uuid.New()
	repo := &repoMock{}
	repo.On("CountUnread", mock.Anything, user).Return(int64(7), nil).Once()
	h := appnotif.NewUnreadCountHandler(repo)
	n, err := h.Handle(context.Background(), user)
	require.NoError(t, err)
	assert.Equal(t, int64(7), n)
}
