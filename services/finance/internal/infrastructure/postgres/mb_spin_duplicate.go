// Package postgres provides PostgreSQL implementations for domain repositories.
//
// This file holds the MB Spin duplicate/lineage PRIMITIVES only (phase P8-b).
// ⛔ No recalc rule, no dozing scaling, no impact preview lives here — those are
// application-layer concerns. This layer answers three questions and nothing
// more: clone one row, list a parent's recalc candidates, is this ORION code
// taken.
//
// ⛔ ISOLATION: nothing in this file may reach into the calc-engine v2 (rmcost).
// Duplicating a spin never propagates into yarn products (D24) — the chain stops
// at the child spin.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// DuplicateSpin transactionally clones one spin into a fresh R&D child.
//
// Steps, in the order the design fixes them:
//  1. BEGIN
//  2. SELECT ... FOR UPDATE the source row — the lock is what makes the cycle
//     check meaningful: without it a concurrent re-parent could slip in between
//     the walk-up and the INSERT.
//  3. assertNoParentCycle via the pure domain rule, reading through this same tx
//  4. INSERT the clone
//  5. COMMIT
func (r *MBSpinRepository) DuplicateSpin(ctx context.Context, in mbspin.DuplicateInput) (mbspin.DuplicateOutput, error) {
	if in.SourceSpinID == uuid.Nil {
		return mbspin.DuplicateOutput{}, mbspin.ErrNotFound
	}
	if in.ActorUserID == "" {
		return mbspin.DuplicateOutput{}, mbspin.ErrEmptyCreatedBy
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return mbspin.DuplicateOutput{}, fmt.Errorf("begin tx: %w", err)
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

	src, err := r.lockSourceSpin(ctx, tx, in.SourceSpinID)
	if err != nil {
		return mbspin.DuplicateOutput{}, err
	}

	depth, err := mbspin.AssertNoParentCycle(in.SourceSpinID, r.txParentLookup(ctx, tx))
	if err != nil {
		return mbspin.DuplicateOutput{}, err
	}

	name, err := mbspin.CloneMgtName(src.MgtName, in.MgtName)
	if err != nil {
		return mbspin.DuplicateOutput{}, err
	}

	newID, err := r.insertClone(ctx, tx, src, in, name)
	if err != nil {
		return mbspin.DuplicateOutput{}, err
	}

	if err := tx.Commit(); err != nil {
		return mbspin.DuplicateOutput{}, fmt.Errorf("commit duplicate spin: %w", err)
	}
	committed = true

	return mbspin.DuplicateOutput{
		NewSpinID:    newID,
		ParentSpinID: in.SourceSpinID,
		HeadID:       src.HeadID,
		MgtName:      name,
		LineageDepth: depth,
	}, nil
}

// ListChildren returns the direct recalc CANDIDATES of a parent spin (A6).
//
// The status filter is part of the contract, not an optimization: non-R&D
// children are excluded here (A7) so no caller can accidentally overwrite an
// actual production row. ⛔ Single level — this never recurses (R13).
func (r *MBSpinRepository) ListChildren(ctx context.Context, parentID uuid.UUID) ([]*mbspin.Entity, error) {
	rows, err := r.db.QueryContext(ctx,
		r.selectCols()+` WHERE mbs_parent_spin_id = $1 AND deleted_at IS NULL AND mbs_status = $2
		                 ORDER BY created_at ASC, mbs_id ASC`,
		parentID, mbspin.StatusRnD,
	)
	if err != nil {
		return nil, fmt.Errorf("list mb spin children: %w", err)
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
		return nil, fmt.Errorf("iterate mb spin children: %w", err)
	}
	return items, nil
}

// ExistsByOrionItemCode reports whether the code is already taken by a live spin.
//
// ⚠ Callers must invoke this ONLY for a code that is changing (see the interface
// doc): running it on an unchanged code would reject every one of the 466 legacy
// rows that share a duplicated code.
func (r *MBSpinRepository) ExistsByOrionItemCode(ctx context.Context, code string) (bool, error) {
	if code == "" {
		return false, nil
	}
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_mb_spin WHERE mbs_orion_item_code = $1 AND deleted_at IS NULL)`, code,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by orion item code: %w", err)
	}
	return exists, nil
}

// =============================================================================
// Helpers
// =============================================================================

// mbSpinCloneSource carries only the columns the clone actually needs. It is
// deliberately narrower than mbSpinDTO: reading the whole row would invite
// copying a column that A5/D19 says must be nulled.
type mbSpinCloneSource struct {
	HeadID          uuid.UUID
	MgtName         string
	Denier          sql.NullFloat64
	Filament        sql.NullInt64
	Dozing          sql.NullFloat64
	CC              sql.NullString
	CostRateMkt     sql.NullFloat64
	MBSLdrPrsn      sql.NullFloat64
	MBSRunLdrPct    sql.NullFloat64
	MBSFinalProduct sql.NullString
	Lesture         sql.NullString
	CostProductID   sql.NullInt64
}

// lockSourceSpin loads the clonable columns of the source under FOR UPDATE.
//
// mbs_cost_product_id is taken from the parent HEAD, not from the source spin:
// the head is the owner of record (the spin column is a derived copy, D18), so
// reading the head keeps a clone correct even for a source row that predates the
// 000490 backfill. NULL is a legitimate result for a DRAFT head (D23).
//
// FOR UPDATE OF s pins only mst_mb_spin — the joined head must not be locked by
// a duplicate.
func (r *MBSpinRepository) lockSourceSpin(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*mbSpinCloneSource, error) {
	var s mbSpinCloneSource
	err := tx.QueryRowContext(ctx, `
		SELECT s.mbs_mbh_id, s.mbs_mgt_name,
		       s.mbs_denier, s.mbs_filament, s.mbs_dozing,
		       s.mbs_cc, s.mbs_cost_rate_mkt,
		       s.mbs_ldr_prsn, s.mbs_run_ldr_pct, s.mbs_final_product,
		       s.mbs_lesture, h.mbh_cost_product_id
		  FROM mst_mb_spin s
		  JOIN mst_mb_head h ON h.mbh_id = s.mbs_mbh_id
		 WHERE s.mbs_id = $1 AND s.deleted_at IS NULL
		   FOR UPDATE OF s`, id,
	).Scan(
		&s.HeadID, &s.MgtName,
		&s.Denier, &s.Filament, &s.Dozing,
		&s.CC, &s.CostRateMkt,
		&s.MBSLdrPrsn, &s.MBSRunLdrPct, &s.MBSFinalProduct,
		&s.Lesture, &s.CostProductID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, mbspin.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock source mb spin: %w", err)
	}
	return &s, nil
}

// txParentLookup adapts a single mbs_parent_spin_id read to the domain's
// ParentLookup, bound to this transaction so the walk-up sees the same snapshot
// the FOR UPDATE established.
//
// A row that has vanished reports "no parent" rather than an error: fk_mbs_parent_spin
// is ON DELETE SET NULL and an ancestor may be soft-deleted, so a dangling
// pointer terminates the chain instead of failing the duplicate.
func (r *MBSpinRepository) txParentLookup(ctx context.Context, tx *sql.Tx) mbspin.ParentLookup {
	return func(id uuid.UUID) (*uuid.UUID, error) {
		var parent sql.NullString
		err := tx.QueryRowContext(ctx,
			`SELECT mbs_parent_spin_id FROM mst_mb_spin WHERE mbs_id = $1`, id,
		).Scan(&parent)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read parent spin id: %w", err)
		}
		return nullableUUIDPtr(parent), nil
	}
}

// insertClone writes the clone row. The column policy of A5/D19 is expressed
// literally in the SQL so it can be read off the statement:
//
//	mbs_oracle_sys_id / mbs_orion_item_code / mbs_mb_costing => NULL
//	mbs_status                                               => 'R and D' (D5)
//	mbs_ldr_is_fixed / mbs_dozing_is_fixed                   => FALSE, explicit
//
// ⚠ The two FALSE literals are MANDATORY and must never become NULL: NULL is
// interpreted as "fixed" (mbspin.IsFixedLDR / IsFixedDozing), so a NULL clone
// would be silently and permanently excluded from every future recalc.
func (r *MBSpinRepository) insertClone(
	ctx context.Context, tx *sql.Tx, src *mbSpinCloneSource, in mbspin.DuplicateInput, name string,
) (uuid.UUID, error) {
	denier := src.Denier
	if in.Denier != nil {
		denier = sql.NullFloat64{Float64: *in.Denier, Valid: true}
	}
	filament := src.Filament
	if in.Filament != nil {
		filament = sql.NullInt64{Int64: int64(*in.Filament), Valid: true}
	}

	newID := uuid.New()
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		INSERT INTO mst_mb_spin (
			mbs_id, mbs_mbh_id, mbs_mgt_name,
			mbs_oracle_sys_id, mbs_orion_item_code, mbs_mb_costing,
			mbs_denier, mbs_filament, mbs_dozing,
			mbs_cc, mbs_cost_rate_mkt,
			mbs_status, mbs_ldr_prsn, mbs_run_ldr_pct, mbs_final_product, mbs_lesture,
			mbs_ldr_is_fixed, mbs_dozing_is_fixed,
			mbs_parent_spin_id, mbs_duplicated_at, mbs_duplicated_by,
			mbs_cost_product_id,
			mbs_is_active, created_at, created_by
		) VALUES (
			$1, $2, $3,
			NULL, NULL, NULL,
			$4, $5, $6,
			$7, $8,
			$9, $10, $11, $12, $13,
			FALSE, FALSE,
			$14, $15, $16,
			$17,
			TRUE, $18, $16
		)`,
		newID, src.HeadID, name,
		denier, filament, src.Dozing,
		src.CC, src.CostRateMkt,
		mbspin.StatusRnD, src.MBSLdrPrsn, src.MBSRunLdrPct, src.MBSFinalProduct, src.Lesture,
		in.SourceSpinID, now, in.ActorUserID,
		src.CostProductID,
		now,
	)
	if err != nil {
		if isMBSpinUniqueViolation(err) {
			return uuid.Nil, mbspin.ErrAlreadyExists
		}
		return uuid.Nil, fmt.Errorf("insert cloned mb spin: %w", err)
	}
	return newID, nil
}
