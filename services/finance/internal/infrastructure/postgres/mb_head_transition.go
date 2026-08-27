// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// Transition atomically persists a workflow-state change: updates mst_mb_head's
// entry_status/current_version/state_reason (and, when params is non-nil, the frozen
// mbh_param_* snapshot columns), inserts a mst_mb_workflow_log audit row, and — when
// snapshotOnTransition says so — snapshots the current composition into
// mst_mb_composition_version. All writes commit or roll back together.
func (r *MBHeadRepository) Transition(ctx context.Context, id uuid.UUID, fromState, toState string, currentVersion int32, stateReason, actorUserID string, params *mbhead.ParamSnapshot) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := r.updateEntryStatusTx(ctx, tx, id, fromState, toState, currentVersion, stateReason, actorUserID, params); err != nil {
			return err
		}
		if err := r.insertWorkflowLogTx(ctx, tx, id, fromState, toState, actorUserID, stateReason, currentVersion); err != nil {
			return err
		}
		if err := r.writeLockLogTx(ctx, tx, id, fromState, toState, actorUserID, stateReason); err != nil {
			return err
		}
		if snapshotOnTransition(fromState, toState) {
			if err := r.compositionRepo.SnapshotVersion(ctx, tx, id.String(), currentVersion, actorUserID); err != nil {
				return err
			}
		}
		return nil
	})
}

// snapshotOnTransition decides whether this transition must write a composition
// snapshot into mst_mb_composition_version.
//
// 🔴 K-55 BUG FIX. The rule used to be "toState == VALIDATED", full stop. That is wrong
// for exactly one transition: UNLOCK_REQUESTED → VALIDATED, i.e. RejectUnlock putting a
// VALIDATED-origin head back where it came from. RejectUnlock deliberately does ⛔ NOT
// bump the version (nothing was edited — the unlock was refused), so the version being
// re-snapshotted is the very one the ORIGINAL Validate already snapshotted. That trips
// uq_mbcv_seq (000440:16) and rolls back the WHOLE transaction, so refusing an unlock on
// a VALIDATED-origin head fails outright. APPROVED-origin refusals never hit it because
// APPROVED is not a snapshotting state.
//
// 🔴 WHY option (a) — narrow the condition — and ⛔ NOT option (b), ON CONFLICT DO
// NOTHING on SnapshotVersion. (b) would cure every re-entry path at once, including ones
// nobody has found yet — but that is precisely its cost: a duplicate snapshot is a
// SYMPTOM, and the only two ways to produce one are "a version was re-validated without
// being bumped" (a bug) or this one transition (a known, understood, non-bug). Silencing
// the constraint would turn any FUTURE version-bump bug into a snapshot that silently
// does not happen, leaving mst_mb_composition_version quietly stale against the recipe it
// claims to freeze — unnoticeable until someone costs from it. The unique constraint
// stays loud; this one legitimate re-entry is named and excluded by name.
//
// CONSEQUENCE, stated plainly: this closes ONE path. Any other future transition that
// re-enters VALIDATED without bumping the version will still fail on uq_mbcv_seq. That is
// intended — it will fail loudly, at the transition, rather than corrupt the snapshot
// trail. Add the case here deliberately if such a path is ever introduced.
//
// Normal validation (DRAFT → VALIDATED, and any other origin) still snapshots exactly as
// before: ⛔ this must never disable a legitimate snapshot.
func snapshotOnTransition(fromState, toState string) bool {
	if toState != mbhead.StatusValidated {
		return false
	}
	return fromState != mbhead.StatusUnlockRequested
}

// writeLockLogTx appends the mst_mb_head_lock_log row for a lock-related transition,
// or does nothing when the transition touches nothing lock-related.
//
// 🔴 Same transaction as the status update and the workflow log — one commit for the
// whole transition, ⛔ never a second commit for the lock side of it.
func (r *MBHeadRepository) writeLockLogTx(
	ctx context.Context, tx *sql.Tx, id uuid.UUID, fromState, toState, actorUserID, reason string,
) error {
	effect := mbhead.DeriveLockEffect(fromState, toState)
	if effect.Event == "" {
		return nil
	}
	return insertLockLogTx(ctx, tx, id, effect, actorUserID, reason)
}

