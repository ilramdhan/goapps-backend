// Package prdrequest holds application-layer command handlers for the ProductRequest aggregate.
package prdrequest

import (
	"context"

	"github.com/google/uuid"

	domainprdrequest "github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
)

// ResolveCommand carries inputs to ResolveHandler.
type ResolveCommand struct {
	ID             uuid.UUID
	ProductID      uuid.UUID
	ResolutionNote string
	UpdatedBy      string
}

// ResolveHandler closes a product request as RESOLVED by linking an existing product.
type ResolveHandler struct {
	repo domainprdrequest.Repository
}

// NewResolveHandler constructs a ResolveHandler.
func NewResolveHandler(repo domainprdrequest.Repository) *ResolveHandler {
	return &ResolveHandler{repo: repo}
}

// Handle fetches the request, resolves it with the given product, and persists the result.
// Returns ErrNotFound if absent, ErrAlreadyResolved, ErrAlreadyRejected, ErrCannotResolve,
// or ErrInvalidProductLink if the productID is zero.
func (h *ResolveHandler) Handle(ctx context.Context, cmd ResolveCommand) (*domainprdrequest.Request, error) {
	existing, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := existing.Resolve(cmd.ProductID, cmd.ResolutionNote, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	return existing, nil
}
