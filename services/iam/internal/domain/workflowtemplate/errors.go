// Package workflowtemplate contains the WorkflowTemplate aggregate (PRD v1.3 §0.1 D6).
package workflowtemplate

import "errors"

// Sentinel errors for the workflowtemplate domain.
var (
	ErrNotFound          = errors.New("workflow template not found")
	ErrInvalidKind       = errors.New("invalid workflow template kind")
	ErrInvalidName       = errors.New("invalid workflow template name")
	ErrInvalidDesc       = errors.New("invalid workflow template description")
	ErrNoSteps           = errors.New("workflow template requires at least one step")
	ErrInvalidStep       = errors.New("invalid workflow template step")
	ErrInvalidResolution = errors.New("invalid approver resolution type")
	ErrStepOrder         = errors.New("step_no must start at 1 and increment by 1")
)
