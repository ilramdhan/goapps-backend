package notification_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
)

func TestType_IsValid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   notification.Type
		want bool
	}{
		{notification.TypeExportReady, true},
		{notification.TypeAlert, true},
		{notification.TypeApproval, true},
		{notification.TypeChat, true},
		{notification.TypeReminder, true},
		{notification.TypeSystem, true},
		{notification.TypeMention, true},
		{notification.TypeAssignment, true},
		{notification.TypeAnnouncement, true},
		{notification.Type(""), false},
		{notification.Type("UNKNOWN"), false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, c.in.IsValid(), "%q", c.in)
	}
}

func TestParseType(t *testing.T) {
	t.Parallel()
	got, err := notification.ParseType("ALERT")
	require.NoError(t, err)
	assert.Equal(t, notification.TypeAlert, got)

	_, err = notification.ParseType("BOGUS")
	require.Error(t, err)
	assert.True(t, errors.Is(err, notification.ErrInvalidType))
}

func TestSeverity_IsValid(t *testing.T) {
	t.Parallel()
	for _, v := range []notification.Severity{
		notification.SeverityInfo, notification.SeveritySuccess,
		notification.SeverityWarning, notification.SeverityError,
	} {
		assert.True(t, v.IsValid(), "%q", v)
	}
	assert.False(t, notification.Severity("").IsValid())
	assert.False(t, notification.Severity("FATAL").IsValid())
}

func TestParseSeverity_Error(t *testing.T) {
	t.Parallel()
	_, err := notification.ParseSeverity("nope")
	require.Error(t, err)
	assert.True(t, errors.Is(err, notification.ErrInvalidSeverity))
}

func TestActionType_IsValid(t *testing.T) {
	t.Parallel()
	for _, v := range []notification.ActionType{
		notification.ActionNone, notification.ActionNavigate, notification.ActionDownload,
		notification.ActionExternalLink, notification.ActionApproveReject,
		notification.ActionAcknowledge, notification.ActionMultiAction,
		notification.ActionReply, notification.ActionSnooze, notification.ActionCustom,
	} {
		assert.True(t, v.IsValid(), "%q", v)
	}
	assert.False(t, notification.ActionType("").IsValid())
}

func TestParseActionType_Error(t *testing.T) {
	t.Parallel()
	_, err := notification.ParseActionType("WAT")
	require.Error(t, err)
	assert.True(t, errors.Is(err, notification.ErrInvalidActionType))
}

func TestStatus_IsValid(t *testing.T) {
	t.Parallel()
	for _, v := range []notification.Status{
		notification.StatusUnread, notification.StatusRead, notification.StatusArchived,
	} {
		assert.True(t, v.IsValid(), "%q", v)
	}
	assert.False(t, notification.Status("DROPPED").IsValid())
}

func TestParseStatus_Error(t *testing.T) {
	t.Parallel()
	_, err := notification.ParseStatus("DELETED")
	require.Error(t, err)
	assert.True(t, errors.Is(err, notification.ErrInvalidStatus))
}
