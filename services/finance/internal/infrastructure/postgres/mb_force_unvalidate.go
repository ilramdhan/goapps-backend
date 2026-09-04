package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// ForceUnvalidateTransition atomically forces a VALIDATED head back to DRAFT for the Bulk
// MB Head Regenerate feature (Phase B, Super Admin only). In ONE transaction it:
//  1. updates mst_mb_head's entry_status to DRAFT, sets state_reason, and clears the P10
//     lock columns (mbh_is_locked/mbh_unlock_requested_at/mbh_unlock_requested_by) so the
//     head is fully editable again — mirrors GrantUnlock's lock clearing, but skips
//     mst_mb_head_lock_log because this is not a lock-related transition per DeriveLockEffect;
//  2. deliberately LEAVES mbh_cost_product_id/mbh_cost_generated_at/mbh_cost_generated_by
//     untouched so a subsequent Bulk Validate takes the lighter regenerateCostProductRMs
//     path instead of the FULL autoGenCostProduct path (see mb_autogen_repository.go's
//     TransitionWithAutoGen). Resetting these columns to NULL here used to force every
//     re-validate back onto autoGenCostProduct, which duplicated cost_product_master and
//     mst_mb_spin rows on every regenerate and even re-generated MB Spin rows already
//     locked as actual (mbs_ldr_is_actual = TRUE). Preserving the cost lineage columns
//     keeps regenerateCostProductRMs (which never touches mst_mb_spin) in the loop, which
//     both prevents the duplication and automatically protects locked-actual spin rows;
//  3. inserts a mst_mb_workflow_log audit row via the same helper Transition uses.
//
// ⛔ Unlike Transition/TransitionWithAutoGen, this codebase's transition methods never guard
// on currentVersion in the WHERE clause — none of them do, and this mirrors that convention.
// currentVersion is still recorded on the workflow-log row, exactly as Transition does.
func (r *MBHeadRepository) ForceUnvalidateTransition(ctx context.Context, id uuid.UUID, currentVersion int, stateReason, actorUserID string) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := r.forceUnvalidateUpdateTx(ctx, tx, id, stateReason, actorUserID); err != nil {
			return err
		}
		version := currentVersion
		if version > math.MaxInt32 {
			version = math.MaxInt32
		}
		if err := r.insertWorkflowLogTx(ctx, tx, id, mbhead.StatusValidated, mbhead.StatusDraft, actorUserID, stateReason, int32(version)); err != nil { //nolint:gosec // clamped to MaxInt32 above
			return err
		}
		return nil
	})
}

func (r *MBHeadRepository) forceUnvalidateUpdateTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, stateReason, actorUserID string) error {
	const q = `
		UPDATE mst_mb_head
		SET mbh_entry_status = $2,
		    mbh_state_reason = NULLIF($3, ''),
		    mbh_is_locked = FALSE,
		    mbh_unlock_requested_at = NULL,
		    mbh_unlock_requested_by = NULL,
		    updated_at = NOW(),
		    updated_by = NULLIF($4, '')
		WHERE mbh_id = $1 AND deleted_at IS NULL`
	result, err := tx.ExecContext(ctx, q, id, mbhead.StatusDraft, stateReason, actorUserID)
	if err != nil {
		return fmt.Errorf("mb_force_unvalidate: update mst_mb_head: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mb_force_unvalidate: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbhead.ErrNotFound
	}
	return nil
}
