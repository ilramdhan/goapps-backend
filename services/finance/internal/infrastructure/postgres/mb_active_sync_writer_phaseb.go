package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// Phase B/C/D writer items (design part 2 §5.6, §5.6a, §5.7): supersede between two
// EXISTING spins, head-only supersede (Q15) and guarded deactivation. Each item is one
// transaction; any SKIPPED/FAILED verdict rolls the whole item back.

// mbasForUpdate is the row-lock suffix of the writer's state reads.
const mbasForUpdate = " FOR UPDATE"

// mbasItemTx runs fn in one transaction. fn returns commit=true to commit; otherwise
// the transaction is rolled back and *res is returned as is (skip/fail verdict).
func (w *MBActiveSyncWriter) mbasItemTx(ctx context.Context, res *mbactivesync.ItemResult,
	fn func(tx *sql.Tx) (bool, error),
) (out mbactivesync.ItemResult, err error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return *res, fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback: %w", rbErr)
		}
	}()
	commit, err := fn(tx)
	if err != nil || !commit {
		return *res, err
	}
	if err := tx.Commit(); err != nil {
		return *res, fmt.Errorf("commit: %w", err)
	}
	committed = true
	return *res, nil
}

// mbasVerdict sets a skip/fail verdict on res; the item is rolled back.
func mbasVerdict(res *mbactivesync.ItemResult, status, reason, msg string) (bool, error) {
	res.Result, res.Reason, res.Message = status, reason, msg
	return false, nil
}

func mbasJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// Referrer guard (same rule as the supersede repoint and the planner has_refs)
// ---------------------------------------------------------------------------

// mbasSpinRefs counts the MB_SP_CODE rows referencing the spin: pointer rows (all
// filled_by) + pointer-less rows on its text key, INCLUDING text rows that are
// ambiguous because the ORION code is shared (planner rule: conservative).
func mbasSpinRefs(ctx context.Context, tx *sql.Tx, spinID uuid.UUID, orion *string) (int, error) {
	rows, err := listSupersedeParamRowsOn(ctx, tx, supersedeSpin{SpinID: spinID, OrionItemCode: orion}, false)
	if err != nil {
		return 0, err
	}
	return len(rows.Pointer) + len(rows.TextOnly) + rows.SharedTextRows, nil
}

// ---------------------------------------------------------------------------
// SupersedeExisting (§5.6)
// ---------------------------------------------------------------------------

// mbasSupersedeImage is the before/after image of a SUPERSEDE item.
type mbasSupersedeImage struct {
	OldSpinID           uuid.UUID  `json:"old_spin_id"`
	OldSpinActive       bool       `json:"old_spin_is_active"`
	OldSpinSupersededBy *uuid.UUID `json:"old_spin_superseded_by_spin_id"`
	OldHeadID           uuid.UUID  `json:"old_head_id"`
	OldHeadActive       bool       `json:"old_head_is_active"`
	OldHeadSupersededBy *uuid.UUID `json:"old_head_superseded_by_head_id"`
	NewSpinID           uuid.UUID  `json:"new_spin_id"`
	NewHeadID           uuid.UUID  `json:"new_head_id"`
	// The new spin is an existing, already-live spin (phase A target or phase D
	// reactivation), not a fresh Validate spin. Un-supersede (swapSpinsTx) sets it
	// is_active=false + superseded_by=old, so its prior state is recorded here to
	// let the operator / a revert restore it after Un-supersede.
	NewSpinActive       bool       `json:"new_spin_is_active"`
	NewSpinSupersededBy *uuid.UUID `json:"new_spin_superseded_by_spin_id"`
	NewHeadActive       bool       `json:"new_head_is_active"`

	// after only
	BatchID               *uuid.UUID  `json:"batch_id,omitempty"`
	RepointedRows         int         `json:"repointed_rows,omitempty"`
	AffectedProductIDs    []int64     `json:"affected_product_ids,omitempty"`
	DeactivatedHeadIDs    []uuid.UUID `json:"deactivated_head_ids,omitempty"`
	DeactivatedProductIDs []int64     `json:"deactivated_product_ids,omitempty"`
	Warnings              []string    `json:"warnings,omitempty"`
}

