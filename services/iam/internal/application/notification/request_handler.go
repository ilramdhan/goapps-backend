package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
)

// notifCreator is the minimal interface RequestHandler needs from CreateHandler,
// allowing tests to stub it without importing the concrete struct.
type notifCreator interface {
	Handle(ctx context.Context, cmd CreateCommand) (*notification.Notification, error)
}

// EmailDispatcher sends an email summary for a single notification.
// Implementations must be safe to call from a background goroutine.
type EmailDispatcher interface {
	Dispatch(ctx context.Context, n *notification.Notification)
}

// EmailDispatcherFunc is a function adapter for EmailDispatcher.
type EmailDispatcherFunc func(ctx context.Context, n *notification.Notification)

// Dispatch implements EmailDispatcher.
func (f EmailDispatcherFunc) Dispatch(ctx context.Context, n *notification.Notification) {
	f(ctx, n)
}

// RecipientRuleCmd specifies one recipient resolution rule.
type RecipientRuleCmd struct {
	RuleType iamv1.RecipientRuleType
	Value    string // semantic value: user UUID string, permission code, dept code, or role name
}

// RequestCommand is the fan-out request: one notification sent to all users
// resolved by the union of all rules.
type RequestCommand struct {
	Rules         []RecipientRuleCmd
	Type          notification.Type
	Severity      notification.Severity
	Title         string
	Body          string
	ActionType    notification.ActionType
	ActionPayload string
	SourceType    string
	SourceID      string
	ExpiresAt     *time.Time
	CreatedBy     string
}

// RequestResult reports the outcome of a fan-out request.
type RequestResult struct {
	// EventID is a single UUID that groups all notifications created by this
	// fan-out request, useful for correlation in logs and audit trails.
	EventID        uuid.UUID
	RecipientCount int
}

// RequestHandler fans out a notification to all users matched by a set of
// recipient rules, deduplicating across rules before dispatching.
type RequestHandler struct {
	creator  notifCreator
	resolver UserResolver
	email    EmailDispatcher
}

// NewRequestHandler constructs a RequestHandler. email may be nil — when nil,
// no email dispatch is attempted.
func NewRequestHandler(creator notifCreator, resolver UserResolver, email EmailDispatcher) *RequestHandler {
	return &RequestHandler{creator: creator, resolver: resolver, email: email}
}

// Handle resolves all recipient rules, deduplicates the resulting user IDs,
// then calls the inner CreateHandler once per unique recipient. It returns a
// RequestResult with the event correlation ID and the number of notifications
// actually persisted.
func (h *RequestHandler) Handle(ctx context.Context, cmd RequestCommand) (RequestResult, error) {
	eventID := uuid.New()

	recipients, err := h.resolveAll(ctx, cmd.Rules)
	if err != nil {
		return RequestResult{}, fmt.Errorf("resolve recipients: %w", err)
	}

	count := 0
	for _, userID := range recipients {
		createCmd := CreateCommand{
			RecipientUserID: userID,
			Type:            cmd.Type,
			Severity:        cmd.Severity,
			Title:           cmd.Title,
			Body:            cmd.Body,
			ActionType:      cmd.ActionType,
			ActionPayload:   cmd.ActionPayload,
			SourceType:      cmd.SourceType,
			SourceID:        cmd.SourceID,
			ExpiresAt:       cmd.ExpiresAt,
			CreatedBy:       cmd.CreatedBy,
		}
		n, createErr := h.creator.Handle(ctx, createCmd)
		if createErr != nil {
			log.Warn().Err(createErr).Str("event_id", eventID.String()).
				Str("recipient_user_id", userID.String()).
				Msg("notification fan-out: failed to create notification for user")
			continue
		}
		count++
		h.dispatchEmail(n)
	}

	return RequestResult{EventID: eventID, RecipientCount: count}, nil
}

// resolveAll unions all recipient rules and deduplicates the resulting user
// IDs. Order is deterministic within each rule but not across rules.
func (h *RequestHandler) resolveAll(ctx context.Context, rules []RecipientRuleCmd) ([]uuid.UUID, error) {
	seen := make(map[uuid.UUID]struct{})
	var ordered []uuid.UUID

	for _, rule := range rules {
		ids, err := h.resolveRule(ctx, rule)
		if err != nil {
			return nil, fmt.Errorf("resolve rule %v(%q): %w", rule.RuleType, rule.Value, err)
		}
		for _, id := range ids {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			ordered = append(ordered, id)
		}
	}
	return ordered, nil
}

// resolveRule dispatches a single rule to the appropriate resolver method.
func (h *RequestHandler) resolveRule(ctx context.Context, rule RecipientRuleCmd) ([]uuid.UUID, error) {
	switch rule.RuleType {
	case iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_USER_ID:
		id, err := uuid.Parse(rule.Value)
		if err != nil {
			return nil, fmt.Errorf("parse user ID %q: %w", rule.Value, err)
		}
		return h.resolver.GetByUserID(ctx, id)
	case iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_PERMISSION:
		return h.resolver.GetByPermission(ctx, rule.Value)
	case iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_DEPT:
		return h.resolver.GetByDept(ctx, rule.Value)
	case iamv1.RecipientRuleType_RECIPIENT_RULE_TYPE_BY_ROLE:
		return h.resolver.GetByRole(ctx, rule.Value)
	default:
		return nil, fmt.Errorf("unsupported rule type: %v", rule.RuleType)
	}
}

// dispatchEmail fires the email dispatcher in a background goroutine when the
// dispatcher is configured and the notification is non-nil. Uses a separate
// context with a 30-second timeout so a slow email provider never blocks the
// caller.
func (h *RequestHandler) dispatchEmail(n *notification.Notification) {
	if h.email == nil || n == nil {
		return
	}
	go func() {
		emailCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		h.email.Dispatch(emailCtx, n)
	}()
}
