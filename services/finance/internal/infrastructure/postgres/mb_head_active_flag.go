package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// Active-flag rules (mb-active-flag-rules 3+4). applyActiveFlagTx runs inside the head update
// transaction of the APP paths (MBHeadRepository.Update / UpdateWithCascade) BEFORE the head
// row is written, and compares the entity's new is_active with the stored one:
//
//   - true -> false: rejected with *mbhead.InUseError while the head is in use; otherwise its
//     live spins go inactive and its master product too, unless something live still needs it.
//   - false -> true: root spin (not child, not superseded) and master product go active.
//
// The data tools (mb_active_sync_writer*.go, legacy-fill-mb, mb-comparison-fill) write
// mbh_is_active with their own direct SQL and deliberately never call this.

// mbHeadBlockingUsersSQL lists what blocks deactivating $1, as (name, via) rows. A "user" is
// any non-deleted head whose CURRENT composition references $1 (frozen version snapshot for
// a versioned head, the working set for a never-validated one) and that is either active or
// in the batch candidate set (rule 1: an inactive head an active head still needs).
// DELIBERATE: active heads of ANY entry status count as users (stricter than the batch set).
// An active user names itself; an inactive chain member names the ACTIVE root(s) that need it.
const mbHeadBlockingUsersSQL = `
		WITH RECURSIVE reach(root_id, mbh_id) AS (
		    SELECT mbh_id, mbh_id FROM mst_mb_head
		    WHERE mbh_entry_status = 'VALIDATED' AND mbh_is_active = TRUE AND deleted_at IS NULL
		  UNION
		    SELECT r.root_id, ref.mbh_id
		    FROM reach r
		    JOIN mst_mb_head p ON p.mbh_id = r.mbh_id
		    JOIN mst_mb_composition_version v
		      ON v.mbcv_mbh_id = p.mbh_id AND v.mbcv_version = p.mbh_current_version
		     AND v.mbcv_source_type = 'MB'
		    JOIN mst_mb_head ref ON ref.mbh_id = v.mbcv_mb_ref_mbh_id
		     AND ref.mbh_entry_status = 'VALIDATED' AND ref.deleted_at IS NULL
		), users AS (
		    SELECT u.mbh_id, u.mbh_mb_costing, u.mbh_is_active
		    FROM mst_mb_head u
		    WHERE u.deleted_at IS NULL AND u.mbh_id <> $1
		      AND (u.mbh_is_active OR u.mbh_id IN (SELECT mbh_id FROM reach))
		      AND (
		        (u.mbh_current_version > 0 AND EXISTS (
		            SELECT 1 FROM mst_mb_composition_version v
		            WHERE v.mbcv_mbh_id = u.mbh_id AND v.mbcv_version = u.mbh_current_version
		              AND v.mbcv_source_type = 'MB' AND v.mbcv_mb_ref_mbh_id = $1))
		        OR (u.mbh_current_version = 0 AND EXISTS (
		            SELECT 1 FROM mst_mb_composition c
		            WHERE c.mbcm_mbh_id = u.mbh_id AND c.deleted_at IS NULL
		              AND c.mbcm_source_type = 'MB' AND c.mbcm_mb_ref_mbh_id = $1))
		      )
		)
		SELECT name, via FROM (
		    SELECT mbh_mb_costing AS name, mbh_mb_costing AS via FROM users WHERE mbh_is_active
		  UNION
		    SELECT root.mbh_mb_costing, u.mbh_mb_costing
		    FROM users u
		    JOIN reach ON reach.mbh_id = u.mbh_id
		    JOIN mst_mb_head root ON root.mbh_id = reach.root_id
		    WHERE NOT u.mbh_is_active
		) x ORDER BY name, via`

