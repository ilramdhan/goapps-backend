// Package workflowinstance contains the WorkflowInstance aggregate (PRD v1.3 §0.1 D6).
package workflowinstance

import "errors"

// Sentinel errors.
var (
	ErrNotFound           = errors.New("workflow instance not found")
	ErrNoActiveTemplate   = errors.New("no active workflow template for kind")
	ErrInvalidEntityKind  = errors.New("invalid entity_kind")
	ErrInvalidStatus      = errors.New("invalid status transition")
	ErrNotInProgress      = errors.New("workflow instance is not IN_PROGRESS")
	ErrRejectNotAllowed   = errors.New("reject is not allowed at this step")
	ErrCurrentStepMissing = errors.New("current step row not found")
	ErrInvalidComment     = errors.New("comment is invalid")
)
