// Package iamclient provides gRPC client wrappers for calling the IAM service
// from finance code.
package iamclient

import "context"

// WorkflowClient is a seam for calling IAM's workflow engine.
// Finance uses this to start a workflow instance on CPR submit.
// The interface is intentionally narrow — callers should not depend on more
// than they need. The real gRPC implementation will be added in T17/T18 once
// the WorkflowInstanceService proto is generated.
type WorkflowClient interface {
	// StartInstance starts a workflow instance for a request.
	// templateCode identifies which IAM workflow template to instantiate.
	// entityType and entityID identify the requesting entity (e.g. "CPR", "123").
	// actor is the user ID initiating the action.
	// Returns the instance UUID created by IAM, or an error.
	// Callers treat this as best-effort: log on error, continue on success.
	StartInstance(ctx context.Context, templateCode, entityType, entityID, actor string) (string, error)
}

// NoopWorkflowClient is a null implementation used when IAM integration is
// disabled or not yet wired (e.g. before T17 proto generation).
type NoopWorkflowClient struct{}

// StartInstance returns empty string and nil — a safe no-op.
func (n *NoopWorkflowClient) StartInstance(_ context.Context, _, _, _, _ string) (string, error) {
	return "", nil
}