const mbasOldSpinStateQuery = `
	SELECT s.mbs_is_active, s.mbs_superseded_by_spin_id, s.mbs_mbh_id,
	       h.mbh_is_active, h.mbh_superseded_by_head_id
	  FROM mst_mb_spin s JOIN mst_mb_head h ON h.mbh_id = s.mbs_mbh_id
	 WHERE s.mbs_id = $1 AND s.deleted_at IS NULL`

const mbasNewSpinStateQuery = `
	SELECT s.mbs_is_active, s.mbs_superseded_by_spin_id, h.mbh_is_active
	  FROM mst_mb_spin s JOIN mst_mb_head h ON h.mbh_id = s.mbs_mbh_id
	 WHERE s.mbs_id = $1`

// mbasUnsupersedeRollbackWarning is surfaced on every applied SUPERSEDE item: the
// only rollback (existing Un-supersede) also deactivates the pre-existing new spin.
func mbasUnsupersedeRollbackWarning(newSpinID uuid.UUID) string {
	return fmt.Sprintf("ROLLBACK NOTE: Un-supersede of this batch deactivates new spin %s "+
		"(is_active=false, superseded_by=old); reactivate it afterwards from before_json "+
		"(new_spin_is_active / new_spin_superseded_by_spin_id) without touching the old spin", newSpinID)
}

func readMBASSupersedeImage(ctx context.Context, tx *sql.Tx, oldSpinID, newSpinID, newHeadID uuid.UUID) (*mbasSupersedeImage, bool, error) {
	img := &mbasSupersedeImage{OldSpinID: oldSpinID, NewSpinID: newSpinID, NewHeadID: newHeadID}
	var spinBy, headBy uuid.NullUUID
	err := tx.QueryRowContext(ctx, mbasOldSpinStateQuery, oldSpinID).Scan(
		&img.OldSpinActive, &spinBy, &img.OldHeadID, &img.OldHeadActive, &headBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read old spin state: %w", err)
	}
	img.OldSpinSupersededBy = nullUUIDPtr(spinBy)
	img.OldHeadSupersededBy = nullUUIDPtr(headBy)
	var newBy uuid.NullUUID
	err = tx.QueryRowContext(ctx, mbasNewSpinStateQuery, newSpinID).Scan(&img.NewSpinActive, &newBy, &img.NewHeadActive)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("read new spin state: %w", err)
	}
	img.NewSpinSupersededBy = nullUUIDPtr(newBy)
	return img, true, nil
}

// SupersedeExisting implements mbactivesync.Writer (design part 2 §5.6): the existing
// supersede step helpers run in the same order as supersedeOnAutoGenTx minus the
// spin INSERT, so Un-supersede (batch + OLD workflow log) reverses it unchanged.
func (w *MBActiveSyncWriter) SupersedeExisting(ctx context.Context, in mbactivesync.SupersedeInput) (mbactivesync.ItemResult, error) {
	res := mbactivesync.ItemResult{SpinID: in.OldSpinID.String(), HeadID: in.OldHeadID.String()}
	return w.mbasItemTx(ctx, &res, func(tx *sql.Tx) (bool, error) {
		return w.supersedeExistingTx(ctx, tx, in, &res)
	})
}

func (w *MBActiveSyncWriter) supersedeExistingTx(ctx context.Context, tx *sql.Tx, in mbactivesync.SupersedeInput, res *mbactivesync.ItemResult) (bool, error) {
	newImg, ok, err := mbasLockNewSpin(ctx, tx, in.NewSpinID, res)
	if err != nil || !ok {
		return false, err
	}
	cands, err := findSupersedeCandidatesBySpinOn(ctx, tx, in.OldSpinID)
	if err != nil {
		return false, err
	}
	before, found, err := readMBASSupersedeImage(ctx, tx, in.OldSpinID, in.NewSpinID, newImg.Spin.headID)
	if err != nil {
		return false, err
	}
	if !found {
		return mbasVerdict(res, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "old spin missing or deleted")
	}
	res.HeadID = before.OldHeadID.String()
	res.BeforeJSON = mbasJSON(before)
	if len(cands) == 0 {
		return mbasOldSpinNotLive(res, before, in.NewSpinID)
	}
	if st, reason, msg := mbasSupersedeBlocked(in, cands[0], newImg); reason != "" {
		return mbasVerdict(res, st, reason, msg)
	}
	out, err := mbasRunSupersedeSteps(ctx, tx, cands, newImg.Spin.headID, in.NewSpinID, in.Actor)
	if err != nil {
		return false, err
	}
	after, _, err := readMBASSupersedeImage(ctx, tx, in.OldSpinID, in.NewSpinID, newImg.Spin.headID)
	if err != nil {
		return false, err
	}
	after.BatchID = &out.BatchID
	after.RepointedRows = out.RepointedRows
	after.AffectedProductIDs = out.AffectedProductIDs
	after.DeactivatedHeadIDs = out.DeactivatedHeadIDs
	after.DeactivatedProductIDs = out.DeactivatedProductIDs
	after.Warnings = append(append([]string(nil), out.Warnings...), mbasUnsupersedeRollbackWarning(in.NewSpinID))
	res.Result, res.Reason = mbactivesync.ResultApplied, ""
	res.AfterJSON = mbasJSON(after)
	res.Message = fmt.Sprintf("batch %s: %d param row(s) repointed, %d product(s) affected, head deactivated=%t",
		out.BatchID, out.RepointedRows, len(out.AffectedProductIDs), len(out.DeactivatedHeadIDs) > 0)
	if len(out.Warnings) > 0 {
		res.Message += "; " + strings.Join(out.Warnings, "; ")
	}
	res.Message += "; " + mbasUnsupersedeRollbackWarning(in.NewSpinID)
	return true, nil
}

