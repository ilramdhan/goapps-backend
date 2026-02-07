// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// GetDetailQuery represents the get user with detail query.
type GetDetailQuery struct {
	UserID string
}

// GetDetailResult holds the user and their detail.
type GetDetailResult struct {
	User   *user.User
	Detail *user.Detail
}

// GetDetailHandler handles the get user detail query.
type GetDetailHandler struct {
	repo user.Repository
}

// NewGetDetailHandler creates a new GetDetailHandler.
func NewGetDetailHandler(repo user.Repository) *GetDetailHandler {
	return &GetDetailHandler{repo: repo}
}

// Handle executes the get user detail query.
func (h *GetDetailHandler) Handle(ctx context.Context, query GetDetailQuery) (*GetDetailResult, error) {
	id, err := uuid.Parse(query.UserID)
	if err != nil {
		return nil, shared.ErrNotFound
	}

	// 1. Get user.
	u, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 2. Get user detail.
	detail, err := h.repo.GetDetailByUserID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &GetDetailResult{
		User:   u,
		Detail: detail,
	}, nil
}
