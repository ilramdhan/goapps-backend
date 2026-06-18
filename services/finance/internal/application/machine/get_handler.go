// Package machine provides application layer handlers for Machine operations.
package machine

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
)

// GetQuery holds the input for retrieving a single machine.
type GetQuery struct {
	MachineID uuid.UUID
}

// GetHandler handles the GetMachine query.
type GetHandler struct {
	repo machine.Repository
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(repo machine.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the get machine query.
func (h *GetHandler) Handle(ctx context.Context, query GetQuery) (*machine.Entity, error) {
	return h.repo.GetByID(ctx, query.MachineID)
}