// mbasLockNewSpin locks the superseding spin (+ its head) and checks it is ready:
// live, on a live head, with a shade and an FG (its row ran in an earlier phase).
func mbasLockNewSpin(ctx context.Context, tx *sql.Tx, newSpinID uuid.UUID, res *mbactivesync.ItemResult) (*mbasImage, bool, error) {
	notReady := func(msg string) (*mbasImage, bool, error) {
		_, err := mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonNewSpinNotReady, msg)
		return nil, false, err
	}
	if newSpinID == uuid.Nil {
		return notReady("new spin not created yet")
	}
	img, found, err := readMBASImage(ctx, tx, newSpinID, true)
	if err != nil {
		return nil, false, err
	}
	switch {
	case !found || img.Head.deleted:
		return notReady("new spin or its head missing/deleted")
	case !img.Spin.IsActive || img.Spin.superseded:
		return notReady("new spin is not live")
	case !img.Head.IsActive || img.Head.superseded:
		return notReady("new spin's head is not live")
	case strings.TrimSpace(img.Spin.ShadeCode) == "" || img.Spin.FGProductID == nil:
		return notReady("new spin has no shade or FG yet")
	}
	return img, true, nil
}

// mbasOldSpinNotLive: K5 found no live candidate. Superseded by this new spin → done;
// otherwise the plan is stale (deactivated / superseded elsewhere since the plan).
func mbasOldSpinNotLive(res *mbactivesync.ItemResult, before *mbasSupersedeImage, newSpinID uuid.UUID) (bool, error) {
	if before.OldSpinSupersededBy != nil && *before.OldSpinSupersededBy == newSpinID {
		return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyInTarget, "old spin already superseded by the new spin")
	}
	if before.OldSpinSupersededBy != nil {
		return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonStalePlan, "old spin superseded by another spin")
	}
	return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonStalePlan, "old spin is no longer active")
}

// mbasSupersedeBlocked re-checks the plan pair against the locked rows.
func mbasSupersedeBlocked(in mbactivesync.SupersedeInput, old supersedeSpin, newImg *mbasImage) (status, reason, msg string) {
	switch {
	case in.OldHeadID != uuid.Nil && old.HeadID != in.OldHeadID:
		return mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "old spin moved to another head"
	case old.HeadID == newImg.Spin.headID:
		return mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "old and new spin share the same head"
	case mbactivesync.NormShadeLoose(old.ShadeCode) != "" &&
		mbactivesync.NormShadeLoose(old.ShadeCode) != mbactivesync.NormShadeLoose(newImg.Spin.ShadeCode):
		return mbactivesync.ResultFailed, mbactivesync.ReasonShadeMismatch, "old and new spin shades differ beyond format"
	}
	return "", "", ""
}

