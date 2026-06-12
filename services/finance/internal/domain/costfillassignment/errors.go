// Package costfillassignment is the fill-assignment domain: who fills and who
// approves parameters at each routing level, with global/product/request config.
package costfillassignment

import "errors"

// Sentinel errors.
var (
	// ErrInvalidActorType when a filler/approver type is not USER or DEPT.
	ErrInvalidActorType = errors.New("actor type must be USER or DEPT")
	// ErrEmptyActorValue when a filler/approver value is blank.
	ErrEmptyActorValue = errors.New("actor value must not be empty")
	// ErrConfigNotFound when no config exists for a route level in any tier.
	ErrConfigNotFound = errors.New("missing assignment config for route level")
	// ErrTaskNotFound when a fill task does not exist.
	ErrTaskNotFound = errors.New("fill task not found")
	// ErrInvalidTransition when a fill task state change is not allowed.
	ErrInvalidTransition = errors.New("invalid fill task transition")
	// ErrAlreadyClaimed when claiming a task that another user already owns.
	ErrAlreadyClaimed = errors.New("fill task already claimed")
	// ErrNoApprover when approving/rejecting a task that has no approver.
	ErrNoApprover = errors.New("fill task has no approver")
	// ErrFillIncomplete when submitting a task that still has unfilled parameters.
	ErrFillIncomplete = errors.New("all parameters must be filled before submitting")
)
