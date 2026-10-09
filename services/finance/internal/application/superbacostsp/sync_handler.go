package superbacostsp

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/superbacostsp"
)

// SyncResult summarizes one sync run.
type SyncResult struct {
	TotalRows int
	Inserted  int
	Updated   int
	Unchanged int
	Skipped   int
	Duration  time.Duration
}

// SyncHandler pulls the master from the Oracle legacy source and upserts by
// legacy_sys_id. A nil source (not configured) yields ErrSyncNotConfigured.
// The Oracle implementation of domain.Source is delivered in phase P4.
type SyncHandler struct {
	source domain.Source
	repo   domain.Repository
	logger zerolog.Logger
}

// NewSyncHandler creates a SyncHandler; source may be nil.
func NewSyncHandler(source domain.Source, repo domain.Repository, logger zerolog.Logger) *SyncHandler {
	return &SyncHandler{source: source, repo: repo, logger: logger}
}

// Execute runs the sync.
func (h *SyncHandler) Execute(ctx context.Context) (*SyncResult, error) {
	if h.source == nil {
		return nil, domain.ErrSyncNotConfigured
	}
	start := time.Now()
	items, err := h.source.ListSuperbaCostSP(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch superba cost sp from oracle: %w", err)
	}
	res := &SyncResult{TotalRows: len(items)}
	for _, item := range items {
		outcome, upErr := h.repo.UpsertByLegacySysID(ctx, item)
		if upErr != nil {
			return nil, fmt.Errorf("upsert superba cost sp %d: %w", item.LegacySysID, upErr)
		}
		switch outcome {
		case domain.OutcomeInserted:
			res.Inserted++
		case domain.OutcomeUpdated:
			res.Updated++
		case domain.OutcomeUnchanged:
			res.Unchanged++
		case domain.OutcomeSkipped:
			res.Skipped++
		}
	}
	res.Duration = time.Since(start)
	h.logger.Info().Int("total", res.TotalRows).Int("inserted", res.Inserted).
		Int("updated", res.Updated).Int("unchanged", res.Unchanged).Int("skipped", res.Skipped).
		Msg("Superba cost sp sync completed")
	return res, nil
}
