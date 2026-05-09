// Package prdrequest holds application-layer command handlers for the ProductRequest aggregate.
package prdrequest

import (
	"context"

	"github.com/google/uuid"

	domainprdrequest "github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
)

// GetCommand carries inputs to GetHandler.
type GetCommand struct {
	ID uuid.UUID
}

// GetHandler retrieves a single product request by its UUID.
type GetHandler struct {
	repo domainprdrequest.Repository
}

// NewGetHandler constructs a GetHandler.
func NewGetHandler(repo domainprdrequest.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle fetches a Request by ID, returning ErrNotFound if absent.
func (h *GetHandler) Handle(ctx context.Context, cmd GetCommand) (*domainprdrequest.Request, error) {
	return h.repo.GetByID(ctx, cmd.ID)
}
