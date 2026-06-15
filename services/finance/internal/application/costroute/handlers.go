// Package costroute holds use cases for the persisted routing DAG
// (cost_route_head + cost_route_seq + cost_route_rm).
package costroute

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// ParamCompletenessChecker counts unfilled required params for a route.
type ParamCompletenessChecker interface {
	CountUnfilledParams(ctx context.Context, headID int64) (int, error)
}

// RouteNotifier emits lock/unlock notifications for linked CPRs.
type RouteNotifier interface {
	NotifyRouteLocked(ctx context.Context, headID int64, actorID, actorName string) error
	NotifyRouteUnlocked(ctx context.Context, headID int64, actorID, actorName string) error
}

// GetByProductHandler returns the active head for a product.
type GetByProductHandler struct {
	repo costroute.Repository
}

// NewGetByProductHandler constructs a GetByProductHandler.
func NewGetByProductHandler(repo costroute.Repository) *GetByProductHandler {
	return &GetByProductHandler{repo: repo}
}

// Handle executes the lookup.
func (h *GetByProductHandler) Handle(ctx context.Context, productSysID int64) (*costroute.Head, error) {
	return h.repo.GetActiveByProduct(ctx, productSysID)
}

// GetGraphHandler returns the full graph for a head.
type GetGraphHandler struct {
	repo costroute.Repository
}

// NewGetGraphHandler constructs a GetGraphHandler.
func NewGetGraphHandler(repo costroute.Repository) *GetGraphHandler {
	return &GetGraphHandler{repo: repo}
}

// Handle loads the graph.
func (h *GetGraphHandler) Handle(ctx context.Context, headID int64) (*costroute.Graph, error) {
	return h.repo.GetGraph(ctx, headID)
}

// SaveGraphHandler validates + persists the entire graph in one tx.
type SaveGraphHandler struct {
	repo costroute.Repository
}

// NewSaveGraphHandler constructs a SaveGraphHandler.
func NewSaveGraphHandler(repo costroute.Repository) *SaveGraphHandler {
	return &SaveGraphHandler{repo: repo}
}

// Handle runs level-discipline validation then bulk-saves.
func (h *SaveGraphHandler) Handle(ctx context.Context, headID int64, g *costroute.Graph, actor string) (*costroute.Graph, error) {
	if g == nil {
		return nil, fmt.Errorf("save graph: nil payload")
	}
	if g.Head == nil {
		// Caller may not have populated head; load it for context.
		loaded, err := h.repo.GetHead(ctx, headID)
		if err != nil {
			return nil, err
		}
		g.Head = loaded
	}
	if g.Head.IsLocked() {
		return nil, costroute.ErrLocked
	}
	if err := g.ValidateLevels(); err != nil {
		return nil, err
	}
	return h.repo.SaveGraph(ctx, headID, g, actor)
}

// MarkCompleteHandler transitions the head DRAFT -> COMPLETE.
type MarkCompleteHandler struct {
	repo costroute.Repository
}

// NewMarkCompleteHandler constructs a MarkCompleteHandler.
func NewMarkCompleteHandler(repo costroute.Repository) *MarkCompleteHandler {
	return &MarkCompleteHandler{repo: repo}
}

// Handle marks the head COMPLETE.
func (h *MarkCompleteHandler) Handle(ctx context.Context, headID int64, actor string) (*costroute.Head, error) {
	head, err := h.repo.GetHead(ctx, headID)
	if err != nil {
		return nil, err
	}
	if err := head.MarkComplete(); err != nil {
		return nil, err
	}
	if err := h.repo.SaveHead(ctx, head, actor); err != nil {
		return nil, err
	}
	return head, nil
}

// LockHandler transitions COMPLETE -> LOCKED, optionally checking param completeness.
type LockHandler struct {
	repo     costroute.Repository
	checker  ParamCompletenessChecker
	notifier RouteNotifier
}