// mbasRunSupersedeSteps calls the existing supersede step helpers in the order of
// supersedeOnAutoGenTx, without createSpinOn (S_new already exists).
func mbasRunSupersedeSteps(ctx context.Context, tx *sql.Tx, cands []supersedeSpin, newHeadID, newSpinID uuid.UUID, actor string) (*mbhead.SupersedeOutcome, error) {
	out := &mbhead.SupersedeOutcome{BatchID: uuid.New(), NewSpinID: newSpinID}
	oldIDs := make([]string, 0, len(cands))
	for _, c := range cands {
		out.OldSpinIDs = append(out.OldSpinIDs, c.SpinID)
		oldIDs = append(oldIDs, c.SpinID.String())
	}
	if err := supersedeDeactivateOldSpinsTx(ctx, tx, oldIDs, actor); err != nil {
		return nil, err
	}
	if err := supersedeLinkOldSpinsTx(ctx, tx, oldIDs, newSpinID); err != nil {
		return nil, err
	}
	if err := supersedeRepointAllTx(ctx, tx, out, cands, actor); err != nil {
		return nil, err
	}
	if err := supersedeDeactivateOldHeadsTx(ctx, tx, out, cands, newHeadID, oldIDs, actor); err != nil {
		return nil, err
	}
	if err := supersedeDeactivateOldProductsTx(ctx, tx, out, cands, oldIDs, actor); err != nil {
		return nil, err
	}
	if err := supersedeWriteWorkflowLogsTx(ctx, tx, out, cands, newHeadID, actor); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// SupersedeHeadOnly (§5.6a, Q15)
// ---------------------------------------------------------------------------

// mbasHeadSupersedeImage is the before/after image of a SUPERSEDE_HEAD item.
type mbasHeadSupersedeImage struct {
	IsActive        bool       `json:"mbh_is_active"`
	SupersededBy    *uuid.UUID `json:"mbh_superseded_by_head_id"`
	SupersededAt    *time.Time `json:"mbh_superseded_at"`
	SupersededActor *string    `json:"mbh_superseded_by"`

	deleted bool
}

const mbasHeadSupersedeQuery = `
	SELECT mbh_is_active, mbh_superseded_by_head_id, mbh_superseded_at, mbh_superseded_by, deleted_at IS NOT NULL
	  FROM mst_mb_head WHERE mbh_id = $1`

func readMBASHeadSupersede(ctx context.Context, tx *sql.Tx, headID uuid.UUID, lock bool) (*mbasHeadSupersedeImage, bool, error) {
	q := mbasHeadSupersedeQuery
	if lock {
		q += mbasForUpdate
	}
	var (
		img mbasHeadSupersedeImage
		by  uuid.NullUUID
		at  sql.NullTime
		who sql.NullString
	)
	err := tx.QueryRowContext(ctx, q, headID).Scan(&img.IsActive, &by, &at, &who, &img.deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read head supersede state: %w", err)
	}
	img.SupersededBy = nullUUIDPtr(by)
	if at.Valid {
		v := at.Time
		img.SupersededAt = &v
	}
	img.SupersededActor = nullableStringPtr(who)
	return &img, true, nil
}

// mbasHeadOnlyLogMeta is the mbwl_meta of the OLD-head SUPERSEDE row of a head-only
// supersede: no batch, no spins (new_spin_id is null), head_deactivated=true.
type mbasHeadOnlyLogMeta struct {
	Event              string      `json:"event"`
	Role               string      `json:"role"`
	Kind               string      `json:"kind"`
	NewHeadID          uuid.UUID   `json:"new_head_id"`
	NewSpinID          *uuid.UUID  `json:"new_spin_id"`
	OldSpinIDs         []uuid.UUID `json:"old_spin_ids"`
	HeadDeactivated    bool        `json:"head_deactivated"`
	DeactivatedHeadIDs []uuid.UUID `json:"deactivated_head_ids"`
}

// SupersedeHeadOnly implements mbactivesync.Writer (design part 2 §5.6a, Q15).
func (w *MBActiveSyncWriter) SupersedeHeadOnly(ctx context.Context, in mbactivesync.SupersedeHeadInput) (mbactivesync.ItemResult, error) {
	res := mbactivesync.ItemResult{HeadID: in.OldHeadID.String()}
	return w.mbasItemTx(ctx, &res, func(tx *sql.Tx) (bool, error) {
		return w.supersedeHeadOnlyTx(ctx, tx, in, &res)
	})
}

func (w *MBActiveSyncWriter) supersedeHeadOnlyTx(ctx context.Context, tx *sql.Tx, in mbactivesync.SupersedeHeadInput, res *mbactivesync.ItemResult) (bool, error) {
	before, found, err := readMBASHeadSupersede(ctx, tx, in.OldHeadID, true)
	if err != nil {
		return false, err
	}
	if !found || before.deleted {
		return mbasVerdict(res, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "old head missing or deleted")
	}
	res.BeforeJSON = mbasJSON(before)
	newHead, found, err := readMBASHeadSupersede(ctx, tx, in.NewHeadID, true)
	if err != nil {
		return false, err
	}
	if st, reason, msg := mbasHeadOnlyBlocked(in, before, newHead, found); reason != "" {
		return mbasVerdict(res, st, reason, msg)
	}
	live, err := mbasLiveSpinsOfHead(ctx, tx, in.OldHeadID)
	if err != nil {
		return false, err
	}
	if len(live) > 0 {
		return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonStalePlan,
			fmt.Sprintf("old head has %d live spin(s) now: re-plan as spin-level SUPERSEDE", len(live)))
	}
	// Referrers are summed over live spins (planner/§5.7 rule) — 0 here by construction;
	// mbasLiveSpinsOfHead is the same recount the deactivate guard uses.
	if _, err := tx.ExecContext(ctx, `
		UPDATE mst_mb_head
		   SET mbh_is_active = FALSE, mbh_superseded_by_head_id = $2, mbh_superseded_at = NOW(),
		       mbh_superseded_by = $3, updated_at = NOW(), updated_by = $3
		 WHERE mbh_id = $1 AND deleted_at IS NULL`, in.OldHeadID, in.NewHeadID, in.Actor); err != nil {
		return false, fmt.Errorf("supersede head: %w", err)
	}
	meta := mbasHeadOnlyLogMeta{
		Event: supersedeWorkflowLogToState, Role: "OLD", Kind: string(mbactivesync.KindSupersedeHead),
		NewHeadID: in.NewHeadID, OldSpinIDs: []uuid.UUID{}, HeadDeactivated: true,
		DeactivatedHeadIDs: []uuid.UUID{in.OldHeadID},
	}
	if err := insertSupersedeEventLogTx(ctx, tx, in.OldHeadID, in.Actor,
		"SUPERSEDE: recipe superseded by a newer recipe of the same family and shade (no live spin)",
		supersedeWorkflowLogToState, meta); err != nil {
		return false, err
	}
	after, _, err := readMBASHeadSupersede(ctx, tx, in.OldHeadID, false)
	if err != nil {
		return false, err
	}
	res.Result, res.Reason = mbactivesync.ResultApplied, ""
	res.AfterJSON = mbasJSON(after)
	return true, nil
}

