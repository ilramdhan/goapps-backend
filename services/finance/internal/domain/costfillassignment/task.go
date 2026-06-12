package costfillassignment

import "time"

// Fill task statuses.
const (
	StatusInactive        = "INACTIVE"
	StatusActive          = "ACTIVE"
	StatusFilling         = "FILLING"
	StatusFilled          = "FILLED"
	StatusApprovalPending = "APPROVAL_PENDING"
	StatusApproved        = "APPROVED"
	StatusRejected        = "REJECTED"
)

// Completion-chain level constants (L100–L102 are the post-routing sign-off steps).
const (
	CompletionLevelStart = int32(100) // first completion level; all regular levels must be < 100
	CompletionLevelFin   = int32(102) // last completion level; approving this triggers PARAMETER_COMPLETE
)

// Task is one fill task = one (request × route_level). Config is snapshotted at
// creation and immutable thereafter.
type Task struct {
	TaskID            int64
	RequestID         int64
	RouteHeadID       int64
	RouteLevel        int32
	FillerType        string
	FillerValue       string
	ApproverType      string
	ApproverValue     string
	ReapproveOnChange bool
	SLAFillHours      int32
	SLAApproveHours   int32
	status            string
	TotalParams       int32
	FilledParams      int32
	ClaimedBy         string
	ClaimedAt         *time.Time
	FilledAt          *time.Time
	ActivatedAt       time.Time
	LastNotifiedAt    *time.Time
}

// NewTask builds a fresh ACTIVE task from a resolved config snapshot.
func NewTask(requestID, routeHeadID int64, rc ResolvedConfig, totalParams int32) *Task {
	return &Task{
		RequestID:         requestID,
		RouteHeadID:       routeHeadID,
		RouteLevel:        rc.RouteLevel,
		FillerType:        rc.FillerType,
		FillerValue:       rc.FillerValue,
		ApproverType:      rc.ApproverType,
		ApproverValue:     rc.ApproverValue,
		ReapproveOnChange: rc.ReapproveOnChange,
		SLAFillHours:      rc.SLAFillHours,
		SLAApproveHours:   rc.SLAApproveHours,
		status:            StatusActive,
		TotalParams:       totalParams,
	}
}

// NewInactiveTask builds a completion-chain task (L101, L102) that starts INACTIVE.
// It is activated by CompletionGate when the preceding level is approved.
func NewInactiveTask(requestID, routeHeadID int64, rc ResolvedConfig, totalParams int32) *Task {
	t := NewTask(requestID, routeHeadID, rc, totalParams)
	t.status = StatusInactive
	return t
}

// Hydrate rebuilds a Task from persisted columns (repository use only).
func Hydrate(taskID, requestID, routeHeadID int64, routeLevel int32,
	fillerType, fillerValue, approverType, approverValue, status, claimedBy string,
	reapprove bool, slaFill, slaApprove, total, filled int32, activatedAt time.Time, filledAt *time.Time) *Task {
	return &Task{
		TaskID: taskID, RequestID: requestID, RouteHeadID: routeHeadID, RouteLevel: routeLevel,
		FillerType: fillerType, FillerValue: fillerValue, ApproverType: approverType, ApproverValue: approverValue,
		ReapproveOnChange: reapprove, SLAFillHours: slaFill, SLAApproveHours: slaApprove,
		status: status, TotalParams: total, FilledParams: filled, ClaimedBy: claimedBy, ActivatedAt: activatedAt,
		FilledAt: filledAt,
	}
}

// Status returns the current state.
func (t *Task) Status() string { return t.status }

// HasApprover reports whether an approver is configured.
func (t *Task) HasApprover() bool { return t.ApproverType != "" }

// SetReapproveOnChange is a test/seed convenience for the snapshot flag.
func (t *Task) SetReapproveOnChange(v bool) { t.ReapproveOnChange = v }

// Claim marks the task FILLING owned by userID. Allowed only from ACTIVE.
func (t *Task) Claim(userID string) error {
	if t.status != StatusActive {
		return ErrInvalidTransition
	}
	now := time.Now()
	t.ClaimedBy = userID
	t.ClaimedAt = &now
	t.status = StatusFilling
	return nil
}

// Submit advances a FILLING task: APPROVAL_PENDING if it has an approver,
// otherwise straight to APPROVED. All parameters must be filled first.
func (t *Task) Submit() error {
	if t.status != StatusFilling {
		return ErrInvalidTransition
	}
	if t.TotalParams > 0 && t.FilledParams < t.TotalParams {
		return ErrFillIncomplete
	}
	now := time.Now()
	t.FilledAt = &now
	if t.HasApprover() {
		t.status = StatusApprovalPending
	} else {
		t.status = StatusApproved
	}
	return nil
}

// Approve moves APPROVAL_PENDING → APPROVED.
func (t *Task) Approve(_ string) error {
	if t.status != StatusApprovalPending {
		return ErrInvalidTransition
	}
	if !t.HasApprover() {
		return ErrNoApprover
	}
	t.status = StatusApproved
	return nil
}

// Reject moves APPROVAL_PENDING → REJECTED.
func (t *Task) Reject(_, _ string) error {
	if t.status != StatusApprovalPending {
		return ErrInvalidTransition
	}
	if !t.HasApprover() {
		return ErrNoApprover
	}
	t.status = StatusRejected
	return nil
}

// Resubmit moves REJECTED → FILLING so the filler can revise.
func (t *Task) Resubmit() error {
	if t.status != StatusRejected {
		return ErrInvalidTransition
	}
	t.status = StatusFilling
	return nil
}

// MarkEditedAfterApproval handles the reapprove_on_change case: editing an
// APPROVED task's params re-opens approval for this level only.
func (t *Task) MarkEditedAfterApproval() error {
	if t.status != StatusApproved {
		return ErrInvalidTransition
	}
	if t.ReapproveOnChange && t.HasApprover() {
		t.status = StatusApprovalPending
	}
	return nil
}

// IsOverdue reports whether the fill SLA window has elapsed for an unfinished task.
func (t *Task) IsOverdue(now time.Time) bool {
	if t.status == StatusApproved {
		return false
	}
	deadline := t.ActivatedAt.Add(time.Duration(t.SLAFillHours) * time.Hour)
	return now.After(deadline)
}
