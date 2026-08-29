// Package postgres provides PostgreSQL implementations for domain repositories.
//
// This file holds the WRITE side of the MB Spin child-recalc pass (phase P8-c)
// plus the unfiltered child read the pass needs in order to REPORT what it
// skipped.
//
// ⛔ ISOLATION: nothing here may reach into the calc-engine v2 (rmcost). The
// recalc chain stops at the child spin (decision D24): no yarn product, no
// cst_product_cost row, is written or recalculated from this file.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// Verify the recalc contract is satisfied at compile time.
var _ mbspin.RecalcRepository = (*MBSpinRepository)(nil)

// recalcWorkflowLogToState is the mbwl_to_state written for a child-recalc
// operation.
//
// ⚠ A recalc is NOT an mbh_entry_status transition, yet mbwl_to_state is
// NOT NULL (migration 000448:5). Rather than mint a new vocabulary token, this
// reuses the event name that user decision K-18 already fixed for the
// mbwl_meta payload, so the audit row and its meta document agree and no new
// state string enters the system. mbwl_from_state stays NULL: there is no state
// being left.
const recalcWorkflowLogToState = "DOZING_CHANGED"

// ListAllChildren returns EVERY non-deleted direct child of a parent spin,
// whatever its status — the superset of ListChildren.
//
// ⛔ ONE LEVEL ONLY. Like ListChildren this is a single flat query against
// mbs_parent_spin_id; there is no recursive CTE and no self-join, so a
// grandchild can never appear in the result (R13).
func (r *MBSpinRepository) ListAllChildren(ctx context.Context, parentID uuid.UUID) ([]*mbspin.Entity, error) {
	rows, err := r.db.QueryContext(ctx,
		r.selectCols()+` WHERE mbs_parent_spin_id = $1 AND deleted_at IS NULL
		                 ORDER BY created_at ASC, mbs_id ASC`,
		parentID,
	)
	if err != nil {
		return nil, fmt.Errorf("list all mb spin children: %w", err)
	}
	defer closeRows(rows)

	var items []*mbspin.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate all mb spin children: %w", err)
	}
	return items, nil
}

// ApplyChildRecalc writes one recalc OPERATION in one transaction.
//
// Per child in in.Updates: mbs_dozing plus the mbs_last_recalc_at / by trail
// (migration 000484 — this is the FIRST write path those two columns have ever
// had; Create and Update do not touch them).
//
// Per child in in.LDRUpdates (Task D): mbs_ldr_calculated_pct plus
// mbs_ldr_type = 'CALCULATED' and the same mbs_last_recalc_at/by audit trail.
// A child may appear in BOTH Updates and LDRUpdates in the same call — the two
// lists are written by two independent statements, so a child touched by both
// gets both sets of columns updated without either overwriting the other.
//
// Per OPERATION: exactly ONE mst_mb_workflow_log row carrying mbwl_meta, ⛔ never
// one row per child (plan §P8 "Jejak"). The row is written even when Updates and
// LDRUpdates are both empty, because "this operation was eligible to change
// nothing" is itself the fact the audit trail must keep.
//
// ⛔ Both UPDATE statements re-assert the candidate predicate in their WHERE
// clause (status = 'R and D', not deleted, parent matches). The caller already
// filtered, but a row that changed status between the read and the write must
// NOT be overwritten — A7 is enforced twice on purpose, and the second time is
// the one that holds under concurrency.
func (r *MBSpinRepository) ApplyChildRecalc(ctx context.Context, in mbspin.RecalcApplyInput) error {
	if in.HeadID == uuid.Nil {
		return mbspin.ErrInvalidHeadID
	}
	if in.Actor == "" {
		return mbspin.ErrEmptyCreatedBy
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin recalc tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	if err := r.updateRecalcedChildren(ctx, tx, in); err != nil {
		return err
	}
	if err := r.updateRecalcedLDRChildren(ctx, tx, in); err != nil {
		return err
	}
	if err := r.insertRecalcLog(ctx, tx, in); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recalc: %w", err)
	}
	committed = true
	return nil
}

func (r *MBSpinRepository) updateRecalcedChildren(ctx context.Context, tx *sql.Tx, in mbspin.RecalcApplyInput) error {
	const q = `
		UPDATE mst_mb_spin SET
			mbs_dozing         = $2,
			mbs_last_recalc_at = $3,
			mbs_last_recalc_by = $4,
			updated_at         = $3,
			updated_by         = $4
		WHERE mbs_id = $1
		  AND mbs_parent_spin_id = $5
		  AND deleted_at IS NULL
		  AND mbs_status = $6`
	for i := range in.Updates {
		u := in.Updates[i]
		if _, err := tx.ExecContext(ctx, q,
			u.SpinID, u.NewDozing, in.At, in.Actor, in.ParentSpinID, mbspin.StatusRnD,
		); err != nil {
			return fmt.Errorf("recalc child mb spin %s: %w", u.SpinID, err)
		}
	}
	return nil
}

// updateRecalcedLDRChildren writes the Task D LDR half of the pass: each child
// in in.LDRUpdates gets mbs_ldr_calculated_pct rewritten, mbs_ldr_type flipped
// to CALCULATED (mbspin.LDRTypeCalculated), and the same audit trail columns
// the dozing branch above sets (mbs_last_recalc_at/by), reusing the same in.At
// / in.Actor so both halves of one pass are stamped identically. Deliberately
// separate from updateRecalcedChildren: a child can be dozing-only,
// LDR-only, both, or (if absent from both slices) neither, per Task D
// business rule 7.
func (r *MBSpinRepository) updateRecalcedLDRChildren(ctx context.Context, tx *sql.Tx, in mbspin.RecalcApplyInput) error {
	const q = `
		UPDATE mst_mb_spin SET
			mbs_ldr_calculated_pct = $2,
			mbs_ldr_type           = $3,
			mbs_last_recalc_at     = $4,
			mbs_last_recalc_by     = $5,
			updated_at             = $4,
			updated_by             = $5
		WHERE mbs_id = $1
		  AND mbs_parent_spin_id = $6
		  AND deleted_at IS NULL
		  AND mbs_status = $7`
	for i := range in.LDRUpdates {
		u := in.LDRUpdates[i]
		if _, err := tx.ExecContext(ctx, q,
			u.SpinID, u.NewLDRCalculatedPct, mbspin.LDRTypeCalculated, in.At, in.Actor,
			in.ParentSpinID, mbspin.StatusRnD,
		); err != nil {
			return fmt.Errorf("recalc child mb spin LDR %s: %w", u.SpinID, err)
		}
	}
	return nil
}

func (r *MBSpinRepository) insertRecalcLog(ctx context.Context, tx *sql.Tx, in mbspin.RecalcApplyInput) error {
	// Reuses the same statement as MBWorkflowLogRepository.Create so the two
	// write paths cannot drift on which columns they populate.
	_, err := tx.ExecContext(ctx, insertWorkflowLogQuery,
		in.HeadID, "", recalcWorkflowLogToState, in.Actor, in.LogReason, 0, in.LogMeta,
	)
	if err != nil {
		return fmt.Errorf("insert recalc workflow log: %w", err)
	}
	return nil
}