// NewLockHandler constructs a LockHandler.
func NewLockHandler(repo costroute.Repository) *LockHandler {
	return &LockHandler{repo: repo}
}

// WithParamChecker attaches an optional param completeness checker.
func (h *LockHandler) WithParamChecker(c ParamCompletenessChecker) *LockHandler {
	h.checker = c
	return h
}

// WithNotifier attaches an optional route notifier.
func (h *LockHandler) WithNotifier(n RouteNotifier) *LockHandler {
	h.notifier = n
	return h
}

// Handle locks the head, optionally checking param completeness first.
func (h *LockHandler) Handle(ctx context.Context, headID int64, actorID, actorName string) (*costroute.Head, error) {
	if h.checker != nil {
		unfilled, err := h.checker.CountUnfilledParams(ctx, headID)
		if err != nil {
			return nil, fmt.Errorf("check params: %w", err)
		}
		if unfilled > 0 {
			return nil, fmt.Errorf("%w: %d required params are empty", costroute.ErrParamIncomplete, unfilled)
		}
	}
	head, err := h.repo.GetHead(ctx, headID)
	if err != nil {
		return nil, err
	}
	if err := head.Lock(actorID); err != nil {
		return nil, err
	}
	if err := h.repo.SaveHead(ctx, head, actorID); err != nil {
		return nil, err
	}
	if h.notifier != nil {
		if notifyErr := h.notifier.NotifyRouteLocked(ctx, headID, actorID, actorName); notifyErr != nil {
			log.Warn().Err(notifyErr).Int64("head_id", headID).Msg("LockHandler: notify locked failed (non-blocking)")
		}
	}
	return head, nil
}

// UnlockHandler transitions LOCKED -> COMPLETE and records the actor.
type UnlockHandler struct {
	repo     costroute.Repository
	notifier RouteNotifier
}

// NewUnlockHandler constructs an UnlockHandler.
func NewUnlockHandler(repo costroute.Repository) *UnlockHandler {
	return &UnlockHandler{repo: repo}
}

// WithNotifier attaches an optional route notifier.
func (h *UnlockHandler) WithNotifier(n RouteNotifier) *UnlockHandler {
	h.notifier = n
	return h
}

// Handle unlocks the head.
func (h *UnlockHandler) Handle(ctx context.Context, headID int64, actorID, actorName string) (*costroute.Head, error) {
	head, err := h.repo.GetHead(ctx, headID)
	if err != nil {
		return nil, err
	}
	if err := head.Unlock(actorID); err != nil {
		return nil, err
	}
	if err := h.repo.SaveHead(ctx, head, actorID); err != nil {
		return nil, err
	}
	if h.notifier != nil {
		if notifyErr := h.notifier.NotifyRouteUnlocked(ctx, headID, actorID, actorName); notifyErr != nil {
			log.Warn().Err(notifyErr).Int64("head_id", headID).Msg("UnlockHandler: notify unlocked failed (non-blocking)")
		}
	}
	return head, nil
}

// DeleteHandler soft-deletes the head.
type DeleteHandler struct {
	repo costroute.Repository
}

// NewDeleteHandler constructs a DeleteHandler.
func NewDeleteHandler(repo costroute.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle deletes the head (refuses if LOCKED).
func (h *DeleteHandler) Handle(ctx context.Context, headID int64, actor string) error {
	head, err := h.repo.GetHead(ctx, headID)
	if err != nil {
		return err
	}
	if head.IsLocked() {
		return costroute.ErrLocked
	}
	return h.repo.DeleteHead(ctx, headID, actor)
}

// ListHandler returns paginated heads with optional filters.
type ListHandler struct {
	repo costroute.Repository
}

// NewListHandler constructs a ListHandler.
func NewListHandler(repo costroute.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the list.
func (h *ListHandler) Handle(ctx context.Context, f costroute.Filter) ([]*costroute.Head, int64, error) {
	return h.repo.ListHeads(ctx, f)
}
