// Package role provides application layer handlers for Role operations.
package role

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// GetQuery represents the get role query.
type GetQuery struct {
	RoleID string
}

// GetHandler handles the GetRole query.
type GetHandler struct {
	repo role.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo role.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get role query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*role.Role, error) {
	id, err := uuid.Parse(query.RoleID)
	if err != nil {
		return nil, shared.ErrNotFound
	}

	return h.repo.GetByID(ctx, id)
}
