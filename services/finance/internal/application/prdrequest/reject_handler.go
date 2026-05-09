// Package prdrequest holds application-layer command handlers for the ProductRequest aggregate.
package prdrequest

import (
	"context"

	"github.com/google/uuid"

	domainprdrequest "github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
)

// RejectCommand carries inputs to RejectHandler.
type RejectCommand struct {
	ID        uuid.UUID
	Reason    string
	UpdatedBy string
}

// RejectHandler closes a product request as REJECTED with a mandatory reason.
type RejectHandler struct {
	repo domainprdrequest.Repository
}

// NewRejectHandler constructs a RejectHandler.
func NewRejectHandler(repo domainprdrequest.Repository) *RejectHandler {
	return &RejectHandler{repo: repo}
}

// Handle fetches the request, rejects it with the given reason, and persists the result.
// Returns ErrNotFound if absent, ErrAlreadyRejected, ErrAlreadyResolved, ErrCannotReject,
// or ErrInvalidRejectReason if the reason is too short.
func (h *RejectHandler) Handle(ctx context.Context, cmd RejectCommand) (*domainprdrequest.Request, error) {
	existing, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := existing.Reject(cmd.Reason, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	return existing, nil
}
