package upload

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	uploaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/upload"
)

// CommitHandler commits a validated upload session into bi_fact_metric.
type CommitHandler struct {
	repo uploaddomain.Repository
}

// NewCommitHandler constructs a CommitHandler.
func NewCommitHandler(repo uploaddomain.Repository) *CommitHandler {
	return &CommitHandler{repo: repo}
}

// Handle commits the session and refreshes materialized views.
func (h *CommitHandler) Handle(ctx context.Context, uploadID uuid.UUID) (*uploaddomain.Upload, error) {
	session, err := h.repo.GetSession(ctx, uploadID)
	if err != nil {
		return nil, err
	}
	if session.Status() != uploaddomain.StatusValidated {
		return nil, uploaddomain.ErrNotCommittable
	}

	session.MarkCommitting()
	if err := h.repo.UpdateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("set committing status: %w", err)
	}

	committed, commitErr := h.repo.CommitToFact(ctx, uploadID)
	if commitErr != nil {
		h.markFailed(ctx, session)
		return nil, fmt.Errorf("commit staging to fact: %w", commitErr)
	}

	if refreshErr := h.repo.RefreshViews(ctx); refreshErr != nil {
		h.markFailed(ctx, session)
		return nil, fmt.Errorf("refresh materialized views: %w", refreshErr)
	}

	session.MarkCommitted(committed)
	if err := h.repo.UpdateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("set committed status: %w", err)
	}
	return session, nil
}

func (h *CommitHandler) markFailed(ctx context.Context, session *uploaddomain.Upload) {
	session.MarkFailed()
	if err := h.repo.UpdateSession(ctx, session); err != nil {
		log.Warn().Err(err).Str("upload_id", session.ID().String()).Msg("failed to mark upload session FAILED")
	}
}
