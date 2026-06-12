package costfillassignment

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// CompletionGateHandler implements CompletionGate.
//
// When all fill tasks with level < 100 are APPROVED it calls
// CPRCompleter.MarkParameterComplete directly. The L100-L102 completion chain
// has been removed (migration 000373 deletes those config rows); CONFIRM/APPROVE/
// RELEASE are now driven by dedicated CPR domain methods with their own permissions.
type CompletionGateHandler struct {
	taskRepo     domain.TaskRepository
	configRepo   domain.ConfigRepository // kept for interface compatibility; not used by gate
	completer    CPRCompleter            // optional; nil → MarkParameterComplete is a no-op (logged)
	notifier     CompletionNotifier      // optional; nil → notifications are skipped
	fillNotifier FillEventNotifier       // optional; nil → falls back to notifier (USER-only)
}

// NewCompletionGateHandler constructs the gate. configRepo is required.
// completer and notifier are optional (nil-safe, best-effort).
func NewCompletionGateHandler(
	taskRepo domain.TaskRepository,
	configRepo domain.ConfigRepository,
	completer CPRCompleter,
	notifier CompletionNotifier,
) *CompletionGateHandler {
	return &CompletionGateHandler{
		taskRepo:   taskRepo,
		configRepo: configRepo,
		completer:  completer,
		notifier:   notifier,
	}
}

var _ CompletionGate = (*CompletionGateHandler)(nil)

// WithFillNotifier attaches a FillEventNotifier. Returns receiver for chaining.
func (g *CompletionGateHandler) WithFillNotifier(fn FillEventNotifier) *CompletionGateHandler {
	g.fillNotifier = fn
	return g
}

// CheckAndAdvance is called after a task is approved.
// approvedLevel is the route_level of the task that just got approved.
// All levels funnel through the same gate: check whether every task below the
// completion threshold is approved, and if so trigger MarkParameterComplete.
func (g *CompletionGateHandler) CheckAndAdvance(ctx context.Context, requestID int64, approvedLevel int32) error {
	_ = approvedLevel // level used only for routing; all paths lead to the same gate
	return g.handleLevelApproved(ctx, requestID)
}

// handleLevelApproved checks whether all fill tasks (level < CompletionLevelStart)
// are approved. When they are, it calls MarkParameterComplete on the CPR aggregate.
func (g *CompletionGateHandler) handleLevelApproved(ctx context.Context, requestID int64) error {
	remaining, err := g.taskRepo.CountNonApprovedBelow(ctx, requestID, domain.CompletionLevelStart)
	if err != nil {
		return fmt.Errorf("count non-approved regular tasks for request %d: %w", requestID, err)
	}
	if remaining > 0 {
		return nil // other regular levels are still pending
	}

	// All regular levels approved — trigger completion directly.
	return g.triggerComplete(ctx, requestID)
}

// triggerComplete calls MarkParameterComplete and fires a best-effort completion
// notification to the requester.
func (g *CompletionGateHandler) triggerComplete(ctx context.Context, requestID int64) error {
	if g.completer == nil {
		log.Warn().Int64("request_id", requestID).
			Msg("CompletionGateHandler: all levels approved but CPRCompleter is nil — PARAMETER_COMPLETE not triggered")
		return nil
	}
	requesterID, requestNo, err := g.completer.MarkParameterComplete(ctx, requestID, "system")
	if err != nil {
		return fmt.Errorf("mark parameter complete for request %d: %w", requestID, err)
	}
	if (g.fillNotifier != nil || g.notifier != nil) && requesterID != "" {
		g.notifyComplete(ctx, requestID, requesterID, requestNo)
	}
	return nil
}

// notifyComplete fires a best-effort STATUS_CHANGE notification to the requester.
func (g *CompletionGateHandler) notifyComplete(ctx context.Context, requestID int64, requesterID, requestNo string) {
	if g.fillNotifier != nil {
		if notifyErr := g.fillNotifier.NotifyAllApproved(ctx, requestID, requesterID, requestNo); notifyErr != nil {
			log.Warn().Err(notifyErr).Int64("request_id", requestID).
				Msg("CompletionGateHandler: fillNotifier.NotifyAllApproved failed (non-fatal)")
		}
		return
	}
	if notifyErr := g.notifier.NotifyComplete(ctx, requestID, requesterID, requestNo); notifyErr != nil {
		log.Warn().Err(notifyErr).Int64("request_id", requestID).
			Msg("CompletionGateHandler: completion notifyComplete failed (non-fatal)")
	}
}