func mbasHeadOnlyBlocked(in mbactivesync.SupersedeHeadInput, old, newHead *mbasHeadSupersedeImage, newFound bool) (status, reason, msg string) {
	switch {
	case in.OldHeadID == in.NewHeadID:
		return mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "old and new head are the same"
	case old.SupersededBy != nil && *old.SupersededBy == in.NewHeadID:
		return mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyInTarget, "old head already superseded by the new head"
	case old.SupersededBy != nil:
		return mbactivesync.ResultSkipped, mbactivesync.ReasonStalePlan, "old head superseded by another head"
	case !newFound || newHead.deleted || !newHead.IsActive || newHead.SupersededBy != nil:
		return mbactivesync.ResultSkipped, mbactivesync.ReasonNewHeadNotReady, "new head is not live (activating row not applied yet?)"
	}
	return "", "", ""
}

// ---------------------------------------------------------------------------
// DeactivateSpin / DeactivateHead (§5.7)
// ---------------------------------------------------------------------------

// mbasLiveSpin is one live spin of a head (locked) with its text key source.
type mbasLiveSpin struct {
	ID    uuid.UUID `json:"id"`
	Orion *string   `json:"-"`
	// IsActive is the state before deactivation (always true for a live spin).
	IsActive bool `json:"is_active"`
}

