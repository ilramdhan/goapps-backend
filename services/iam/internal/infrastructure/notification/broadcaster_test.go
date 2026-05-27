package notification_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
	notifinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/notification"
)

func newNotif(t *testing.T, recipient uuid.UUID) *notification.Notification {
	t.Helper()
	n, err := notification.NewNotification(
		recipient,
		notification.TypeAlert, notification.SeverityInfo,
		"hi", "", notification.ActionNone, "",
		"", "", "system", nil,
	)
	require.NoError(t, err)
	return n
}

func TestBroadcaster_PublishToSubscriber(t *testing.T) {
	t.Parallel()
	b := notifinfra.NewBroadcaster()
	user := uuid.New()
	ch, unsub := b.Subscribe(user)
	defer unsub()

	n := newNotif(t, user)
	b.Publish(n)

	select {
	case got := <-ch:
		assert.Equal(t, n.ID(), got.ID())
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestBroadcaster_OnlyDeliversToOwnRecipient(t *testing.T) {
	t.Parallel()
	b := notifinfra.NewBroadcaster()
	userA, userB := uuid.New(), uuid.New()

	chA, unsubA := b.Subscribe(userA)
	defer unsubA()
	chB, unsubB := b.Subscribe(userB)
	defer unsubB()

	b.Publish(newNotif(t, userA))

	select {
	case <-chA: // ok
	case <-time.After(time.Second):
		t.Fatal("userA didn't receive its notification")
	}

	select {
	case <-chB:
		t.Fatal("userB must not receive userA's notification")
	case <-time.After(50 * time.Millisecond):
		// ok
	}
}

func TestBroadcaster_MultipleSubscribersFanOut(t *testing.T) {
	t.Parallel()
	b := notifinfra.NewBroadcaster()
	user := uuid.New()
	const subs = 5

	chans := make([]<-chan *notification.Notification, subs)
	unsubs := make([]func(), subs)
	for i := range subs {
		chans[i], unsubs[i] = b.Subscribe(user)
	}
	defer func() {
		for _, u := range unsubs {
			u()
		}
	}()

	b.Publish(newNotif(t, user))

	var wg sync.WaitGroup
	wg.Add(subs)
	for i, ch := range chans {
		go func(i int, ch <-chan *notification.Notification) {
			defer wg.Done()
			select {
			case <-ch: // ok
			case <-time.After(time.Second):
				t.Errorf("subscriber %d didn't receive", i)
			}
		}(i, ch)
	}
	wg.Wait()
}

func TestBroadcaster_UnsubscribeStopsDelivery(t *testing.T) {
	t.Parallel()
	b := notifinfra.NewBroadcaster()
	user := uuid.New()
	ch, unsub := b.Subscribe(user)

	assert.Equal(t, 1, b.SubscriberCount(user))
	unsub()
	assert.Equal(t, 0, b.SubscriberCount(user))

	// channel should be closed after unsubscribe
	_, ok := <-ch
	assert.False(t, ok, "channel must be closed after unsubscribe")
}

func TestBroadcaster_PublishNilIsSafe(t *testing.T) {
	t.Parallel()
	b := notifinfra.NewBroadcaster()
	b.Publish(nil) // should not panic
}

func TestBroadcaster_PublishToNoSubscribersIsSafe(t *testing.T) {
	t.Parallel()
	b := notifinfra.NewBroadcaster()
	b.Publish(newNotif(t, uuid.New())) // should not panic
}