type activeFlagQuerier interface {
	supersedeQuerier
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func applyActiveFlagTx(ctx context.Context, tx *sql.Tx, entity *mbhead.Entity) error {
	var was, superseded bool
	err := tx.QueryRowContext(ctx,
		`SELECT mbh_is_active, mbh_superseded_by_head_id IS NOT NULL FROM mst_mb_head WHERE mbh_id = $1 AND deleted_at IS NULL FOR UPDATE`, entity.ID()).Scan(&was, &superseded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // updateOn reports the missing row exactly as before
	}
	if err != nil {
		return fmt.Errorf("read stored mb head is_active: %w", err)
	}
	if was == entity.IsActive() {
		return nil
	}
	actor := ""
	if entity.UpdatedBy() != nil {
		actor = *entity.UpdatedBy()
	}
	if entity.IsActive() {
		if superseded {
			return mbhead.ErrReactivateSuperseded
		}
		return reactivateHeadChildrenTx(ctx, tx, entity, actor)
	}
	return deactivateHeadChildrenTx(ctx, tx, entity, actor)
}

func deactivateHeadChildrenTx(ctx context.Context, tx *sql.Tx, entity *mbhead.Entity, actor string) error {
	rows, err := tx.QueryContext(ctx, mbHeadBlockingUsersSQL, entity.ID())
	if err != nil {
		return fmt.Errorf("list mbs using head: %w", err)
	}
	defer closeRows(rows)
	var usedBy []string
	for rows.Next() {
		var name, via string
		if err := rows.Scan(&name, &via); err != nil {
			return fmt.Errorf("scan mb using head: %w", err)
		}
		if name != via {
			name += " (via " + via + ")"
		}
		usedBy = append(usedBy, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate mbs using head: %w", err)
	}
	if len(usedBy) > 0 {
		return &mbhead.InUseError{UsedBy: usedBy}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE mst_mb_spin SET mbs_is_active = FALSE, updated_at = NOW(), updated_by = $2
		WHERE mbs_mbh_id = $1 AND deleted_at IS NULL AND mbs_is_active`, entity.ID(), actor); err != nil {
		return fmt.Errorf("deactivate head spins: %w", err)
	}
	pid := entity.CostProductID()
	if pid <= 0 {
		return nil
	}
	free, err := mbProductFreeToDeactivateOn(ctx, tx, pid)
	if err != nil || !free {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE cost_product_master SET cpm_is_active = FALSE, cpm_updated_at = NOW(), cpm_updated_by = $2
		WHERE cpm_product_sys_id = $1 AND cpm_is_active`, pid, actor); err != nil {
		return fmt.Errorf("deactivate head mb product: %w", err)
	}
	return nil
}

// reactivateHeadChildrenTx reactivates ONE root spin (the same deterministic pick as
// mbasHeadSpins: non-superseded root, oldest first) and the master product. Choice: the
// product is reactivated only when a root spin was reactivated, already is active, or the
// head has no spins at all; a head whose only roots are superseded keeps its product off.
func reactivateHeadChildrenTx(ctx context.Context, tx *sql.Tx, entity *mbhead.Entity, actor string) error {
	var rootActive, hasRoot bool
	err := tx.QueryRowContext(ctx, `
		SELECT mbs_is_active FROM mst_mb_spin
		WHERE mbs_mbh_id = $1 AND deleted_at IS NULL AND mbs_parent_spin_id IS NULL
		  AND mbs_superseded_by_spin_id IS NULL AND mbs_superseded_at IS NULL
		ORDER BY created_at ASC, mbs_id ASC LIMIT 1`, entity.ID()).Scan(&rootActive)
	switch {
	case err == nil:
		hasRoot = true
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("read head root spin: %w", err)
	}
	if hasRoot && !rootActive {
		if _, err := tx.ExecContext(ctx, `
			UPDATE mst_mb_spin SET mbs_is_active = TRUE, updated_at = NOW(), updated_by = $2
			WHERE mbs_id = (
			    SELECT mbs_id FROM mst_mb_spin
			    WHERE mbs_mbh_id = $1 AND deleted_at IS NULL AND mbs_parent_spin_id IS NULL
			      AND mbs_superseded_by_spin_id IS NULL AND mbs_superseded_at IS NULL
			    ORDER BY created_at ASC, mbs_id ASC LIMIT 1)`, entity.ID(), actor); err != nil {
			if isSpinShadeFgViolation(err) {
				return mbspin.ErrSpinShadeFgExists
			}
			return fmt.Errorf("reactivate head root spin: %w", err)
		}
	}
	if !hasRoot {
		var anySpin bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM mst_mb_spin WHERE mbs_mbh_id = $1 AND deleted_at IS NULL)`,
			entity.ID()).Scan(&anySpin); err != nil {
			return fmt.Errorf("read head spins: %w", err)
		}
		if anySpin {
			return nil // only superseded roots / children: product stays as is
		}
	}
	if pid := entity.CostProductID(); pid > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE cost_product_master SET cpm_is_active = TRUE, cpm_updated_at = NOW(), cpm_updated_by = $2
			WHERE cpm_product_sys_id = $1 AND NOT cpm_is_active`, pid, actor); err != nil {
			return fmt.Errorf("reactivate head mb product: %w", err)
		}
	}
	return nil
}

// mbProductFreeToDeactivateOn is the shared "no live reference remains" check for an MB
// master product: the supersede check (no live active spin points at it, supersedeProductWillDeactivateOn,
// unchanged) plus the cost_route_rm product refs of active parent products.
// (FG links point at POY products, never at the MB product, so they cannot reference it.)
func mbProductFreeToDeactivateOn(ctx context.Context, q activeFlagQuerier, productSysID int64) (bool, error) {
	// supersedeProductWillDeactivateOn already covers "no live active spin points at it".
	free, err := supersedeProductWillDeactivateOn(ctx, q, productSysID, []string{})
	if err != nil || !free {
		return false, err
	}
	// cost_route_rm under a soft-deleted route seq still counts: deliberately conservative.
	var refs bool
	err = q.QueryRowContext(ctx, `
		SELECT EXISTS (
		    SELECT 1 FROM cost_route_rm r
		    JOIN cost_product_master p ON p.cpm_product_sys_id = r.crm_parent_product_sys_id
		    WHERE r.crm_rm_product_sys_id = $1 AND r.crm_parent_product_sys_id <> $1 AND p.cpm_is_active)`,
		productSysID).Scan(&refs)
	if err != nil {
		return false, fmt.Errorf("check mb product references: %w", err)
	}
	return !refs, nil
}