// mbasLiveSpinsOfHead locks (FOR UPDATE) the live (active, not superseded, not deleted)
// spins of the head.
func mbasLiveSpinsOfHead(ctx context.Context, tx *sql.Tx, headID uuid.UUID) ([]mbasLiveSpin, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT mbs_id, mbs_orion_item_code FROM mst_mb_spin
		 WHERE mbs_mbh_id = $1 AND deleted_at IS NULL AND mbs_is_active AND mbs_superseded_by_spin_id IS NULL
		 ORDER BY created_at, mbs_id
		   FOR UPDATE`, headID)
	if err != nil {
		return nil, fmt.Errorf("list live spins of head: %w", err)
	}
	defer closeRows(rows)
	var out []mbasLiveSpin
	for rows.Next() {
		var (
			s     mbasLiveSpin
			orion sql.NullString
		)
		if err := rows.Scan(&s.ID, &orion); err != nil {
			return nil, fmt.Errorf("scan live spin: %w", err)
		}
		s.Orion = nullableStringPtr(orion)
		s.IsActive = true
		out = append(out, s)
	}
	return out, rows.Err()
}

// mbasDeactivateSpinOn sets is_active=false through the entity and the existing updateOn.
func (w *MBActiveSyncWriter) mbasDeactivateSpinOn(ctx context.Context, tx *sql.Tx, spinID uuid.UUID, actor string) error {
	entity, err := loadSpinForRepointOn(ctx, tx, spinID)
	if err != nil {
		return err
	}
	inactive := false
	if err := entity.Update(mbspin.UpdateInput{IsActive: &inactive}, actor); err != nil {
		return fmt.Errorf("deactivate spin: %w", err)
	}
	return w.spin.updateOn(ctx, tx, entity)
}

// DeactivateSpin implements mbactivesync.Writer (design part 2 §5.7).
func (w *MBActiveSyncWriter) DeactivateSpin(ctx context.Context, in mbactivesync.DeactivateSpinInput) (mbactivesync.ItemResult, error) {
	res := mbactivesync.ItemResult{SpinID: in.SpinID.String()}
	return w.mbasItemTx(ctx, &res, func(tx *sql.Tx) (bool, error) {
		return w.deactivateSpinTx(ctx, tx, in, &res)
	})
}

func (w *MBActiveSyncWriter) deactivateSpinTx(ctx context.Context, tx *sql.Tx, in mbactivesync.DeactivateSpinInput, res *mbactivesync.ItemResult) (bool, error) {
	before, found, err := readMBASImage(ctx, tx, in.SpinID, true)
	if err != nil {
		return false, err
	}
	if !found {
		return mbasVerdict(res, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "spin missing or deleted")
	}
	res.HeadID = before.Spin.headID.String()
	res.BeforeJSON = before.json()
	if !before.Spin.IsActive || before.Spin.superseded {
		return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyInTarget, "spin already inactive")
	}
	var orion sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT mbs_orion_item_code FROM mst_mb_spin WHERE mbs_id = $1`, in.SpinID).Scan(&orion); err != nil {
		return false, fmt.Errorf("read spin orion code: %w", err)
	}
	refs, err := mbasSpinRefs(ctx, tx, in.SpinID, nullableStringPtr(orion))
	if err != nil {
		return false, err
	}
	if refs > 0 {
		return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonHasRefs,
			fmt.Sprintf("%d MB_SP_CODE row(s) still reference the spin", refs))
	}
	if err := w.mbasDeactivateSpinOn(ctx, tx, in.SpinID, in.Actor); err != nil {
		if errors.Is(err, mbspin.ErrNotFound) {
			return mbasVerdict(res, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "spin vanished")
		}
		return false, err
	}
	after, _, err := readMBASImage(ctx, tx, in.SpinID, false)
	if err != nil {
		return false, err
	}
	res.Result, res.Reason = mbactivesync.ResultApplied, ""
	res.AfterJSON = after.json()
	return true, nil
}

// mbasDeactHeadImage is the before/after image of DEACTIVATE_HEAD: the head plus
// EVERY spin the item deactivated (revert restores exactly those).
type mbasDeactHeadImage struct {
	Head  mbasDeactHead  `json:"head"`
	Spins []mbasLiveSpin `json:"spins"`
}

type mbasDeactHead struct {
	IsActive    bool   `json:"mbh_is_active"`
	FGProductID *int64 `json:"mbh_fg_product_id"`
	FGItemCode  string `json:"mbh_fg_item_code"`

	deleted    bool
	superseded bool
}

func readMBASDeactHead(ctx context.Context, tx *sql.Tx, headID uuid.UUID, lock bool) (*mbasDeactHead, bool, error) {
	q := `SELECT mbh_is_active, mbh_fg_product_id, COALESCE(mbh_fg_item_code, ''),
	             deleted_at IS NOT NULL, mbh_superseded_by_head_id IS NOT NULL
	        FROM mst_mb_head WHERE mbh_id = $1`
	if lock {
		q += mbasForUpdate
	}
	var (
		h  mbasDeactHead
		fg sql.NullInt64
	)
	err := tx.QueryRowContext(ctx, q, headID).Scan(&h.IsActive, &fg, &h.FGItemCode, &h.deleted, &h.superseded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read head: %w", err)
	}
	h.FGProductID = nullInt64Ptr(fg)
	return &h, true, nil
}

