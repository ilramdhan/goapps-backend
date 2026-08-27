// Package shade provides application layer handlers for the shade master (R8).
package shade

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// SyncResult summarizes one Oracle-to-Postgres shade sync run.
type SyncResult struct {
	TotalRows int
	Inserted  int
	Updated   int
	Skipped   int
	Duration  time.Duration
}

// SyncHandler orchestrates the Oracle -> Postgres shade sync (R8), mirroring the
// fetch-then-upsert shape of oraclesync.SyncHandler.
//
// This handler deliberately does NOT integrate with the job/execution
// persistence system (internal/domain/job) that oraclesync uses: wiring a new
// job type there touches shared job-domain code (enum, exhaustive switches)
// that is out of this task's stated scope (finance shade layer only), and there
// is no RPC/cron entrypoint yet to trigger this handler anyway — that requires
// proto that does not exist. A caller (cron, CLI, or a future RPC) can call
// Execute directly and log/persist the SyncResult itself.
type SyncHandler struct {
	oracleRepo shade.Source
	pgRepo     shade.Repository
	logger     zerolog.Logger
}

// NewSyncHandler creates a new SyncHandler. oracleRepo may be nil if Oracle is
// unconfigured; Execute then returns shade.ErrSyncNotConfigured.
func NewSyncHandler(oracleRepo shade.Source, pgRepo shade.Repository, logger zerolog.Logger) *SyncHandler {
	return &SyncHandler{oracleRepo: oracleRepo, pgRepo: pgRepo, logger: logger}
}

// Execute fetches the full shade master from Oracle and upserts it into
// cost_erp_shade. Rows whose provenance is MANUAL are left untouched by
// ShadeRepository.UpsertSourced. Shades present in Postgres but no longer
// present in Oracle are left as-is (no auto soft-delete) — a settled decision,
// not an oversight: see the R8 report for the rationale.
func (h *SyncHandler) Execute(ctx context.Context) (*SyncResult, error) {
	if h.oracleRepo == nil {
		return nil, shade.ErrSyncNotConfigured
	}

	start := time.Now()
	items, err := h.oracleRepo.ListShades(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch shade master from oracle: %w", err)
	}
	h.logger.Info().Int("rows", len(items)).Msg("Fetched shade master from Oracle")

	result := &SyncResult{TotalRows: len(items)}
	for _, item := range items {
		outcome, upsertErr := h.pgRepo.UpsertSourced(ctx, item)
		if upsertErr != nil {
			return nil, fmt.Errorf("upsert shade %q: %w", item.Code, upsertErr)
		}
		switch outcome {
		case shade.OutcomeInserted:
			result.Inserted++
		case shade.OutcomeUpdated:
			result.Updated++
		case shade.OutcomeSkipped:
			result.Skipped++
		}
	}
	result.Duration = time.Since(start)

	h.logger.Info().
		Int("total", result.TotalRows).
		Int("inserted", result.Inserted).
		Int("updated", result.Updated).
		Int("skipped", result.Skipped).
		Dur("duration", result.Duration).
		Msg("Shade master sync completed")

	return result, nil
}
