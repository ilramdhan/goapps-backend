package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
	notifinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/notification"
)

// StreamEvent is an event delivered through the stream.
type StreamEvent struct {
	EventID      string                     // sortable resume cursor
	Notification *notification.Notification // nil when IsHeartbeat
	IsHeartbeat  bool
}

// StreamHandler subscribes to a recipient's notification stream and emits
// catchup events from the DB followed by realtime events from the broadcaster.
//
// The handler is consumed by the gRPC layer which forwards each StreamEvent
// to the gRPC server-streaming response and (in turn) to the BFF SSE bridge.
type StreamHandler struct {
	repo            notification.Repository
	broadcaster     *notifinfra.Broadcaster
	heartbeatPeriod time.Duration
}

// NewStreamHandler constructs the handler. Pass heartbeatPeriod=0 to disable
// heartbeats (default 30s when zero is passed via the public ctor).
func NewStreamHandler(repo notification.Repository, b *notifinfra.Broadcaster, heartbeatPeriod time.Duration) *StreamHandler {
	if heartbeatPeriod <= 0 {
		heartbeatPeriod = 30 * time.Second
	}
	return &StreamHandler{repo: repo, broadcaster: b, heartbeatPeriod: heartbeatPeriod}
}

// Handle drives the stream lifecycle. The emit callback is invoked for each
// event that should be sent to the client; if it returns an error the stream
// terminates. Returns when ctx is canceled or emit fails.
//
// `since` is an optional cursor matching a previous EventID; pass empty string
// to skip catchup and only receive realtime events.
func (h *StreamHandler) Handle(
	ctx context.Context,
	recipient uuid.UUID,
	since string,
	emit func(StreamEvent) error,
) error {
	if recipient == uuid.Nil {
		return notification.ErrEmptyRecipient
	}

	// Subscribe FIRST so events published while we run catchup aren't lost.
	// Capture a cutoff cursor immediately after subscribe — any DB row with a
	// (created_at, id) <= cutoff is delivered via replay; rows after cutoff
	// arrive via the broadcaster channel. This avoids both gaps AND duplicates.
	ch, unsub := h.broadcaster.Subscribe(recipient)
	defer unsub()
	cutoffTime := time.Now().UTC()
	cutoffID := uuid.Max // sentinel: all real ids are < uuid.Max lexicographically

	lastTime, lastID, err := h.runCatchup(ctx, recipient, since, cutoffTime, cutoffID, emit)
	if err != nil {
		return err
	}

	tick := time.NewTicker(h.heartbeatPeriod)
	defer tick.Stop()

	return h.realtimeLoop(ctx, ch, tick.C, lastTime, lastID, emit)
}

// runCatchup replays missed events from `since` cursor up to the just-captured
// cutoff. Returns the last (time, id) actually emitted so the realtime loop
// can dedup broadcaster events against it.
func (h *StreamHandler) runCatchup(
	ctx context.Context,
	recipient uuid.UUID,
	since string,
	cutoffTime time.Time, cutoffID uuid.UUID,
	emit func(StreamEvent) error,
) (time.Time, uuid.UUID, error) {
	if since == "" {
		return time.Time{}, uuid.Nil, nil
	}
	afterTime, afterID, err := decodeCursor(since)
	if err != nil {
		// Bad/unknown cursor: intentionally swallow the parse error and start
		// realtime delivery without replay — the client may send a stale or
		// malformed Last-Event-ID; treating it as "skip catchup" is far better
		// than tearing the stream down.
		return time.Time{}, uuid.Nil, nil //nolint:nilerr // intentional: skip catchup on bad cursor
	}
	return h.replay(ctx, recipient, afterTime, afterID, cutoffTime, cutoffID, emit)
}

