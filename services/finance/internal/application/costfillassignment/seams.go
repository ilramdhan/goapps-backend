package costfillassignment

import "context"

// Notifier sends SLA/overdue notifications. Implementation provided externally.
type Notifier interface {
	NotifyOverdue(ctx context.Context, taskID int64) error
}

// CompletionGate checks if all fill tasks for a request are approved and fires next state.
type CompletionGate interface {
	CheckAndAdvance(ctx context.Context, requestID int64) error
}

// FillTaskCreator creates fill tasks for all route levels of a request.
// It is called by MarkParameterPending BEFORE the state transition.
// Returns ErrConfigNotFound (wrapped) if a level has no global config.
type FillTaskCreator interface {
	CreateForRequest(ctx context.Context, requestID, productSysID, routeHeadID int64, routeLevels []int32, totalParams int32) error
}
