// Package mbrelock holds the cron job that closes expired MB-recipe unlock windows.
package mbrelock

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// jobTimeout bounds one whole sweep. A sweep is a small SELECT plus one short
// transaction per candidate, so minutes is generous; the bound exists so a stuck
// database cannot leave a cron goroutine running until the next tick.
const jobTimeout = 2 * time.Minute

// Job re-locks MB recipe heads whose granted unlock window has expired.
//
// Per user decision K-57 option (a) an expired window returns the head to the state it
// was unlocked FROM (APPROVED or VALIDATED) and locks it again — as if the unlock had
// never been granted. ⛔ It is NOT left in DRAFT and ⛔ not left open.
//
// The deadline itself is stamped at grant time into
// mst_mb_head_lock_log.mbhl_auto_relock_at as NOW() + mbhead.AutoRelockAfter (24h,
// decision K-51 option (a)). This job is the code path that finally acts on it, and it
// is the only producer of mbhl_event = 'RELOCK'.
//
// Intended to be driven by a cron scheduler (hourly). Because the candidate query
// tests the LATEST lock-log row, a head that has already been relocked stops matching
// at once — running the job more often than necessary is harmless.
type Job struct {
	repo mbhead.RelockRepository
}

// NewJob constructs the auto-relock job.
func NewJob(repo mbhead.RelockRepository) *Job {
	return &Job{repo: repo}
}

// Run is the cron entry point. It takes no arguments and builds its own context, so it
// can be handed straight to cron.AddFunc.
//
// 🔴 It ⛔ NEVER panics: a panic in a cron goroutine takes the whole service down. Every
// failure is logged and the sweep carries on — one bad candidate must not shield the
// rest from being re-locked.
func (j *Job) Run() {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

	candidates, err := j.repo.ListExpiredUnlocks(ctx)
	if err != nil {
		log.Error().Err(err).Msg("mb_relock_job: list expired unlocks failed")
		return
	}
	if len(candidates) == 0 {
		return
	}

	relocked, skipped, failed := 0, 0, 0
	for _, c := range candidates {
		switch outcome := j.relockOne(ctx, c); outcome {
		case outcomeRelocked:
			relocked++
		case outcomeSkipped:
			skipped++
		case outcomeFailed:
			failed++
		}
	}
	log.Info().
		Int("candidates", len(candidates)).
		Int("relocked", relocked).Int("skipped", skipped).Int("failed", failed).
		Msg("mb_relock_job: sweep finished")
}

type outcome int

const (
	outcomeRelocked outcome = iota
	outcomeSkipped
	outcomeFailed
)

// relockOne closes one window. It returns rather than propagating, so the caller's loop
// never stops early.
func (j *Job) relockOne(ctx context.Context, c mbhead.RelockCandidate) outcome {
	// ⛔ NEVER guess between APPROVED and VALIDATED. Without a pre-unlock status the
	// state to return to is unknown, and returning a VALIDATED recipe as merely
	// APPROVED (or the reverse) would silently rewrite its costing standing. The head
	// stays open and a human decides — consistent with K-52 option (a) and with
	// Entity.RejectUnlock's ErrUnlockOriginUnknown.
	if c.PreUnlockStatus == "" {
		log.Warn().
			Str("mbh_id", c.ID.String()).Str("mb_costing", c.MBCosting).
			Msg("mb_relock_job: skipped — pre-unlock status unknown, will not guess APPROVED vs VALIDATED")
		return outcomeSkipped
	}
	if err := j.repo.ApplyRelock(ctx, c, c.PreUnlockStatus); err != nil {
		log.Error().Err(err).
			Str("mbh_id", c.ID.String()).Str("mb_costing", c.MBCosting).
			Str("to_state", c.PreUnlockStatus).
			Msg("mb_relock_job: apply relock failed")
		return outcomeFailed
	}
	log.Info().
		Str("mbh_id", c.ID.String()).Str("mb_costing", c.MBCosting).
		Str("to_state", c.PreUnlockStatus).
		Msg("mb_relock_job: unlock window expired, head re-locked")
	return outcomeRelocked
}
