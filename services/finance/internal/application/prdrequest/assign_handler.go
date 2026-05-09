// Package prdrequest holds application-layer command handlers for the ProductRequest aggregate.
package prdrequest

import (
	"context"

	"github.com/google/uuid"

	domainprdrequest "github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
)

// AssignCommand carries inputs to AssignHandler.
type AssignCommand struct {
	ID         uuid.UUID
	AssigneeID uuid.UUID
	UpdatedBy  string
}

// AssignHandler sets an assignee on a product request, transitioning OPEN → IN_REVIEW.
type AssignHandler struct {
	repo domainprdrequest.Repository
}

// NewAssignHandler constructs an AssignHandler.
func NewAssignHandler(repo domainprdrequest.Repository) *AssignHandler {
	return &AssignHandler{repo: repo}
}

// Handle fetches the request, applies the assignment, and persists the result.
// Returns ErrNotFound if absent, ErrCannotAssign if in a terminal status,
// ErrInvalidAssignee if the assignee UUID is zero.
func (h *AssignHandler) Handle(ctx context.Context, cmd AssignCommand) (*domainprdrequest.Request, error) {
	existing, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := existing.Assign(cmd.AssigneeID, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	return existing, nil
}
