// Package notification provides in-memory pub/sub for the notification stream.
//
// Each authenticated user can have multiple subscribers (open SSE streams in
// different tabs). Publish fans out to all of a recipient's subscribers,
// non-blocking — slow consumers drop events and rely on the next StreamNotifications
// connect (with `since` cursor) to catch up via the database.
package notification

import (
	"sync"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
)

// Broadcaster is the in-memory event bus.
//
// For multi-replica IAM in production this should be backed by Redis pub/sub
// (or NATS). For single-replica IAM the in-memory variant is sufficient and
// missed events on subscribe are recovered from the DB via the `since` cursor.
type Broadcaster struct {
	mu   sync.RWMutex
	subs map[uuid.UUID]map[chan *notification.Notification]struct{}
}

// NewBroadcaster returns a fresh Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subs: make(map[uuid.UUID]map[chan *notification.Notification]struct{}),
	}
}

// Subscribe registers a new subscriber for `recipient`.
// Returns the receive channel and an unsubscribe function the caller must call
// (typically via defer) when the stream ends.
//
// The channel buffer is small on purpose: a slow consumer should be skipped
// rather than block other subscribers. Catchup happens on the next reconnect.
func (b *Broadcaster) Subscribe(recipient uuid.UUID) (<-chan *notification.Notification, func()) {
	const bufferSize = 16
	ch := make(chan *notification.Notification, bufferSize)

	b.mu.Lock()
	if _, ok := b.subs[recipient]; !ok {
		b.subs[recipient] = make(map[chan *notification.Notification]struct{})
	}
	b.subs[recipient][ch] = struct{}{}
	b.mu.Unlock()

	unsub := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		set, ok := b.subs[recipient]
		if !ok {
			return
		}
		delete(set, ch)
		if len(set) == 0 {
			delete(b.subs, recipient)
		}
		close(ch)
	}
	return ch, unsub
}

// Publish fans out a notification to all subscribers of its recipient.
// Non-blocking: slow consumers drop the event.
func (b *Broadcaster) Publish(n *notification.Notification) {
	if n == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	set, ok := b.subs[n.RecipientUserID()]
	if !ok {
		return
	}
	for ch := range set {
		select {
		case ch <- n:
		default:
			// drop — subscriber will resync via DB on reconnect
		}
	}
}

// SubscriberCount returns the total number of subscribers for `recipient`.
// Useful for tests and metrics.
func (b *Broadcaster) SubscriberCount(recipient uuid.UUID) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs[recipient])
}
