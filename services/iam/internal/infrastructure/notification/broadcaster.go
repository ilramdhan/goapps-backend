// Package notification provides in-memory pub/sub for the notification stream.
//
// Each authenticated user can have multiple subscribers (open SSE streams in
// different tabs). Publish fans out to all of a recipient's in-memory subscribers.
//
// When a Redis client is provided (via NewRedisBroadcaster), events are also
// published to a Redis pub/sub channel so that other IAM pods pick them up.
// This allows HPA to scale IAM to multiple replicas without losing SSE events.
// Same-pod events are deduplicated via the originating pod ID embedded in the
// Redis message, so subscribers receive each notification exactly once.
package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
)

const notifChannelPrefix = "iam:notif:"

// Broadcaster is the in-memory event bus.
//
// For multi-replica IAM deployments, construct it with NewRedisBroadcaster so
// that events published on one pod are received by all pods via Redis pub/sub.
// For single-replica deployments, NewBroadcaster (in-memory only) is sufficient.
type Broadcaster struct {
	mu        sync.RWMutex
	subs      map[uuid.UUID]map[chan *domain.Notification]struct{}
	rdb       *redis.Client // nil → in-memory only
	selfPodID string
}

// NewBroadcaster returns a fresh in-memory-only Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subs: make(map[uuid.UUID]map[chan *domain.Notification]struct{}),
	}
}

// NewRedisBroadcaster returns a Broadcaster backed by Redis pub/sub for
// cross-pod fan-out. Pass the same *redis.Client used for the session cache.
// If rdb is nil, falls back to in-memory behavior.
func NewRedisBroadcaster(rdb *redis.Client) *Broadcaster {
	return &Broadcaster{
		subs:      make(map[uuid.UUID]map[chan *domain.Notification]struct{}),
		rdb:       rdb,
		selfPodID: resolvePodID(),
	}
}

// resolvePodID returns the K8s pod name (os.Hostname) or a random UUID fallback.
func resolvePodID() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return uuid.New().String()
	}
	return h
}

// Subscribe registers a new subscriber for recipient.
// Returns the receive channel and an unsubscribe function the caller must invoke
// (typically via defer) when the stream ends.
//
// If a Redis client is configured, a background goroutine bridges events from
// other pods into the returned channel. Events from this pod are excluded to
// prevent duplicates (they arrive via the in-memory path already).
func (b *Broadcaster) Subscribe(recipient uuid.UUID) (<-chan *domain.Notification, func()) {
	const bufferSize = 16
	ch := make(chan *domain.Notification, bufferSize)

	b.mu.Lock()
	if _, ok := b.subs[recipient]; !ok {
		b.subs[recipient] = make(map[chan *domain.Notification]struct{})
	}
	b.subs[recipient][ch] = struct{}{}
	b.mu.Unlock()

	unsubMem := b.makeUnsubFunc(recipient, ch)

	if b.rdb == nil {
		return ch, unsubMem
	}

	cancelRedis := b.bridgeRedis(recipient, ch)
	return ch, func() {
		cancelRedis()
		unsubMem()
	}
}

// makeUnsubFunc returns a function that removes ch from the in-memory subscriber
// set for recipient and closes the channel.
func (b *Broadcaster) makeUnsubFunc(recipient uuid.UUID, ch chan *domain.Notification) func() {
	return func() {
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
}

// Publish fans out a notification to all in-memory subscribers of its recipient,
// then publishes to the Redis channel so other pods can forward it to their
// in-memory subscribers. Non-blocking: slow consumers drop the event.
func (b *Broadcaster) Publish(n *domain.Notification) {
	if n == nil {
		return
	}
	b.publishInMemory(n)
	if b.rdb != nil {
		b.publishRedis(n)
	}
}

func (b *Broadcaster) publishInMemory(n *domain.Notification) {
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

func (b *Broadcaster) publishRedis(n *domain.Notification) {
	payload, err := serializeNotif(n, b.selfPodID)
	if err != nil {
		log.Warn().Err(err).Msg("broadcaster: failed to serialize notification for Redis pub/sub")
		return
	}
	channel := notifChannelPrefix + n.RecipientUserID().String()
	if err := b.rdb.Publish(context.Background(), channel, payload).Err(); err != nil {
		log.Warn().Err(err).Msg("broadcaster: failed to publish notification to Redis")
	}
}

// bridgeRedis subscribes to the Redis pub/sub channel for recipient and
// forwards events from other pods into ch. Returns a cancel function that
// terminates the bridge goroutine.
func (b *Broadcaster) bridgeRedis(recipient uuid.UUID, ch chan *domain.Notification) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	channel := notifChannelPrefix + recipient.String()
	pubsub := b.rdb.Subscribe(ctx, channel)

	go func() {
		defer func() {
			if err := pubsub.Close(); err != nil {
				log.Warn().Err(err).Msg("broadcaster: Redis pubsub close error")
			}
		}()
		msgCh := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				b.handleRedisMessage(msg.Payload, ch)
			}
		}
	}()

	return cancel
}

