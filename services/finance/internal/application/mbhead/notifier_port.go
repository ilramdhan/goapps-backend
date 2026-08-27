// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Notification event types for the MB recipe lifecycle. These strings travel to IAM
// as RequestNotification.event_type and are also the first segment of the idempotency
// key, so ⛔ do not rename them casually — a rename makes previously-suppressed
// duplicates deliverable again.
const (
	// EventSubmitted fires on DRAFT → SUBMITTED. Recipients: holders of
	// finance.mb.head.approve.
	EventSubmitted = "MB_HEAD_SUBMITTED"
	// EventReturnedToDraft fires on REJECTED → DRAFT only. Recipients: holders of
	// finance.mb.head.submit.
	//
	// ⛔ This event is deliberately NOT emitted on the other two roads into DRAFT
	// (grant-unlock → DRAFT, and create-new). Grant-unlock already notifies the
	// requester via EventUnlockGranted, so emitting this one there too would deliver
	// two notifications for a single user action. Pending a user decision.
	EventReturnedToDraft = "MB_HEAD_RETURNED_TO_DRAFT"
	// EventUnlockRequested fires on APPROVED/VALIDATED → UNLOCK_REQUESTED.
	// Recipients: holders of finance.mb.recipe.unlock.
	EventUnlockRequested = "MB_HEAD_UNLOCK_REQUESTED"
	// EventUnlockGranted fires on UNLOCK_REQUESTED → DRAFT. Recipient: the original
	// requester.
	EventUnlockGranted = "MB_HEAD_UNLOCK_GRANTED"
	// EventUnlockRejected fires on UNLOCK_REQUESTED → APPROVED/VALIDATED. Recipient:
	// the original requester.
	EventUnlockRejected = "MB_HEAD_UNLOCK_REJECTED"
)

// Permission codes used as BY_PERMISSION rule values.
//
// 🔴 These are the codes that actually exist in IAM's seed data (000068, 000083).
// ⛔ There is NO finance.mb.recipe.approve permission — approval of an MB head is
// gated by finance.mb.head.approve. Do not invent a new code here: a new permission
// needs an IAM migration and a user decision, in that order.
const (
	PermMBHeadSubmit   = "finance.mb.head.submit"
	PermMBHeadApprove  = "finance.mb.head.approve"
	PermMBRecipeUnlock = "finance.mb.recipe.unlock"
)

// Recipient rule types understood by IAM's resolver.
//
// 🔴 RuleBySUserID expects a UUID string — IAM does uuid.Parse on the value and the
// whole fan-out fails when it cannot parse. ⛔ Never pass a username here.
const (
	RuleByPermission = "BY_PERMISSION"
	RuleByUserID     = "BY_USER_ID"
)

// NotifRule is a single recipient-resolution rule.
type NotifRule struct {
	// RuleType is one of the Rule* constants above.
	RuleType string
	// Value is a permission code for BY_PERMISSION, or a user UUID for BY_USER_ID.
	Value string
}

// Event describes one MB recipe lifecycle notification to dispatch.
type Event struct {
	// EventType is one of the Event* constants above.
	EventType string
	// MbhID identifies the head; it is the notification source ID and the second
	// segment of the idempotency key.
	MbhID uuid.UUID
	// MBCosting is the human-readable recipe reference shown in the message body.
	MBCosting string
	// FromState and ToState record the transition that triggered the event. They
	// form the tail of the idempotency key so that a head cycling through the same
	// state twice still produces a distinct notification.
	FromState string
	ToState   string
	// Version is the head's current version at the time of the event, included in
	// the idempotency key so a re-submitted revision notifies again.
	Version int32
	// ActorUserID is the display identity (username) of whoever triggered the event.
	ActorUserID string
	// Rules resolves the recipients.
	Rules []NotifRule
}

// Notifier dispatches MB recipe lifecycle notifications. Implemented by
// iamnotifier.Notifier in the infrastructure layer — this port exists so the
// application layer never imports iamclient directly (Clean Architecture dependency
// direction).
type Notifier interface {
	NotifyEvent(ctx context.Context, event Event) error
}

// emitEvent dispatches a notification best-effort: a nil notifier is a no-op and a
// failing notifier is logged, never returned.
//
// 🔴 Notifications MUST NOT be able to fail a business transition. The transition has
// already been committed by the time this runs; surfacing an error here would tell the
// caller the transition failed when it did not.
func emitEvent(ctx context.Context, n Notifier, event Event) {
	if n == nil {
		return
	}
	if err := n.NotifyEvent(ctx, event); err != nil {
		log.Warn().Err(err).
			Str("event_type", event.EventType).
			Str("mbh_id", event.MbhID.String()).
			Msg("mbhead: notification failed (non-fatal)")
	}
}