// realtimeLoop pumps events from the broadcaster channel + heartbeat ticker
// until the context is canceled or emit returns an error.
func (h *StreamHandler) realtimeLoop(
	ctx context.Context,
	ch <-chan *notification.Notification,
	tick <-chan time.Time,
	lastTime time.Time, lastID uuid.UUID,
	emit func(StreamEvent) error,
) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case n, ok := <-ch:
			if !ok {
				return nil // unsubscribed
			}
			if !cursorAfter(n.CreatedAt(), n.ID(), lastTime, lastID) {
				continue // already delivered via replay
			}
			if err := emit(StreamEvent{EventID: encodeCursor(n.CreatedAt(), n.ID()), Notification: n}); err != nil {
				return fmt.Errorf("emit notification: %w", err)
			}
			lastTime, lastID = n.CreatedAt(), n.ID()
		case t := <-tick:
			if err := emit(StreamEvent{EventID: encodeCursor(t, uuid.Nil), IsHeartbeat: true}); err != nil {
				return fmt.Errorf("emit heartbeat: %w", err)
			}
		}
	}
}

// cursorAfter reports whether (a, aID) sorts strictly after (b, bID). Used to
// dedup broadcaster events against the last replayed cursor.
func cursorAfter(a time.Time, aID uuid.UUID, b time.Time, bID uuid.UUID) bool {
	if b.IsZero() {
		return true
	}
	if a.After(b) {
		return true
	}
	if a.Equal(b) {
		return aID.String() > bID.String()
	}
	return false
}

// replay walks DB rows newer than (afterTime, afterID) up to (cutoffTime, cutoffID)
// in ascending order and emits each. Returns the last emitted cursor so the
// realtime loop can dedup against the broadcaster channel.
//
// Note: the repository's After filter is timestamp-only (created_at > after) —
// rows sharing the exact afterTime are included; we filter sub-second collisions
// with afterID in-memory below, which is safe given the small page window.
func (h *StreamHandler) replay(
	ctx context.Context,
	recipient uuid.UUID,
	afterTime time.Time, afterID uuid.UUID,
	cutoffTime time.Time, cutoffID uuid.UUID,
	emit func(StreamEvent) error,
) (lastTime time.Time, lastID uuid.UUID, err error) {
	page := 1
	const pageSize = 100
	for {
		items, _, qErr := h.repo.ListByRecipient(ctx, recipient, notification.ListFilter{
			Page:     page,
			PageSize: pageSize,
			After:    &afterTime,
			SortDesc: false, // ascending — oldest first so cursor is monotonic
		})
		if qErr != nil {
			return lastTime, lastID, fmt.Errorf("catchup query: %w", qErr)
		}
		for _, n := range items {
			// Skip rows already delivered (same key or older than `since`).
			if !cursorAfter(n.CreatedAt(), n.ID(), afterTime, afterID) {
				continue
			}
			// Stop at cutoff so broadcaster takes over without overlap.
			if cursorAfter(n.CreatedAt(), n.ID(), cutoffTime, cutoffID) {
				return lastTime, lastID, nil
			}
			if eErr := emit(StreamEvent{EventID: encodeCursor(n.CreatedAt(), n.ID()), Notification: n}); eErr != nil {
				return lastTime, lastID, eErr
			}
			lastTime, lastID = n.CreatedAt(), n.ID()
		}
		if len(items) < pageSize {
			return lastTime, lastID, nil
		}
		page++
	}
}

// encodeCursor produces a sortable string cursor: "RFC3339Nano|uuid".
// Heartbeats use uuid.Nil so they're distinguishable but still sortable.
func encodeCursor(t time.Time, id uuid.UUID) string {
	return t.UTC().Format(time.RFC3339Nano) + "|" + id.String()
}

// decodeCursor parses a cursor produced by encodeCursor and returns the
// embedded (timestamp, id) pair so the catchup query can use a composite
// (created_at, id) > (afterTime, afterID) condition without skipping rows
// that share an exact timestamp.
func decodeCursor(s string) (time.Time, uuid.UUID, error) {
	for i, c := range s {
		if c == '|' {
			t, err := time.Parse(time.RFC3339Nano, s[:i])
			if err != nil {
				return time.Time{}, uuid.Nil, fmt.Errorf("parse cursor time: %w", err)
			}
			id, err := uuid.Parse(s[i+1:])
			if err != nil {
				return time.Time{}, uuid.Nil, fmt.Errorf("parse cursor id: %w", err)
			}
			return t, id, nil
		}
	}
	return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor: %q", s)
}
