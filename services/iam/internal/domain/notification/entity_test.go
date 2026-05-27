package notification_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
)

func newValidNotification(t *testing.T) *notification.Notification {
	t.Helper()
	n, err := notification.NewNotification(
		uuid.New(),
		notification.TypeExportReady,
		notification.SeveritySuccess,
		"Export selesai",
		"File siap di-download",
		notification.ActionDownload,
		`{"file_path":"x"}`,
		"finance.rm_cost_export",
		uuid.NewString(),
		"system",
		nil,
	)
	require.NoError(t, err)
	return n
}

func TestNewNotification_Valid(t *testing.T) {
	t.Parallel()
	n := newValidNotification(t)
	assert.Equal(t, notification.StatusUnread, n.Status())
	assert.Nil(t, n.ReadAt())
	assert.Nil(t, n.ArchivedAt())
	assert.NotEqual(t, uuid.Nil, n.ID())
	assert.Equal(t, "system", n.CreatedBy())
}

func TestNewNotification_RecipientRequired(t *testing.T) {
	t.Parallel()
	_, err := notification.NewNotification(
		uuid.Nil,
		notification.TypeAlert, notification.SeverityInfo,
		"hi", "", notification.ActionNone, "", "", "", "", nil,
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, notification.ErrEmptyRecipient))
}

func TestNewNotification_TitleRequired(t *testing.T) {
	t.Parallel()
	_, err := notification.NewNotification(
		uuid.New(),
		notification.TypeAlert, notification.SeverityInfo,
		"", "", notification.ActionNone, "", "", "", "", nil,
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, notification.ErrEmptyTitle))
}

func TestNewNotification_InvalidEnum(t *testing.T) {
	t.Parallel()
	_, err := notification.NewNotification(
		uuid.New(),
		notification.Type("BOGUS"), notification.SeverityInfo,
		"hi", "", notification.ActionNone, "", "", "", "", nil,
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, notification.ErrInvalidType))
}

func TestMarkAsRead(t *testing.T) {
	t.Parallel()
	n := newValidNotification(t)
	n.MarkAsRead()
	assert.Equal(t, notification.StatusRead, n.Status())
	require.NotNil(t, n.ReadAt())

	prev := *n.ReadAt()
	time.Sleep(time.Millisecond)
	n.MarkAsRead() // idempotent
	assert.Equal(t, prev, *n.ReadAt(), "second MarkAsRead must be a no-op")
}

func TestArchive(t *testing.T) {
	t.Parallel()
	n := newValidNotification(t)
	require.NoError(t, n.Archive())
	assert.Equal(t, notification.StatusArchived, n.Status())
	require.NotNil(t, n.ArchivedAt())
	require.NotNil(t, n.ReadAt(), "archiving an unread sets read_at too")

	err := n.Archive()
	require.Error(t, err)
	assert.True(t, errors.Is(err, notification.ErrAlreadyArchived))
}

func TestIsExpired(t *testing.T) {
	t.Parallel()
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	n1 := newValidNotification(t)
	assert.False(t, n1.IsExpired(now), "no expires_at = never expired")

	n2 := notification.Reconstruct(uuid.New(), uuid.New(),
		notification.TypeAlert, notification.SeverityInfo,
		"t", "", notification.ActionNone, "",
		notification.StatusUnread, nil, nil, &past,
		"", "", now, "system",
	)
	assert.True(t, n2.IsExpired(now))

	n3 := notification.Reconstruct(uuid.New(), uuid.New(),
		notification.TypeAlert, notification.SeverityInfo,
		"t", "", notification.ActionNone, "",
		notification.StatusUnread, nil, nil, &future,
		"", "", now, "system",
	)
	assert.False(t, n3.IsExpired(now))
}
