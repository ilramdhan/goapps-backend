package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// GENERATE_SPIN role=ROOT for a head that already has its MB cost product but no spin
// (K7, design part 2 §5.5). The root spin is built by the SAME auto-gen builder the
// first Validate uses (mbAutoGenSpinOrionCode + mbBuildAutoGenSpin + createSpinOn), in
// one transaction with the head row locked. Heads without cost product go through
// mbactivesync.HeadWorkflow (Submit → Validate) instead.

// mbasGenImage is the before/after image of a GenerateRootSpin item.
type mbasGenImage struct {
	HeadID        uuid.UUID  `json:"head_id"`
	CostProductID int64      `json:"mbh_cost_product_id"`
	RootSpinID    *uuid.UUID `json:"root_spin_id"`
}

// mbasLockHeadEntityOn loads the head entity with its row locked (existing head reader
// columns). mbhead.ErrNotFound when missing or soft-deleted.
func mbasLockHeadEntityOn(ctx context.Context, tx *sql.Tx, headID uuid.UUID) (*mbhead.Entity, error) {
	r := &MBHeadRepository{}
	return r.scanOne(tx.QueryRowContext(ctx,
		r.selectCols()+` WHERE mbh_id = $1 AND deleted_at IS NULL FOR UPDATE OF mst_mb_head`, headID))
}

// mbasHeadSpins reports the head's live root spin (uuid.Nil if none) and whether the
// head has any non-deleted spin at all.
func mbasHeadSpins(ctx context.Context, tx *sql.Tx, headID uuid.UUID) (root uuid.UUID, anySpin bool, err error) {
	const q = `
		SELECT COUNT(*) > 0,
		       (SELECT mbs_id FROM mst_mb_spin
		         WHERE mbs_mbh_id = $1 AND deleted_at IS NULL AND mbs_parent_spin_id IS NULL
		           AND mbs_superseded_by_spin_id IS NULL
		         ORDER BY created_at ASC, mbs_id ASC LIMIT 1)
		  FROM mst_mb_spin
		 WHERE mbs_mbh_id = $1 AND deleted_at IS NULL`
	var rootID uuid.NullUUID
	if err := tx.QueryRowContext(ctx, q, headID).Scan(&anySpin, &rootID); err != nil {
		return uuid.Nil, false, fmt.Errorf("read head spins: %w", err)
	}
	if rootID.Valid {
		root = rootID.UUID
	}
	return root, anySpin, nil
}

// GenerateRootSpin implements mbactivesync.Writer (K7). Idempotent: a head that already
// has a live root spin → SKIPPED already_in_target (SpinID = that root).
func (w *MBActiveSyncWriter) GenerateRootSpin(ctx context.Context, in mbactivesync.GenerateRootInput) (mbactivesync.ItemResult, error) {
	res := mbactivesync.ItemResult{HeadID: in.HeadID.String()}
	return w.mbasItemTx(ctx, &res, func(tx *sql.Tx) (bool, error) {
		return w.generateRootSpinTx(ctx, tx, in, &res)
	})
}

func (w *MBActiveSyncWriter) generateRootSpinTx(ctx context.Context, tx *sql.Tx, in mbactivesync.GenerateRootInput, res *mbactivesync.ItemResult) (bool, error) {
	head, err := mbasLockHeadEntityOn(ctx, tx, in.HeadID)
	if errors.Is(err, mbhead.ErrNotFound) {
		return mbasVerdict(res, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "head missing or deleted")
	}
	if err != nil {
		return false, fmt.Errorf("lock head: %w", err)
	}
	if head.SupersededByHeadID() != nil {
		return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonHeadSuperseded, "head is superseded")
	}
	root, anySpin, err := mbasHeadSpins(ctx, tx, in.HeadID)
	if err != nil {
		return false, err
	}
	before := mbasGenImage{HeadID: in.HeadID, CostProductID: head.CostProductID()}
	if root != uuid.Nil {
		before.RootSpinID = &root
	}
	res.BeforeJSON = mbasJSON(before)
	if root != uuid.Nil {
		res.SpinID = root.String()
		return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyInTarget, "head already has a live root spin")
	}
	if anySpin {
		return mbasVerdict(res, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "head has spins but no live root spin")
	}
	if head.CostProductID() == 0 {
		return mbasVerdict(res, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan,
			"head has no MB cost product; it must be validated (HeadWorkflow.EnsureValidated)")
	}

	orion, err := mbAutoGenSpinOrionCode(ctx, tx, head)
	if err != nil {
		return false, err
	}
	spin, err := mbBuildAutoGenSpin(in.HeadID, head, mbEmptyStringToPtr(orion), head.CostProductID(), in.Actor)
	if err != nil {
		return false, err
	}
	if err := createSpinOn(ctx, tx, spin); err != nil {
		// createSpinOn already maps uix_mbs_shade_fg_live to ErrSpinShadeFgExists.
		if errors.Is(err, mbspin.ErrSpinShadeFgExists) || isSpinShadeFgViolation(err) {
			return mbasVerdict(res, mbactivesync.ResultFailed, mbactivesync.ReasonFGConflict,
				"a live spin with the same shade and FG already exists (unique shade+FG)")
		}
		return false, fmt.Errorf("insert root spin: %w", err)
	}
	after, _, err := readMBASImage(ctx, tx, spin.ID(), false)
	if err != nil {
		return false, err
	}
	res.SpinID = spin.ID().String()
	res.AfterJSON = after.json()
	res.Result, res.Reason = mbactivesync.ResultApplied, ""
	return true, nil
}
