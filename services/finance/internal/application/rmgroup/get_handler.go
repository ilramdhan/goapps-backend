// Package rmgroup provides application layer handlers for RM group head and detail operations.
package rmgroup

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// GetQuery retrieves a head (and optionally its details) by ID.
type GetQuery struct {
	HeadID      string
	WithDetails bool
	ActiveOnly  bool
}

// GetResult bundles the head with optional detail rows.
type GetResult struct {
	Head    *rmgroup.Head
	Details []*rmgroup.Detail
}

// GetHandler handles GetHead queries.
type GetHandler struct {
	repo rmgroup.Repository
}

// NewGetHandler builds a GetHandler.
func NewGetHandler(repo rmgroup.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle returns the head and, when requested, its detail rows. Soft-deleted rows
// are omitted from the detail list regardless of ActiveOnly; ActiveOnly further
// restricts the result to rows where is_active = true.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*GetResult, error) {
	id, err := uuid.Parse(query.HeadID)
	if err != nil {
		return nil, rmgroup.ErrNotFound
	}

	head, err := h.repo.GetHeadByID(ctx, id)
	if err != nil {
		return nil, err
	}

	result := &GetResult{Head: head}
	if !query.WithDetails {
		return result, nil
	}

	details, err := h.loadDetails(ctx, id, query.ActiveOnly)
	if err != nil {
		return nil, fmt.Errorf("load details: %w", err)
	}
	result.Details = details
	return result, nil
}

func (h *GetHandler) loadDetails(ctx context.Context, headID uuid.UUID, activeOnly bool) ([]*rmgroup.Detail, error) {
	if activeOnly {
		return h.repo.ListActiveDetailsByHeadID(ctx, headID)
	}
	return h.repo.ListDetailsByHeadID(ctx, headID)
}