//nolint:revive // Many parameters — one per persisted transition field, mirrors Transition's shape.
func (r *MBHeadRepository) updateEntryStatusTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, fromState, toState string, currentVersion int32, stateReason, actorUserID string, params *mbhead.ParamSnapshot) error {
	calc, err := r.deriveCheckStatusCalcTx(ctx, tx, id, toState)
	if err != nil {
		return err
	}

	q := `
		UPDATE mst_mb_head
		SET mbh_entry_status = $2, mbh_current_version = $3, mbh_state_reason = NULLIF($4, ''), updated_at = NOW()`
	args := []any{id, toState, currentVersion, stateReason}
	if params != nil {
		q += `,
		    mbh_param_waste = NULLIF($5, '')::numeric, mbh_param_quality_loss = NULLIF($6, '')::numeric,
		    mbh_param_efficiency = NULLIF($7, '')::numeric, mbh_param_dev_expense = NULLIF($8, '')::numeric,
		    mbh_param_packing = NULLIF($9, '')::numeric, mbh_param_mb_prod_per_day = NULLIF($10, '')::numeric,
		    mbh_param_throughput_per_hour = $11, mbh_param_no_of_process = $12`
		args = append(args, params.Waste, params.QualityLoss, params.Efficiency, params.DevExpense,
			params.Packing, params.MBProdPerDay, params.ThroughputPerHour, params.NoOfProcess)
	}
	// Derived check status (000487). Appended LAST so it never disturbs the fixed
	// $5..$12 numbering the params block above depends on. When calc is nil the
	// clause is omitted entirely: an undecided state leaves the stored value alone
	// rather than erasing it.
	if calc != nil {
		args = append(args, *calc)
		q += fmt.Sprintf(`,
		    mbh_check_status_calc = $%d`, len(args))
	}

	// P10 lock columns. Appended AFTER the calc clause for the same reason it was
	// appended last: the placeholder numbers are derived from len(args), so the fixed
	// $5..$12 numbering the params block relies on is never disturbed. When the
	// transition is not lock-related the fragment is empty and no lock column moves.
	lockFrag, args := lockClauses(mbhead.DeriveLockEffect(fromState, toState), actorUserID, stateReason, args)
	q += lockFrag

	q += ` WHERE mbh_id = $1 AND deleted_at IS NULL`

	result, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("mb_head_transition: update entry status: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mb_head_transition: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbhead.ErrNotFound
	}
	return nil
}

// deriveCheckStatusCalcTx computes the value mbh_check_status_calc should take once
// the row has moved to toState. The bought-out flag is read inside the SAME
// transaction so the derivation sees exactly the row being written.
//
// 🔴 The rules live in ONE place only — mbhead.DeriveCheckStatus, a pure Go function.
// ⛔ They are deliberately NOT expressed as a SQL CASE here: a second implementation
// of the same rules would drift from the first, and the only question would be when.
//
// A nil result means "this state has no decided mapping yet" — the caller then omits
// the column from the UPDATE. ⛔ It never writes mbh_check_status, which stays frozen
// as the Oracle import trace (user decision K-1, option 2).
func (r *MBHeadRepository) deriveCheckStatusCalcTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, toState string) (*string, error) {
	var isBoughtout bool
	err := tx.QueryRowContext(ctx,
		`SELECT mbh_is_boughtout FROM mst_mb_head WHERE mbh_id = $1 AND deleted_at IS NULL`, id,
	).Scan(&isBoughtout)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, mbhead.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mb_head_transition: read boughtout flag: %w", err)
	}
	return mbhead.DeriveCheckStatus(toState, isBoughtout), nil
}

// RefreezeCostParams updates the frozen mbh_param_* columns on mst_mb_head and re-runs the CPP
// (cost_product_parameter) freeze from the entity's current in-memory param getters. Unlike
// Validate, this does not change entry_status, does not bump the version, does not create a
// workflow-log row, and does not attempt auto-gen — it assumes the cost product already exists.
// Safe to run against already-VALIDATED heads whose frozen values were incorrect (e.g. after
// ENG-MB-01's fix for the throughput/no_of_process default bug).
func (r *MBHeadRepository) RefreezeCostParams(ctx context.Context, id uuid.UUID, entity *mbhead.Entity, params *mbhead.ParamSnapshot) error {
	if entity.CostProductID() == 0 {
		return fmt.Errorf("refreeze %s: cost product not yet generated", entity.MBCosting())
	}
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		// Update mst_mb_head frozen param columns in-place — no version bump, no status change.
		if _, err := tx.ExecContext(ctx, `
			UPDATE mst_mb_head SET
			    mbh_param_waste              = NULLIF($2, '')::numeric,
			    mbh_param_quality_loss       = NULLIF($3, '')::numeric,
			    mbh_param_efficiency         = NULLIF($4, '')::numeric,
			    mbh_param_dev_expense        = NULLIF($5, '')::numeric,
			    mbh_param_packing            = NULLIF($6, '')::numeric,
			    mbh_param_mb_prod_per_day    = NULLIF($7, '')::numeric,
			    mbh_param_throughput_per_hour = $8,
			    mbh_param_no_of_process       = $9,
			    updated_at = NOW()
			WHERE mbh_id = $1 AND deleted_at IS NULL
		`, id,
			params.Waste, params.QualityLoss, params.Efficiency, params.DevExpense,
			params.Packing, params.MBProdPerDay, params.ThroughputPerHour, params.NoOfProcess,
		); err != nil {
			return fmt.Errorf("refreeze: update mst_mb_head: %w", err)
		}

		// Re-run the CPP freeze (upsert — idempotent per ON CONFLICT ... DO UPDATE).
		if err := mbFreezeCostParams(ctx, tx, entity.CostProductID(), entity, ""); err != nil {
			return fmt.Errorf("refreeze: freeze CPP: %w", err)
		}
		return nil
	})
}

func (r *MBHeadRepository) insertWorkflowLogTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, fromState, toState, actorUserID, reason string, version int32) error {
	const q = `
		INSERT INTO mst_mb_workflow_log
			(mbwl_mbh_id, mbwl_from_state, mbwl_to_state, mbwl_actor_user_id, mbwl_reason, mbwl_version)
		VALUES ($1, NULLIF($2, ''), $3, $4, NULLIF($5, ''), $6)`
	_, err := tx.ExecContext(ctx, q, id, fromState, toState, actorUserID, reason, version)
	if err != nil {
		return fmt.Errorf("mb_head_transition: insert workflow log: %w", err)
	}
	return nil
}
