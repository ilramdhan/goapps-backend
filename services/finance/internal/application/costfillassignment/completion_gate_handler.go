package costfillassignment

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// completionChainLevels lists the L100-L102 levels in activation order.
var completionChainLevels = []int32{100, 101, 102}

// CompletionGateHandler implements CompletionGate with full completion-chain logic:
//
//  1. When all tasks with level < 100 are APPROVED → create L100/L101/L102 tasks
//     (L100 starts ACTIVE; L101 and L102 start INACTIVE).
//
//  2. When L100 is APPROVED → activate L101.
//
//  3. When L101 is APPROVED → activate L102.
//
//  4. When L102 is APPROVED → call CPRCompleter.MarkParameterComplete.
//
// If no global config exists for L100-L102 (ErrConfigNotFound), the handler skips
// the chain and immediately triggers MarkParameterComplete when all regular levels
// are approved (fast-path for deployments without completion config).
type CompletionGateHandler struct {
	taskRepo     domain.TaskRepository
	configRepo   domain.ConfigRepository
	completer    CPRCompleter       // optional; nil → MarkParameterComplete is a no-op (TODO)
	notifier     CompletionNotifier // optional; nil → notifications are skipped
	fillNotifier FillEventNotifier  // optional; nil → falls back to notifier (USER-only)
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
func (g *CompletionGateHandler) CheckAndAdvance(ctx context.Context, requestID int64, approvedLevel int32) error {
	switch approvedLevel {
	case 102:
		return g.handleL102Approved(ctx, requestID)
	case 101:
		return g.handleChainStepApproved(ctx, requestID, 102)
	case 100:
		return g.handleChainStepApproved(ctx, requestID, 101)
	default:
		return g.handleRegularLevelApproved(ctx, requestID)
	}
}

// handleL102Approved fires MarkParameterComplete on the CPR aggregate.
func (g *CompletionGateHandler) handleL102Approved(ctx context.Context, requestID int64) error {
	if g.completer == nil {
		// No completer wired — log and skip. This is a configuration gap, not a
		// business error, so we return nil to avoid blocking the approval chain.
		log.Warn().Int64("request_id", requestID).
			Msg("CompletionGateHandler: L102 approved but CPRCompleter is nil — PARAMETER_COMPLETE not triggered")
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

// handleChainStepApproved activates the next task in the L100→L101→L102 chain.
func (g *CompletionGateHandler) handleChainStepApproved(ctx context.Context, requestID int64, nextLevel int32) error {
	next, err := g.taskRepo.GetByRequestLevel(ctx, requestID, nextLevel)
	if err != nil {
		return fmt.Errorf("get completion task level %d for request %d: %w", nextLevel, requestID, err)
	}
	if err := g.taskRepo.ActivateTask(ctx, next.TaskID); err != nil {
		return fmt.Errorf("activate completion task %d (level %d): %w", next.TaskID, nextLevel, err)
	}
	g.notifyFiller(ctx, next)
	return nil
}

// handleRegularLevelApproved checks whether all regular (< 100) tasks are done
// and, if so, creates the L100-L102 completion chain (or fast-paths to completion).
func (g *CompletionGateHandler) handleRegularLevelApproved(ctx context.Context, requestID int64) error {
	remaining, err := g.taskRepo.CountNonApprovedBelow(ctx, requestID, domain.CompletionLevelStart)
	if err != nil {
		return fmt.Errorf("count non-approved regular tasks for request %d: %w", requestID, err)
	}
	if remaining > 0 {
		return nil // not yet — other regular levels are still pending
	}

	// All regular levels approved. Try to create L100-L102 from config.
	tasks, err := g.buildCompletionTasks(ctx, requestID)
	if err != nil {
		if errors.Is(err, domain.ErrConfigNotFound) {
			// No completion config — fast-path to PARAMETER_COMPLETE.
			log.Info().Int64("request_id", requestID).
				Msg("CompletionGateHandler: no completion config (L100-102); fast-pathing to MarkParameterComplete")
			return g.handleL102Approved(ctx, requestID)
		}
		return fmt.Errorf("build completion tasks for request %d: %w", requestID, err)
	}

	if err := g.taskRepo.BulkInsert(ctx, tasks); err != nil {
		return fmt.Errorf("bulk insert completion tasks for request %d: %w", requestID, err)
	}

	// Notify the L100 filler (tasks[0] is always L100 which starts ACTIVE).
	if len(tasks) > 0 {
		g.notifyFiller(ctx, tasks[0])
	}
	return nil
}

// buildCompletionTasks resolves configs for levels 100, 101, 102 and builds the Task
// slice. L100 starts ACTIVE; L101 and L102 start INACTIVE.
// Returns ErrConfigNotFound (wrapped) if any of the three levels has no global config.
func (g *CompletionGateHandler) buildCompletionTasks(ctx context.Context, requestID int64) ([]*domain.Task, error) {
	// We need the route_head_id; load the L100 task if it already exists (idempotency),
	// or derive it from any existing task for this request.
	existing, err := g.taskRepo.ListByRequest(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("list tasks for request %d: %w", requestID, err)
	}
	var routeHeadID int64
	for _, t := range existing {
		if t.RouteHeadID > 0 {
			routeHeadID = t.RouteHeadID
			break
		}
	}
	if routeHeadID == 0 {
		return nil, fmt.Errorf("cannot determine route_head_id for request %d — no existing tasks", requestID)
	}

	resolver := NewConfigResolverAdapter(g.configRepo)
	tasks := make([]*domain.Task, 0, len(completionChainLevels))
	for i, level := range completionChainLevels {
		rc, resolveErr := resolver.Resolve(ctx, 0, requestID, level)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve completion config level %d: %w", level, resolveErr)
		}
		if i == 0 {
			// L100 starts ACTIVE so it can be claimed immediately.
			tasks = append(tasks, domain.NewTask(requestID, routeHeadID, rc, 0))
		} else {
			// L101 and L102 start INACTIVE; activated sequentially by preceding approval.
			tasks = append(tasks, domain.NewInactiveTask(requestID, routeHeadID, rc, 0))
		}
	}
	return tasks, nil
}

// notifyFiller fires a best-effort notification to the task filler.
// When fillNotifier is set, it handles both USER and DEPT assignee types.
// Falls back to the legacy notifier (USER-only) when fillNotifier is nil.
func (g *CompletionGateHandler) notifyFiller(ctx context.Context, task *domain.Task) {
	if task == nil {
		return
	}
	if g.fillNotifier != nil {
		if notifyErr := g.fillNotifier.NotifyTaskActivated(ctx, task, ""); notifyErr != nil {
			log.Warn().Err(notifyErr).Int64("task_id", task.TaskID).
				Msg("CompletionGateHandler: fillNotifier.NotifyTaskActivated failed (non-fatal)")
		}
		return
	}
	if g.notifier == nil || task.FillerType != domain.ActorUser {
		return
	}
	if notifyErr := g.notifier.NotifyFiller(ctx, task.TaskID, task.FillerValue, ""); notifyErr != nil {
		log.Warn().Err(notifyErr).Int64("task_id", task.TaskID).
			Msg("CompletionGateHandler: completion notifyFiller failed (non-fatal)")
	}
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
