// Package parameter provides application layer handlers for Parameter operations.
package parameter

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// GetQuery represents the get Parameter query.
type GetQuery struct {
	ParamID string
}

// GetHandler handles the GetParameter query.
type GetHandler struct {
	repo parameter.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo parameter.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get Parameter query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*parameter.Parameter, error) {
	id, err := uuid.Parse(query.ParamID)
	if err != nil {
		return nil, parameter.ErrNotFound
	}

	return h.repo.GetByID(ctx, id)
}
