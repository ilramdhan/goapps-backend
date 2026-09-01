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

	// Only a root spin (mbs_parent_spin_id IS NULL) may be duplicated: this caps
	// lineage depth at one level and rejects re-duplicating an already-cloned
	// spin. Checked here, at the same locked-row snapshot used by
	// AssertNoParentCycle below, so a concurrent re-parent cannot slip past it —
	// mirroring why the source row is loaded FOR UPDATE in the first place.
	//
	// A self-referencing parent (mbs_parent_spin_id = mbs_id) is not an ordinary
	// "already duplicated" source — migration 000484 deliberately allows the DB
	// to store that self-loop, and it must surface as ErrParentCycle so the
	// caller sees a lineage-integrity failure, not a routine re-duplication
	// rejection.
	if src.ParentSpinID.Valid {
		if parentID := nullableUUIDPtr(src.ParentSpinID); parentID != nil && *parentID == in.SourceSpinID {
			return mbspin.DuplicateOutput{}, mbspin.ErrParentCycle
		}
		return mbspin.DuplicateOutput{}, mbspin.ErrAlreadyDuplicated
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
	// Shade/cross-section/VS/lusture copy-down (D31/D32 gate, added P1-T1).
	ShadeCode    sql.NullString
	ShadeName    sql.NullString
	CrossSection sql.NullString
	VSNumber     sql.NullString
	LustureCode  sql.NullString
	// LDR provenance (D32=A). LDRType and LDRIsActual are NOT NULL in storage
	// (see mbSpinDTO), so they read directly without a sql.Null wrapper.
	LDRCalculatedPct sql.NullFloat64
	LDRAdjustmentPct sql.NullFloat64
	LDRType          string
	LDRIsActual      bool
	// MBCosting is read from the source so it can be copied onto the clone
	// (D31=B) — previously absent from this struct because insertClone nulled
	// mbs_mb_costing directly instead of copying it.
	MBCosting sql.NullString
	// ParentSpinID is the source's OWN mbs_parent_spin_id (not the clone's,
	// which is always set to the source's id). A non-NULL value here means the
	// source is itself a previously duplicated child, which DuplicateSpin must
	// reject (ErrAlreadyDuplicated) before any further lineage/insert work.
	ParentSpinID sql.NullString
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
		       s.mbs_lesture, h.mbh_cost_product_id,
		       s.mbs_shade_code, s.mbs_shade_name, s.mbs_cross_section,
		       s.mbs_vs_number, s.mbs_lusture_code,
		       s.mbs_ldr_calculated_pct, s.mbs_ldr_adjustment_pct,
		       s.mbs_ldr_type, s.mbs_ldr_is_actual,
		       s.mbs_mb_costing, s.mbs_parent_spin_id
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
		&s.ShadeCode, &s.ShadeName, &s.CrossSection,
		&s.VSNumber, &s.LustureCode,
		&s.LDRCalculatedPct, &s.LDRAdjustmentPct,
		&s.LDRType, &s.LDRIsActual,
		&s.MBCosting, &s.ParentSpinID,
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

// insertClone writes the clone row. The column policy of A5/D19 (as amended by
// D31/D32) is expressed literally in the SQL so it can be read off the
// statement:
//
//	mbs_oracle_sys_id / mbs_orion_item_code                  => NULL (D19, unchanged)
//	mbs_status                                                => 'R and D' (D5)
//	mbs_ldr_is_fixed / mbs_dozing_is_fixed                    => FALSE, explicit
//	mbs_mb_costing                                            => COPIED from source (D31=B)
//	mbs_shade_code / mbs_shade_name / mbs_cross_section       => COPIED from source
//	mbs_vs_number / mbs_lusture_code                          => COPIED from source
//	mbs_ldr_calculated_pct / mbs_ldr_adjustment_pct           => COPIED from source (D32=A)
//	mbs_ldr_type                                              => source value, except
//	                                                              ACTUAL source downgrades
//	                                                              to CALCULATED on the clone (D32=A)
//	mbs_ldr_is_actual                                         => ALWAYS FALSE, regardless of
//	                                                              source (D32=A)
//
// ⚠ The two FALSE literals for mbs_ldr_is_fixed / mbs_dozing_is_fixed are
// MANDATORY and must never become NULL: NULL is interpreted as "fixed"
// (mbspin.IsFixedLDR / IsFixedDozing), so a NULL clone would be silently and
// permanently excluded from every future recalc.
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

	// D32=A: a clone never inherits an ACTUAL LDR type — it is always demoted
	// to CALCULATED, since mbs_ldr_is_actual on the clone is unconditionally
	// FALSE below.
	ldrType := src.LDRType
	if ldrType == mbspin.LDRTypeActual {
		ldrType = mbspin.LDRTypeCalculated
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
			mbs_shade_code, mbs_shade_name, mbs_cross_section,
			mbs_vs_number, mbs_lusture_code,
			mbs_ldr_calculated_pct, mbs_ldr_adjustment_pct, mbs_ldr_type, mbs_ldr_is_actual,
			mbs_parent_spin_id, mbs_duplicated_at, mbs_duplicated_by,
			mbs_cost_product_id,
			mbs_is_active, created_at, created_by
		) VALUES (
			$1, $2, $3,
			NULL, NULL, $4,
			$5, $6, $7,
			$8, $9,
			$10, $11, $12, $13, $14,
			FALSE, FALSE,
			$15, $16, $17,
			$18, $19,
			$20, $21, $22, FALSE,
			$23, $24, $25,
			$26,
			TRUE, $27, $25
		)`,
		newID, src.HeadID, name,
		src.MBCosting,
		denier, filament, src.Dozing,
		src.CC, src.CostRateMkt,
		mbspin.StatusRnD, src.MBSLdrPrsn, src.MBSRunLdrPct, src.MBSFinalProduct, src.Lesture,
		src.ShadeCode, src.ShadeName, src.CrossSection,
		src.VSNumber, src.LustureCode,
		src.LDRCalculatedPct, src.LDRAdjustmentPct, ldrType,
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
