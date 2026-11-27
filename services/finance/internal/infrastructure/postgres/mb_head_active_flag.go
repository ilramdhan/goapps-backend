package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
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

// mbHeadBlockingUsersSQL lists the MBs that block deactivating $1: members of the batch
// candidate set (active heads + inactive heads an active head still needs, rule 1) whose
// CURRENT composition references $1. Current composition = frozen version snapshot for a
// versioned head, the working set for a never-validated one.
const mbHeadBlockingUsersSQL = `
		WITH RECURSIVE live AS (
		    SELECT mbh_id FROM mst_mb_head
		    WHERE mbh_entry_status = 'VALIDATED' AND mbh_is_active = TRUE AND deleted_at IS NULL
		  UNION
		    SELECT ref.mbh_id
		    FROM live l
		    JOIN mst_mb_head p ON p.mbh_id = l.mbh_id
		    JOIN mst_mb_composition_version v
		      ON v.mbcv_mbh_id = p.mbh_id AND v.mbcv_version = p.mbh_current_version
		     AND v.mbcv_source_type = 'MB'
		    JOIN mst_mb_head ref ON ref.mbh_id = v.mbcv_mb_ref_mbh_id
		     AND ref.mbh_entry_status = 'VALIDATED' AND ref.deleted_at IS NULL
		)
		SELECT DISTINCT u.mbh_mb_costing
		FROM mst_mb_head u
		WHERE u.deleted_at IS NULL AND u.mbh_id <> $1
		  AND (u.mbh_id IN (SELECT mbh_id FROM live) OR u.mbh_is_active)
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
		ORDER BY 1`

type activeFlagQuerier interface {
	supersedeQuerier
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func applyActiveFlagTx(ctx context.Context, tx *sql.Tx, entity *mbhead.Entity) error {
	var was bool
	err := tx.QueryRowContext(ctx,
		`SELECT mbh_is_active FROM mst_mb_head WHERE mbh_id = $1 AND deleted_at IS NULL FOR UPDATE`, entity.ID()).Scan(&was)
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
		var code string
		if err := rows.Scan(&code); err != nil {
			return fmt.Errorf("scan mb using head: %w", err)
		}
		usedBy = append(usedBy, code)
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

func reactivateHeadChildrenTx(ctx context.Context, tx *sql.Tx, entity *mbhead.Entity, actor string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE mst_mb_spin SET mbs_is_active = TRUE, updated_at = NOW(), updated_by = $2
		WHERE mbs_mbh_id = $1 AND deleted_at IS NULL AND mbs_parent_spin_id IS NULL
		  AND mbs_superseded_by_spin_id IS NULL AND mbs_superseded_at IS NULL AND NOT mbs_is_active`,
		entity.ID(), actor); err != nil {
		return fmt.Errorf("reactivate head root spin: %w", err)
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
	free, err := supersedeProductWillDeactivateOn(ctx, q, productSysID, []string{})
	if err != nil || !free {
		return false, err
	}
	var refs bool
	err = q.QueryRowContext(ctx, `
		SELECT EXISTS (
		    SELECT 1 FROM cost_route_rm r
		    JOIN cost_product_master p ON p.cpm_product_sys_id = r.crm_parent_product_sys_id
		    WHERE r.crm_rm_product_sys_id = $1 AND r.crm_parent_product_sys_id <> $1 AND p.cpm_is_active)
		OR EXISTS (SELECT 1 FROM mst_mb_spin WHERE mbs_cost_product_id = $1 AND deleted_at IS NULL AND mbs_is_active)`,
		productSysID).Scan(&refs)
	if err != nil {
		return false, fmt.Errorf("check mb product references: %w", err)
	}
	return !refs, nil
}
