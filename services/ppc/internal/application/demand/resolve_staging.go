package demand

import (
	"context"
	"errors"

	"github.com/rs/zerolog/log"

	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
)

// resolveBatchSize bounds one resolve round-trip. It mirrors the finance-side
// cap of 500 pairs per request.
const resolveBatchSize = 500

// ResolveStagingResult summarizes one resolution pass over the staging inbox.
type ResolveStagingResult struct {
	// Pairs is how many distinct (item, shade) keys were sent to finance.
	Pairs int
	// Auto, Ambiguous and NotFound count the resolved pairs per outcome.
	Auto      int
	Ambiguous int
	NotFound  int
	// RowsUpdated is how many staging rows received a resolution.
	RowsUpdated int64
	// Skipped is true when finance was degraded and nothing was attempted.
	Skipped bool
}

// ResolveStaging resolves every not-yet-pulled UNRESOLVED staging row against
// the finance cost product master and records the outcome on the row.
//
// ppc_db and finance_db are separate databases, so this is a batched gRPC call
// rather than a join. Rows a planner already resolved by hand (MANUAL) are
// never overwritten. A degraded finance connection is reported as a skip, not
// an error — the caller keeps working with unresolved rows.
func (s *Service) ResolveStaging(ctx context.Context) (ResolveStagingResult, error) {
	result := ResolveStagingResult{}
	if s.resolver == nil {
		log.Debug().Msg("staging product resolution: no resolver wired, skipping")
		result.Skipped = true
		return result, nil
	}

	pairs, err := s.repo.ListUnresolvedStagingPairs(ctx)
	if err != nil {
		return result, err
	}
	if len(pairs) == 0 {
		return result, nil
	}
	result.Pairs = len(pairs)

	for start := 0; start < len(pairs); start += resolveBatchSize {
		end := start + resolveBatchSize
		if end > len(pairs) {
			end = len(pairs)
		}
		batch, batchErr := s.resolveBatch(ctx, pairs[start:end], &result)
		if batchErr != nil {
			return result, batchErr
		}
		if result.Skipped {
			return ResolveStagingResult{Skipped: true}, nil
		}
		result.RowsUpdated += batch
	}
	return result, nil
}

// resolveBatch resolves one chunk of pairs and persists the outcomes, tallying
// the per-status counters onto result. A degraded resolver marks result skipped
// and returns without an error.
func (s *Service) resolveBatch(ctx context.Context, pairs []demanddomain.StagingPair, result *ResolveStagingResult) (int64, error) {
	resolutions, err := s.resolver.ResolveByErpCode(ctx, pairs)
	if err != nil {
		if errors.Is(err, demanddomain.ErrResolverDegraded) {
			log.Warn().Int("pairs", len(pairs)).Msg("staging product resolution: finance degraded, leaving rows unresolved")
			result.Skipped = true
			return 0, nil
		}
		return 0, err
	}
	for _, r := range resolutions {
		countResolution(result, r.MatchStatus())
	}
	updated, err := s.repo.ApplyStagingResolutions(ctx, resolutions)
	if err != nil {
		return 0, err
	}
	return updated, nil
}

// countResolution tallies one resolution outcome onto the running result.
func countResolution(result *ResolveStagingResult, status string) {
	switch status {
	case demanddomain.MatchStatusAuto:
		result.Auto++
	case demanddomain.MatchStatusAmbiguous:
		result.Ambiguous++
	default:
		result.NotFound++
	}
}

// resolveStagingIfNeeded runs a resolution pass when the given page contains
// unresolved rows, then reports whether the caller should re-read. It is the
// lazy read-path trigger: failures are logged and swallowed so listing the
// inbox never fails because finance is slow or down.
func (s *Service) resolveStagingIfNeeded(ctx context.Context, items []*demanddomain.SalesOrderStaging) bool {
	if s.resolver == nil || !hasUnresolved(items) {
		return false
	}
	res, err := s.ResolveStaging(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("staging product resolution: lazy pass failed, listing unresolved rows as-is")
		return false
	}
	if res.Skipped || res.RowsUpdated == 0 {
		return false
	}
	log.Info().
		Int("pairs", res.Pairs).
		Int("auto", res.Auto).
		Int("ambiguous", res.Ambiguous).
		Int("not_found", res.NotFound).
		Int64("rows_updated", res.RowsUpdated).
		Msg("staging product resolution completed")
	return true
}

// hasUnresolved reports whether any row still awaits product resolution.
func hasUnresolved(items []*demanddomain.SalesOrderStaging) bool {
	for _, item := range items {
		if item.MatchStatus == demanddomain.MatchStatusUnresolved || item.MatchStatus == "" {
			return true
		}
	}
	return false
}
