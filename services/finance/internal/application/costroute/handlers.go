// Package costroute holds use cases for the persisted routing DAG
// (cost_route_head + cost_route_seq + cost_route_rm).
package costroute

import (
	"context"
	"fmt"

	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

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

// LockHandler transitions COMPLETE -> LOCKED.
type LockHandler struct {
	repo costroute.Repository
}

// NewLockHandler constructs a LockHandler.
func NewLockHandler(repo costroute.Repository) *LockHandler {
	return &LockHandler{repo: repo}
}

// Handle locks the head.
func (h *LockHandler) Handle(ctx context.Context, headID int64, actor string) (*costroute.Head, error) {
	head, err := h.repo.GetHead(ctx, headID)
	if err != nil {
		return nil, err
	}
	if err := head.Lock(); err != nil {
		return nil, err
	}
	if err := h.repo.SaveHead(ctx, head, actor); err != nil {
		return nil, err
	}
	return head, nil
}

// UnlockHandler transitions LOCKED -> COMPLETE.
type UnlockHandler struct {
	repo costroute.Repository
}

// NewUnlockHandler constructs an UnlockHandler.
func NewUnlockHandler(repo costroute.Repository) *UnlockHandler {
	return &UnlockHandler{repo: repo}
}

// Handle unlocks the head.
func (h *UnlockHandler) Handle(ctx context.Context, headID int64, actor string) (*costroute.Head, error) {
	head, err := h.repo.GetHead(ctx, headID)
	if err != nil {
		return nil, err
	}
	if err := head.Unlock(); err != nil {
		return nil, err
	}
	if err := h.repo.SaveHead(ctx, head, actor); err != nil {
		return nil, err
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
