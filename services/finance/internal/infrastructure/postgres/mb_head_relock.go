package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// Verify the auto-relock contract at compile time. 🔴 Separate from the
// `var _ mbhead.Repository` assertion in mb_head_repository.go on purpose: the job
// depends on this narrow interface only, so its two methods never appear in the
// 15-method Repository that application handlers and their test doubles implement.
var _ mbhead.RelockRepository = (*MBHeadRepository)(nil)

// ListExpiredUnlocks returns every head whose granted unlock window has run out.
// See mbhead.RelockRepository for the candidate definition; the WHERE clause below
// implements it clause for clause.
func (r *MBHeadRepository) ListExpiredUnlocks(ctx context.Context) ([]mbhead.RelockCandidate, error) {
	// 🔴 The lateral subquery takes the head's LATEST lock-log row and the outer WHERE
	// then insists that row is an UNLOCK_GRANT with an elapsed deadline. Testing the
	// LATEST row (rather than "any UNLOCK_GRANT with an elapsed deadline") is what
	// makes the job idempotent: the moment ApplyRelock writes its RELOCK row, that row
	// becomes the latest and the head stops matching. It also uses idx_mbhl_mbh_at.
	//
	// COALESCE(mbh_is_locked, FALSE) — ⛔ never `= FALSE`: NULL means NOT locked and
	// 4190 production rows are NULL.
	//
	// The pre-unlock status subquery is the SAME shape selectCols uses, so the job and
	// a normal read agree on the answer. It can be NULL; the caller skips those.
	rows, err := r.db.QueryContext(ctx, listExpiredUnlocksQuery,
		mbhead.LockEventUnlockGrant, mbhead.StatusDraft)
	if err != nil {
		return nil, fmt.Errorf("mb_head_relock: list expired unlocks: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var out []mbhead.RelockCandidate
	for rows.Next() {
		var c mbhead.RelockCandidate
		if scanErr := rows.Scan(&c.ID, &c.MBCosting, &c.CurrentVersion, &c.PreUnlockStatus,
			&c.GrantedAt); scanErr != nil {
			return nil, fmt.Errorf("mb_head_relock: scan candidate: %w", scanErr)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mb_head_relock: iterate candidates: %w", err)
	}
	return out, nil
}

// listExpiredUnlocksQuery is built ONCE at init: untouchedSinceGrant is a function, so
// the statement cannot be a const. It is still a fixed string with only $N binds — ⛔ no
// caller input is ever interpolated (gosec G201).
var listExpiredUnlocksQuery = `
		SELECT h.mbh_id, h.mbh_mb_costing, h.mbh_current_version,
		       COALESCE((SELECT w.mbwl_from_state
		                   FROM mst_mb_workflow_log w
		                  WHERE w.mbwl_mbh_id = h.mbh_id
		                    AND w.mbwl_to_state = 'UNLOCK_REQUESTED'
		                  ORDER BY w.mbwl_actor_at DESC
		                  LIMIT 1), '') AS pre_unlock_status,
		       last.mbhl_actor_at AS granted_at
		  FROM mst_mb_head h
		  JOIN LATERAL (
		        SELECT l.mbhl_event, l.mbhl_actor_at, l.mbhl_auto_relock_at
		          FROM mst_mb_head_lock_log l
		         WHERE l.mbhl_mbh_id = h.mbh_id
		         ORDER BY l.mbhl_actor_at DESC
		         LIMIT 1
		  ) last ON TRUE
		 WHERE last.mbhl_event = $1
		   AND last.mbhl_auto_relock_at IS NOT NULL
		   AND last.mbhl_auto_relock_at <= NOW()
		   AND COALESCE(h.mbh_is_locked, FALSE) = FALSE
		   AND h.mbh_entry_status = $2
		   AND h.deleted_at IS NULL
		   AND ` + untouchedSinceGrant("h", "last.mbhl_actor_at") + `
		 ORDER BY h.mbh_mb_costing`

// ApplyRelock closes one expired unlock window in a SINGLE transaction: status back to
// toState, lock columns set, one workflow-log row, one RELOCK lock-log row.
func (r *MBHeadRepository) ApplyRelock(ctx context.Context, c mbhead.RelockCandidate, toState string) error {
	effect := mbhead.DeriveRelockEffect(toState)
	if effect.Event == "" {
		// ⛔ Refuse rather than write a status the lock rules do not recognize.
		return fmt.Errorf("mb_head_relock: %s: %q is not a lockable state: %w",
			c.MBCosting, toState, mbhead.ErrUnlockOriginUnknown)
	}
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := r.relockHeadTx(ctx, tx, c, toState, effect); err != nil {
			return err
		}
		// ⛔ The version is NOT bumped: no recipe content changed, the window merely
		// expired. The log row records the version the head is already on.
		if err := r.insertWorkflowLogTx(ctx, tx, c.ID, mbhead.StatusDraft, toState,
			mbhead.SystemActorID, relockReason, c.CurrentVersion); err != nil {
			return err
		}
		return insertLockLogTx(ctx, tx, c.ID, effect, mbhead.SystemActorID, relockReason)
	})
}

// relockReason is stamped on both audit rows so the trail says WHY the head moved
// without a human touching it. ⛔ It is NOT written to mbh_unlock_reason: that column
// holds the reason the unlock was ASKED for and must survive untouched (principle U-2).
const relockReason = "Auto-relock: unlock window expired"

func (r *MBHeadRepository) relockHeadTx(
	ctx context.Context, tx *sql.Tx, c mbhead.RelockCandidate, toState string, effect mbhead.LockEffect,
) error {
	calc, err := r.deriveCheckStatusCalcTx(ctx, tx, c.ID, toState)
	if err != nil {
		return err
	}
	q := `UPDATE mst_mb_head SET mbh_entry_status = $2, updated_at = NOW()`
	args := []any{c.ID, toState}
	if calc != nil {
		args = append(args, *calc)
		q += fmt.Sprintf(`, mbh_check_status_calc = $%d`, len(args))
	}
	// Lock columns come from the shared renderer, which computes its placeholders from
	// len(args) — ⛔ never hardcoded $N. The reason is passed for symmetry only: this
	// effect sets no unlock request, so lockClauses consumes it nowhere.
	lockFrag, args := lockClauses(effect, mbhead.SystemActorID, relockReason, args)
	q += lockFrag

	// 🔴 The status guard is repeated here, not just in ListExpiredUnlocks: between the
	// scan and this UPDATE a user may have moved or re-locked the head, and the relock
	// must lose that race silently rather than overwrite them.
	args = append(args, mbhead.StatusDraft)
	q += fmt.Sprintf(` WHERE mbh_id = $1 AND deleted_at IS NULL`+
		` AND mbh_entry_status = $%d AND COALESCE(mbh_is_locked, FALSE) = FALSE`, len(args))

	// 🔴 And so is the untouched-since-grant guard, for the SAME reason and a sharper
	// one: the sweep may have listed this head minutes ago, and a save landing in that
	// window is exactly the work this fix exists to protect. Re-evaluated INSIDE the
	// transaction against the grant instant carried on the candidate, so a concurrent
	// save makes the UPDATE match zero rows and the relock loses the race — ⛔ it never
	// overwrites the save. RowsAffected = 0 surfaces as the same "moved concurrently"
	// error the status guard already produces, which the job logs and moves past.
	args = append(args, c.GrantedAt)
	q += fmt.Sprintf(` AND %s`, untouchedSinceGrant("mst_mb_head", fmt.Sprintf("$%d", len(args))))

	result, execErr := tx.ExecContext(ctx, q, args...)
	if execErr != nil {
		return fmt.Errorf("mb_head_relock: update head %s: %w", c.MBCosting, execErr)
	}
	affected, affErr := result.RowsAffected()
	if affErr != nil {
		return fmt.Errorf("mb_head_relock: rows affected: %w", affErr)
	}
	if affected == 0 {
		return fmt.Errorf("mb_head_relock: %s moved or was re-locked concurrently: %w",
			c.MBCosting, mbhead.ErrNotFound)
	}
	return nil
}

// untouchedSinceGrant renders the "nothing was saved since the unlock was granted"
// predicate — the guard that stops the auto-relock job from STEALING work in progress.
//
// 🔴 WHY IT EXISTS. The status test (mbh_entry_status = 'DRAFT') cannot answer the
// question on its own. GrantUnlock parks a reopened head in DRAFT, and every save the
// user then makes deliberately LEAVES it in DRAFT (item D7 — an edited recipe must go
// round submit → approve again). So DRAFT means BOTH "reopened, never touched" and
// "reopened and actively being worked on". Without this predicate the cron would throw
// a half-finished edit back to APPROVED/VALIDATED with no human assent. That is lost
// work, so the rule is: ANY write after the grant ⇒ ⛔ NOT a candidate, ever.
//
// 🔴 WHICH TABLES ARE WATCHED, and why that is the whole editable recipe:
//
//	mst_mb_head        — updated_at, written by Update (mb_head_repository.go:169) and
//	                     by every Transition (mb_head_transition.go:63). COALESCE'd to
//	                     created_at because updated_at is NULLable (000388:16) and a
//	                     never-updated head must still qualify.
//	mst_mb_composition — the recipe LINES, which live in their own table and ⛔ do NOT
//	                     touch the head's updated_at (mb_composition_repository.go:112,
//	                     135, 156 write only mbcm_* columns). Watching the head alone
//	                     would therefore miss the single most likely edit of all — the
//	                     user reopened the recipe precisely to change its composition.
//	mst_mb_head_shade  — additional shade rows, replaced wholesale by ReplaceShades
//	                     (mb_head_shade_repository.go:65) in its OWN transaction; the
//	                     head's updated_at does move on that path too, but only because
//	                     the caller also ran Update, which is not guaranteed.
//
// Both child tables are checked over ALL rows including soft-deleted ones — a DELETE is
// an edit, and mst_mb_composition's delete stamps only deleted_at (`:156`). GREATEST over
// created/updated/deleted covers insert, update and delete with one expression.
//
// ⚠ HONEST LIMIT — GERBANG KEPUTUSAN USER. mst_mb_spin rows (mbs_mbh_id → head) are
// deliberately ⛔ NOT watched. Spin writes are not gated by the head lock at all — they
// are editable whatever the head's status — so a spin edit is not evidence that the
// UNLOCK window is being used, and treating it as such would keep windows open forever.
// If the user wants a spin save to also block the relock, this is the one place to add it.
//
// ⚠ Both sides of every comparison are DATABASE-clock TIMESTAMPTZ values: mbhl_actor_at
// defaults to NOW() (000485), updated_at is written as NOW() by the transition paths, and
// the child timestamps are TIMESTAMPTZ too (000439:14-18, 000483:28-31). ⛔ Never compare
// against a Go-side time.
//
// headAlias is how mst_mb_head is named in the surrounding statement; grantExpr is the
// SQL expression yielding the UNLOCK_GRANT instant (a column reference in the sweep
// query, a bind placeholder in the re-check inside ApplyRelock's transaction).
func untouchedSinceGrant(headAlias, grantExpr string) string {
	return `(
		   COALESCE(` + headAlias + `.updated_at, ` + headAlias + `.created_at) <= ` + grantExpr + `
	   AND NOT EXISTS (
		         SELECT 1 FROM mst_mb_composition cmp
		          WHERE cmp.mbcm_mbh_id = ` + headAlias + `.mbh_id
		            AND GREATEST(cmp.mbcm_created_at,
		                         COALESCE(cmp.mbcm_updated_at, cmp.mbcm_created_at),
		                         COALESCE(cmp.deleted_at, cmp.mbcm_created_at)) > ` + grantExpr + `)
	   AND NOT EXISTS (
		         SELECT 1 FROM mst_mb_head_shade shd
		          WHERE shd.mbhs_mbh_id = ` + headAlias + `.mbh_id
		            AND GREATEST(shd.mbhs_created_at,
		                         COALESCE(shd.mbhs_updated_at, shd.mbhs_created_at),
		                         COALESCE(shd.deleted_at, shd.mbhs_created_at)) > ` + grantExpr + `))`
}