// DeactivateHead implements mbactivesync.Writer (design part 2 §5.7): head
// is_active=false WITHOUT cascade (bypasses ErrHeadLocked, Q13) plus its live spins in
// the same tx, guarded by 0 referrers on those spins. The head's MB cost product is
// NOT deactivated (conservative, reported).
func (w *MBActiveSyncWriter) DeactivateHead(ctx context.Context, in mbactivesync.DeactivateHeadInput) (mbactivesync.ItemResult, error) {
	res := mbactivesync.ItemResult{HeadID: in.HeadID.String()}
	return w.mbasItemTx(ctx, &res, func(tx *sql.Tx) (bool, error) {
		return w.deactivateHeadTx(ctx, tx, in, &res)
	})
}

func (w *MBActiveSyncWriter) deactivateHeadTx(ctx context.Context, tx *sql.Tx, in mbactivesync.DeactivateHeadInput, res *mbactivesync.ItemResult) (bool, error) {
	head, found, err := readMBASDeactHead(ctx, tx, in.HeadID, true)
	if err != nil {
		return false, err
	}
	if !found || head.deleted {
		return mbasVerdict(res, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "head missing or deleted")
	}
	if head.superseded {
		return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonHeadSuperseded, "head is superseded")
	}
	live, ok, err := mbasDeactHeadPrecheck(ctx, tx, head, in.HeadID, res)
	if err != nil || !ok {
		return false, err
	}
	for _, s := range live {
		if err := w.mbasDeactivateSpinOn(ctx, tx, s.ID, in.Actor); err != nil {
			return false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE mst_mb_head SET mbh_is_active = FALSE, updated_at = NOW(), updated_by = $2
		 WHERE mbh_id = $1 AND deleted_at IS NULL`, in.HeadID, in.Actor); err != nil {
		return false, fmt.Errorf("deactivate head: %w", err)
	}
	afterHead, _, err := readMBASDeactHead(ctx, tx, in.HeadID, false)
	if err != nil {
		return false, err
	}
	after := mbasDeactHeadImage{Head: *afterHead, Spins: make([]mbasLiveSpin, 0, len(live))}
	for _, s := range live {
		after.Spins = append(after.Spins, mbasLiveSpin{ID: s.ID, IsActive: false})
	}
	res.Result, res.Reason = mbactivesync.ResultApplied, ""
	res.AfterJSON = mbasJSON(after)
	res.Message = fmt.Sprintf("head and %d live spin(s) deactivated; MB cost product left active", len(live))
	return true, nil
}

// mbasDeactHeadPrecheck locks the live spins, records before_json and applies the
// already_in_target / has_refs guards; ok=false ends the item without writing.
func mbasDeactHeadPrecheck(ctx context.Context, tx *sql.Tx, head *mbasDeactHead, headID uuid.UUID, res *mbactivesync.ItemResult) ([]mbasLiveSpin, bool, error) {
	live, err := mbasLiveSpinsOfHead(ctx, tx, headID)
	if err != nil {
		return nil, false, err
	}
	before := mbasDeactHeadImage{Head: *head, Spins: live}
	if before.Spins == nil {
		before.Spins = []mbasLiveSpin{}
	}
	res.BeforeJSON = mbasJSON(before)
	if !head.IsActive && len(live) == 0 {
		_, err := mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyInTarget, "head already inactive")
		return nil, false, err
	}
	n, err := mbasLiveSpinRefs(ctx, tx, live)
	if err != nil {
		return nil, false, err
	}
	if n > 0 {
		_, err := mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonHasRefs,
			fmt.Sprintf("%d MB_SP_CODE row(s) still reference live spins of the head", n))
		return nil, false, err
	}
	return live, true, nil
}

func mbasLiveSpinRefs(ctx context.Context, tx *sql.Tx, live []mbasLiveSpin) (int, error) {
	total := 0
	for _, s := range live {
		n, err := mbasSpinRefs(ctx, tx, s.ID, s.Orion)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}
