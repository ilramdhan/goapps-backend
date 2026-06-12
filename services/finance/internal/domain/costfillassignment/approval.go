package costfillassignment

import "time"

// Approval decision + trigger constants.
const (
	DecisionApproved = "APPROVED"
	DecisionRejected = "REJECTED"

	TriggerInitial   = "INITIAL"
	TriggerReapprove = "REAPPROVE_ON_CHANGE"
)

// Approval is one approval/rejection event in a task's history.
type Approval struct {
	ApprovalID int64
	TaskID     int64
	Decision   string
	DecidedBy  string
	DecidedAt  time.Time
	Note       string
	Trigger    string
}
