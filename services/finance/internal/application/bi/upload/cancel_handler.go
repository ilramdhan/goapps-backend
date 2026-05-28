package upload

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	uploaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/upload"
)

// CancelHandler cancels a not-yet-committed upload session.
type CancelHandler struct {
	repo uploaddomain.Repository
}

// NewCancelHandler constructs a CancelHandler.
func NewCancelHandler(repo uploaddomain.Repository) *CancelHandler {
	return &CancelHandler{repo: repo}
}

// Handle cancels the session unless it has already been committed.
func (h *CancelHandler) Handle(ctx context.Context, uploadID uuid.UUID) (*uploaddomain.Upload, error) {
	session, err := h.repo.GetSession(ctx, uploadID)
	if err != nil {
		return nil, err
	}
	if session.Status() == uploaddomain.StatusCommitted {
		return nil, uploaddomain.ErrNotCancellable
	}
	session.MarkCancelled()
	if err := h.repo.UpdateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("cancel upload session: %w", err)
	}
	return session, nil
}
