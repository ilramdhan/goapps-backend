// Package notification defines the nil-safe notifier hook used by planning
// usecases. The concrete IAM-backed implementation is wired in a later phase;
// usecases hold a Notifier interface value that may be nil (no-op).
package notification

import "context"

// Event identifies a planning notification kind (PRD page 2 notification matrix).
type Event string

// Notification events.
const (
	// EventUnmatchedSO fires when an Orion SO remains unpulled.
	EventUnmatchedSO Event = "UNMATCHED_SO"
	// EventMTSRequested fires when an MTS demand needs Marketing approval.
	EventMTSRequested Event = "MTS_REQUESTED"
	// EventMTSDecided fires when Marketing approves/rejects an MTS demand.
	EventMTSDecided Event = "MTS_DECIDED"
	// EventWOSubmitted fires when a WO is submitted (notify PC + PM).
	EventWOSubmitted Event = "WO_SUBMITTED"
	// EventWOPCApproved fires when PC approves (notify PM).
	EventWOPCApproved Event = "WO_PC_APPROVED"
	// EventWOApproved fires when a WO is fully approved (notify PPC).
	EventWOApproved Event = "WO_APPROVED"
	// EventWORejected fires when a WO is rejected (notify PPC).
	EventWORejected Event = "WO_REJECTED"
	// EventRMFenceWarning fires when RM allocation nears the fence (notify PPC).
	EventRMFenceWarning Event = "RM_FENCE_WARNING"
	// EventRMFenceBlocked fires when RM allocation exceeds the fence (notify PPC + PM).
	EventRMFenceBlocked Event = "RM_FENCE_BLOCKED"
	// EventBFSCommodityWatch fires when a commodity-watch product's balance-for-sale
	// crosses a threshold (notify Marketing + PPC).
	EventBFSCommodityWatch Event = "BFS_COMMODITY_WATCH"
)

// Message is a single notification payload. Recipients are role codes.
type Message struct {
	Event      Event
	Subject    string
	Body       string
	Recipients []string
	EntityID   int64
}

// Notifier delivers planning notifications. Implementations must be safe to call
// concurrently and must never block the caller for long. A nil Notifier value is
// treated as a no-op by callers via the Notify helper.
type Notifier interface {
	// Notify delivers a notification. Errors are advisory (best-effort delivery).
	Notify(ctx context.Context, msg Message) error
}

// Notify sends msg through n when n is non-nil, swallowing any error so a
// notification failure never breaks the calling usecase. It is the single entry
// point usecases should use so the nil-Notifier case stays centralized.
func Notify(ctx context.Context, n Notifier, msg Message) {
	if n == nil {
		return
	}
	_ = n.Notify(ctx, msg) //nolint:errcheck // best-effort delivery; failures are non-fatal
}
