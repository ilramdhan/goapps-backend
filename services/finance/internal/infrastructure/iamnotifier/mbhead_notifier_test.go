package iamnotifier_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	mbheadapp "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamnotifier"
)

// captureClient records the params of the last RequestNotification call.
type captureClient struct {
	got []iamclient.RequestNotificationParams
	err error
	iamclient.NotificationClient
}

func (c *captureClient) RequestNotification(_ context.Context, p iamclient.RequestNotificationParams) error {
	c.got = append(c.got, p)
	return c.err
}

func sampleEvent(id uuid.UUID) mbheadapp.Event {
	return mbheadapp.Event{
		EventType:   mbheadapp.EventSubmitted,
		MbhID:       id,
		MBCosting:   "MB001",
		FromState:   "DRAFT",
		ToState:     "SUBMITTED",
		Version:     1,
		ActorUserID: "drafter-1",
		Rules: []mbheadapp.NotifRule{
			{RuleType: mbheadapp.RuleByPermission, Value: mbheadapp.PermMBHeadApprove},
		},
	}
}

// TestMBHeadNotifier_MapsRuleAndSource pins the wire shape: the permission rule must
// arrive as the BY_PERMISSION enum, and the source must identify the MB head.
func TestMBHeadNotifier_MapsRuleAndSource(t *testing.T) {
	client := &captureClient{}
	n := iamnotifier.NewMBHeadNotifier(client)
	id := uuid.New()

	require.NoError(t, n.NotifyEvent(context.Background(), sampleEvent(id)))

	require.Len(t, client.got, 1)
	p := client.got[0]
	assert.Equal(t, "finance", p.SourceService)
	assert.Equal(t, "mb_head", p.SourceType)
	assert.Equal(t, id.String(), p.SourceID)
	require.Len(t, p.Rules, 1)
	assert.Equal(t, iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_PERMISSION, p.Rules[0].RuleType)
	assert.Equal(t, "finance.mb.head.approve", p.Rules[0].Value)
	assert.Equal(t, iamv1.NotificationActionType_NOTIFICATION_ACTION_TYPE_NAVIGATE, p.ActionType)
	assert.Contains(t, p.ActionPayload, "/finance/mb-recipe/"+id.String())
}

// TestMBHeadNotifier_IdempotencyKey_VariesPerTransition is the anti-duplicate guarantee:
// the same dispatch retried yields the same key, while a different transition or a
// different version yields a different one.
func TestMBHeadNotifier_IdempotencyKey_VariesPerTransition(t *testing.T) {
	client := &captureClient{}
	n := iamnotifier.NewMBHeadNotifier(client)
	id := uuid.New()
	ctx := context.Background()

	base := sampleEvent(id)
	require.NoError(t, n.NotifyEvent(ctx, base))
	require.NoError(t, n.NotifyEvent(ctx, base)) // retry of the SAME dispatch

	bumped := sampleEvent(id)
	bumped.Version = 2
	require.NoError(t, n.NotifyEvent(ctx, bumped))

	other := sampleEvent(id)
	other.EventType = mbheadapp.EventUnlockRequested
	require.NoError(t, n.NotifyEvent(ctx, other))

	require.Len(t, client.got, 4)
	assert.Equal(t, client.got[0].IdempotencyKey, client.got[1].IdempotencyKey,
		"a retry of the same dispatch must reuse the key so IAM can suppress it")
	assert.NotEqual(t, client.got[0].IdempotencyKey, client.got[2].IdempotencyKey,
		"a new version must notify again")
	assert.NotEqual(t, client.got[0].IdempotencyKey, client.got[3].IdempotencyKey,
		"a different event must notify again")
}

// A ruleless event must not reach IAM at all — an empty rule set resolves to nobody.
func TestMBHeadNotifier_NoRules_DoesNotDispatch(t *testing.T) {
	client := &captureClient{}
	n := iamnotifier.NewMBHeadNotifier(client)

	e := sampleEvent(uuid.New())
	e.Rules = nil
	require.NoError(t, n.NotifyEvent(context.Background(), e))
	assert.Empty(t, client.got)
}

// The client error must be wrapped and returned so the caller can log it — the caller
// is what swallows it, not this adapter.
func TestMBHeadNotifier_ClientError_IsReturned(t *testing.T) {
	client := &captureClient{err: errors.New("boom")}
	n := iamnotifier.NewMBHeadNotifier(client)

	err := n.NotifyEvent(context.Background(), sampleEvent(uuid.New()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), mbheadapp.EventSubmitted)
}
