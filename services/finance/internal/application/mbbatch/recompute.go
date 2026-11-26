package mbbatch

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/rs/zerolog/log"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// triggeredByMBRecompute labels the cal_job rows of the after-re-validate recompute, so
// they are told apart from a manual MB_BATCH (cj_triggered_by is VARCHAR(20)).
const triggeredByMBRecompute = "system:mb_recompute"

// recomputeTimeout bounds one detached recompute (all periods of one trigger).
const recomputeTimeout = 15 * time.Minute

// SnapshotPeriodReader lists, per period, which of the given products already hold a
// non-SUPERSEDED cst_product_cost row. *postgres.CostResultRepository satisfies it.
type SnapshotPeriodReader interface {
	ListActiveSnapshotPeriods(ctx context.Context, productSysIDs []int64) (map[string][]int64, error)
}

// WithSnapshotPeriods installs the period reader the re-validate recompute needs.
// Without it RecomputeRevalidated is a no-op; Handle never uses it.
func WithSnapshotPeriods(r SnapshotPeriodReader) TriggerOption {
	return func(h *TriggerHandler) { h.periods = r }
}

// TriggerRevalidateRecompute runs RecomputeRevalidated detached from the caller: after
// commit, on context.WithoutCancel(ctx) with recomputeTimeout, so the approve response
// never waits for it and can never be failed by it. Results land in cal_job + logs.
func (h *TriggerHandler) TriggerRevalidateRecompute(ctx context.Context, mbhIDs []string, actor string) {
	if h == nil || h.periods == nil || len(mbhIDs) == 0 {
		return
	}
	bg := context.WithoutCancel(ctx)
	ids := slices.Clone(mbhIDs)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Strs("mbh_ids", ids).Msg("mb recompute after re-validate panicked")
			}
		}()
		h.recomputeMu.Lock()
		defer h.recomputeMu.Unlock()
		ctx, cancel := context.WithTimeout(bg, recomputeTimeout)
		defer cancel()
		h.RecomputeRevalidated(ctx, ids, actor)
	}()
}

// RecomputeRevalidated recomputes the cost snapshots of re-validated MB heads and of every
// VALIDATED MB that transitively consumes them (source_type MB in its current composition
// version), for each period where a re-validated head already has an active snapshot and
// the ACTUAL period is not locked. One cal_job (scope MB_BATCH) per period; per-MB errors
// are recorded on it exactly like a manual batch. Never returns an error: everything is
// logged, because the transition that triggered it has already committed.
func (h *TriggerHandler) RecomputeRevalidated(ctx context.Context, mbhIDs []string, actor string) {
	if h.periods == nil || len(mbhIDs) == 0 {
		return
	}
	logger := log.With().Strs("mbh_ids", mbhIDs).Logger()
	sorted, edges, err := buildDAG(ctx, h.svc.headReader, h.svc.edgeReader)
	if err != nil {
		logger.Error().Err(err).Msg("mb recompute after re-validate: build dag failed, snapshots not refreshed")
		return
	}
	roots := rootProducts(sorted, mbhIDs)
	if len(roots) == 0 {
		return
	}
	active, err := h.periods.ListActiveSnapshotPeriods(ctx, roots)
	if err != nil {
		logger.Error().Err(err).Msg("mb recompute after re-validate: list snapshot periods failed, snapshots not refreshed")
		return
	}
	plan := planRecompute(sorted, edges, active)
	filter, err := json.Marshal(map[string]any{"revalidated_mbh_ids": mbhIDs})
	if err != nil {
		filter = nil // traceability only; the recompute itself does not need it
	}
	for _, period := range sortedKeys(plan) {
		run := plan[period]
		if err := costcalcdom.CheckPeriodUnlocked(ctx, h.periodLock, period, costcalcdom.CalcTypeActual); err != nil {
			if errors.Is(err, costcalcdom.ErrPeriodLocked) {
				logger.Info().Str("period", period).Msg("mb recompute after re-validate: period locked, skipped")
			} else {
				logger.Error().Err(err).Str("period", period).Msg("mb recompute after re-validate: period lock check failed, skipped")
			}
			continue
		}
		res, err := h.track(ctx, period, actor, triggeredByMBRecompute, filter, func(jobID int64) (*BatchResult, error) {
			return h.svc.runCandidates(ctx, period, jobID, sorted, run)
		})
		if err != nil {
			logger.Error().Err(err).Str("period", period).Msg("mb recompute after re-validate: period failed")
			continue
		}
		logger.Info().Str("period", period).Int64("job_id", res.JobID).
			Int32("success", res.SuccessCount).Int32("failed", res.FailedCount).
			Msg("mb recompute after re-validate: period done")
	}
}

// rootProducts returns the cost product ids of the requested heads that are still
// VALIDATED (in sorted); a head that left VALIDATED since is ignored.
func rootProducts(sorted []MBHeadCandidate, mbhIDs []string) []int64 {
	want := make(map[string]struct{}, len(mbhIDs))
	for _, id := range mbhIDs {
		want[id] = struct{}{}
	}
	var out []int64
	for _, c := range sorted {
		if _, ok := want[c.MBHID]; ok && c.CostProductID != 0 {
			out = append(out, c.CostProductID)
		}
	}
	return out
}

// planRecompute maps each period to the MBs to recompute there: the heads whose product
// has an active snapshot in that period (active) plus all their transitive dependents,
// in sorted's topological order (children before parents). Pure, so it is unit-tested.
func planRecompute(sorted []MBHeadCandidate, edges []MBEdge, active map[string][]int64) map[string][]MBHeadCandidate {
	byProduct := make(map[int64]string, len(sorted))
	for _, c := range sorted {
		byProduct[c.CostProductID] = c.MBHID
	}
	dependents := make(map[string][]string, len(edges))
	for _, e := range edges {
		if e.MBHID != e.RefMBHID {
			dependents[e.RefMBHID] = append(dependents[e.RefMBHID], e.MBHID)
		}
	}
	plan := make(map[string][]MBHeadCandidate, len(active))
	for period, products := range active {
		closure := dependentClosure(products, byProduct, dependents)
		var run []MBHeadCandidate
		for _, c := range sorted {
			if _, ok := closure[c.MBHID]; ok {
				run = append(run, c)
			}
		}
		if len(run) > 0 {
			plan[period] = run
		}
	}
	return plan
}

// dependentClosure returns the heads of products plus every head that transitively
// consumes one of them.
func dependentClosure(products []int64, byProduct map[int64]string, dependents map[string][]string) map[string]struct{} {
	closure := map[string]struct{}{}
	var queue []string
	for _, pid := range products {
		if id, ok := byProduct[pid]; ok {
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, seen := closure[id]; seen {
			continue
		}
		closure[id] = struct{}{}
		queue = append(queue, dependents[id]...)
	}
	return closure
}

func sortedKeys(m map[string][]MBHeadCandidate) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