func (b *Broadcaster) handleRedisMessage(payload string, ch chan *domain.Notification) {
	n, podID, err := deserializeNotif(payload)
	if err != nil {
		log.Warn().Err(err).Msg("broadcaster: failed to deserialize Redis notification")
		return
	}
	if podID == b.selfPodID {
		return // already delivered via in-memory fan-out
	}
	select {
	case ch <- n:
	default:
		// drop — subscriber will resync via DB on reconnect
	}
}

// SubscriberCount returns the total number of in-memory subscribers for recipient.
func (b *Broadcaster) SubscriberCount(recipient uuid.UUID) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs[recipient])
}

// notifMessage is the JSON wire format used for Redis pub/sub cross-pod fan-out.
type notifMessage struct {
	PodID         string  `json:"pod"`
	ID            string  `json:"id"`
	RecipientID   string  `json:"rid"`
	Type          string  `json:"type"`
	Severity      string  `json:"sev"`
	Title         string  `json:"title"`
	Body          string  `json:"body"`
	ActionType    string  `json:"at"`
	ActionPayload string  `json:"ap"`
	Status        string  `json:"status"`
	ReadAt        *string `json:"ra,omitempty"`
	ArchivedAt    *string `json:"aa,omitempty"`
	ExpiresAt     *string `json:"ea,omitempty"`
	SourceType    string  `json:"stype"`
	SourceID      string  `json:"sid"`
	CreatedAt     string  `json:"ca"`
	CreatedBy     string  `json:"cb"`
}

func serializeNotif(n *domain.Notification, podID string) (string, error) {
	msg := notifMessage{
		PodID:         podID,
		ID:            n.ID().String(),
		RecipientID:   n.RecipientUserID().String(),
		Type:          n.Type().String(),
		Severity:      n.Severity().String(),
		Title:         n.Title(),
		Body:          n.Body(),
		ActionType:    n.ActionType().String(),
		ActionPayload: n.ActionPayload(),
		Status:        n.Status().String(),
		ReadAt:        timeToPtr(n.ReadAt()),
		ArchivedAt:    timeToPtr(n.ArchivedAt()),
		ExpiresAt:     timeToPtr(n.ExpiresAt()),
		SourceType:    n.SourceType(),
		SourceID:      n.SourceID(),
		CreatedAt:     n.CreatedAt().UTC().Format(time.RFC3339Nano),
		CreatedBy:     n.CreatedBy(),
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("marshal notif: %w", err)
	}
	return string(b), nil
}

func deserializeNotif(payload string) (*domain.Notification, string, error) {
	var msg notifMessage
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		return nil, "", fmt.Errorf("unmarshal notif: %w", err)
	}
	id, err := uuid.Parse(msg.ID)
	if err != nil {
		return nil, "", fmt.Errorf("parse notif id: %w", err)
	}
	rid, err := uuid.Parse(msg.RecipientID)
	if err != nil {
		return nil, "", fmt.Errorf("parse recipient id: %w", err)
	}
	typ, err := domain.ParseType(msg.Type)
	if err != nil {
		return nil, "", fmt.Errorf("parse type: %w", err)
	}
	sev, err := domain.ParseSeverity(msg.Severity)
	if err != nil {
		return nil, "", fmt.Errorf("parse severity: %w", err)
	}
	at, err := domain.ParseActionType(msg.ActionType)
	if err != nil {
		return nil, "", fmt.Errorf("parse action type: %w", err)
	}
	st, err := domain.ParseStatus(msg.Status)
	if err != nil {
		return nil, "", fmt.Errorf("parse status: %w", err)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, msg.CreatedAt)
	if err != nil {
		return nil, "", fmt.Errorf("parse created_at: %w", err)
	}
	n := domain.Reconstruct(
		id, rid, typ, sev,
		msg.Title, msg.Body,
		at, msg.ActionPayload, st,
		ptrToTime(msg.ReadAt), ptrToTime(msg.ArchivedAt), ptrToTime(msg.ExpiresAt),
		msg.SourceType, msg.SourceID,
		createdAt, msg.CreatedBy,
	)
	return n, msg.PodID, nil
}

func timeToPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

func ptrToTime(s *string) *time.Time {
	if s == nil {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, *s)
	if err != nil {
		return nil
	}
	return &t
}
