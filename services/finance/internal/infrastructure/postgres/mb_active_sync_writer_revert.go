package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// Revert (design part 2 §8): restore the before-image of ONE applied item, optimistic and
// in one transaction. Only the column GROUPS the item changed (before != after) are
// compared and written: a group still holding the after-image is restored, a group
// already holding the before-image is left, anything else is a later change →
// SKIPPED changed_since_apply (nothing of the item is written).
//
// Per action: row actions and DEACTIVATE_SPIN restore the spin/head image;
// DEACTIVATE_HEAD restores the head and exactly the spins it deactivated;
// SUPERSEDE_HEAD restores the old head; clones and spins created by GENERATE_SPIN
// are deactivated (never deleted); SUPERSEDE is not reverted (use Un-supersede).

// Revert implements mbactivesync.Writer.
func (w *MBActiveSyncWriter) Revert(ctx context.Context, in mbactivesync.RevertInput) (mbactivesync.ItemResult, error) {
	res := mbactivesync.ItemResult{}
	if in.SpinID != uuid.Nil {
		res.SpinID = in.SpinID.String()
	}
	if in.CloneSpinID != uuid.Nil {
		res.CloneSpinID = in.CloneSpinID.String()
	}
	if in.HeadID != uuid.Nil {
		res.HeadID = in.HeadID.String()
	}
	notWired := func(reason, msg string) (mbactivesync.ItemResult, error) {
		res.Result, res.Reason, res.Message = mbactivesync.ResultSkipped, reason, msg
		if reason == mbactivesync.ReasonNotRevertable {
			res.ManualReason = reason
		}
		return res, nil
	}
	kind := mbactivesync.HeadKind(in.Action)
	switch kind {
	case mbactivesync.KindSupersede:
		return notWired(mbactivesync.ReasonNotRevertable, "SUPERSEDE is not reverted by the tool: use Un-supersede (UI/RPC) and reactivate the new spin from before_json")
	case mbactivesync.KindSupersedeHead:
		return w.mbasItemTx(ctx, &res, func(tx *sql.Tx) (bool, error) { return mbasRevertSupersedeHead(ctx, tx, in, &res) })
	case mbactivesync.KindDeactivateHead:
		return w.mbasItemTx(ctx, &res, func(tx *sql.Tx) (bool, error) { return mbasRevertDeactivateHead(ctx, tx, in, &res) })
	case mbactivesync.KindDeactivateSpin:
		return w.mbasItemTx(ctx, &res, func(tx *sql.Tx) (bool, error) { return w.mbasRevertSpinImage(ctx, tx, in, in.SpinID, &res) })
	case mbactivesync.KindReportHead, mbactivesync.KindReportSpin:
		return notWired(mbactivesync.ReasonNotRevertable, "report items change nothing")
	}
	switch mbactivesync.Action(in.Action) {
	case mbactivesync.ActionKeep, mbactivesync.ActionUpdateLDR, mbactivesync.ActionUpdateShade, mbactivesync.ActionReactivate,
		mbactivesync.ActionDuplicateSpin, mbactivesync.ActionGenerateSpin:
	default:
		return notWired(mbactivesync.ReasonNotRevertable, fmt.Sprintf("action %q is not revertable", in.Action))
	}
	switch {
	case in.CloneSpinID != uuid.Nil:
		return w.mbasItemTx(ctx, &res, func(tx *sql.Tx) (bool, error) {
			return w.mbasRevertCreatedSpin(ctx, tx, in.CloneSpinID, in.Actor, mbasSyncHeads(in.BeforeJSON, in.AfterJSON), &res)
		})
	case mbactivesync.Action(in.Action) == mbactivesync.ActionGenerateSpin:
		created, sb, sa := mbasUnwrapRoot(in.BeforeJSON, in.AfterJSON)
		if created {
			heads := mbasSyncHeads(sb, sa)
			return w.mbasItemTx(ctx, &res, func(tx *sql.Tx) (bool, error) {
				return w.mbasRevertCreatedSpin(ctx, tx, in.SpinID, in.Actor, heads, &res)
			})
		}
		in.BeforeJSON, in.AfterJSON = sb, sa
	}
	return w.mbasItemTx(ctx, &res, func(tx *sql.Tx) (bool, error) { return w.mbasRevertSpinImage(ctx, tx, in, in.SpinID, &res) })
}

// mbasUnwrapRoot inspects the images of a GENERATE_SPIN ROOT row. The applier nests them
// (combineJSON) as {generate|ensure: <generation image>, sync: <spin image>}. created is
// true when the generation step created the root spin (its before-image had none); sync
// returns the plain spin before/after images (empty when the row was not nested).
func mbasUnwrapRoot(before, after string) (created bool, syncBefore, syncAfter string) {
	var bm, am map[string]json.RawMessage
	if json.Unmarshal([]byte(before), &bm) != nil || json.Unmarshal([]byte(after), &am) != nil {
		return false, before, after
	}
	_, hasGen := bm["generate"]
	_, hasEns := bm["ensure"]
	if !hasGen && !hasEns {
		return false, before, after
	}
	gen := bm["ensure"]
	if len(gen) == 0 {
		gen = bm["generate"]
	}
	var g struct {
		Root *uuid.UUID `json:"root_spin_id"`
	}
	if len(gen) > 0 && json.Unmarshal(gen, &g) != nil {
		return false, before, after // unreadable generation image: treat as a plain spin image
	}
	return g.Root == nil, string(bm["sync"]), string(am["sync"])
}

func mbasChanged(res *mbactivesync.ItemResult, msg string) (bool, error) {
	return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonChangedSinceApply, msg)
}

// ---------------------------------------------------------------------------
// Spin / head image (row actions, DEACTIVATE_SPIN)
// ---------------------------------------------------------------------------

func mbasEqF4(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return mbactivesync.Fix4(*a) == mbactivesync.Fix4(*b)
}

func mbasEqI64(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func mbasEqBool(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

type mbasSpinGroups struct{ shade, ldr, active, fg, spec bool }

func mbasEqI(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func mbasSpinEq(a, b mbasSpinImage) mbasSpinGroups { // true = group EQUAL
	return mbasSpinGroups{
		shade:  a.ShadeCode == b.ShadeCode && a.ShadeName == b.ShadeName && a.CC == b.CC && a.CrossSection == b.CrossSection,
		ldr:    mbasEqF4(a.RunLDRPct, b.RunLDRPct) && a.LDRType == b.LDRType && a.LDRIsActual == b.LDRIsActual && mbasEqBool(a.LDRIsFixed, b.LDRIsFixed),
		active: a.IsActive == b.IsActive,
		fg:     mbasEqI64(a.FGProductID, b.FGProductID) && a.FGItemCode == b.FGItemCode && a.FGLinkSource == b.FGLinkSource,
		spec:   mbasSpecEq(a, b),
	}
}

// mbasSpecEq compares denier/filament. Images written before P2h-4c carry no spec
// (HasSpec false): the group then looks unchanged and is never restored.
func mbasSpecEq(a, b mbasSpinImage) bool {
	if !a.HasSpec || !b.HasSpec {
		return true
	}
	return mbasEqF4(a.Denier, b.Denier) && mbasEqI(a.Filament, b.Filament)
}

type mbasHeadGroups struct{ active, fg, shade bool }

func mbasHeadEq(a, b mbasHeadImage) mbasHeadGroups {
	return mbasHeadGroups{
		active: a.IsActive == b.IsActive,
		fg:     mbasEqI64(a.FGProductID, b.FGProductID) && a.FGItemCode == b.FGItemCode,
		shade:  a.ShadeCode == b.ShadeCode && a.ShadeName == b.ShadeName,
	}
}

// mbasGroupState classifies one changed group: -1 = holds the before-image, 1 = holds
// the after-image (to restore), 0 = something else (later change).
func mbasGroupState(eqBefore, eqAfter bool) int {
	switch {
	case eqBefore:
		return -1
	case eqAfter:
		return 1
	}
	return 0
}

func (w *MBActiveSyncWriter) mbasRevertSpinImage(ctx context.Context, tx *sql.Tx, in mbactivesync.RevertInput, spinID uuid.UUID, res *mbactivesync.ItemResult) (bool, error) {
	var before, after mbasImage
	if json.Unmarshal([]byte(in.BeforeJSON), &before) != nil || json.Unmarshal([]byte(in.AfterJSON), &after) != nil || spinID == uuid.Nil {
		return mbasVerdict(res, mbactivesync.ResultFailed, mbactivesync.ReasonBadPlanItem, "before_json/after_json/spin_id of the result row are not a spin image")
	}
	cur, found, err := readMBASImage(ctx, tx, spinID, true)
	if err != nil {
		return false, err
	}
	if !found {
		return mbasChanged(res, "spin or head missing or deleted")
	}
	res.HeadID = cur.Spin.headID.String()
	res.BeforeJSON = cur.json()

	rs, rh, allBefore, bad := mbasClassify(cur, &before, &after)
	if bad {
		return mbasChanged(res, "a column the item changed no longer holds its after-image")
	}
	if allBefore {
		return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyReverted, "")
	}
	if err := mbasRestoreSpin(ctx, tx, spinID, before.Spin, in.Actor, rs); err != nil {
		if isSpinShadeFgViolation(err) || isPGUniqueViolation(err) {
			return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonFGConflict, "restoring shade/FG violates unique (shade, FG)")
		}
		return false, err
	}
	if err := mbasRestoreHead(ctx, tx, cur.Spin.headID, before.Head, in.Actor, rh); err != nil {
		return false, err
	}
	now, _, err := readMBASImage(ctx, tx, spinID, false)
	if err != nil {
		return false, err
	}
	res.Result, res.Reason = mbactivesync.ResultApplied, ""
	res.AfterJSON = now.json()
	return true, nil
}

// mbasGroupAcc folds the per-group states of one revert into restore/allBefore/bad.
type mbasGroupAcc struct{ allBefore, bad bool }

func (a *mbasGroupAcc) add(equalAcross, eqB, eqA bool) (restore bool) {
	if equalAcross {
		return false // group not changed by the item
	}
	switch mbasGroupState(eqB, eqA) {
	case 1:
		a.allBefore = false
		return true
	case 0:
		a.allBefore, a.bad = false, true
	}
	return false
}

// mbasClassifyHead decides which head groups to restore for the current head versus
// the before/after images.
func mbasClassifyHead(cur, before, after mbasHeadImage, acc *mbasGroupAcc) mbasHeadGroups {
	hb, ha, hch := mbasHeadEq(cur, before), mbasHeadEq(cur, after), mbasHeadEq(before, after)
	return mbasHeadGroups{
		active: acc.add(hch.active, hb.active, ha.active),
		fg:     acc.add(hch.fg, hb.fg, ha.fg),
		shade:  acc.add(hch.shade, hb.shade, ha.shade),
	}
}

// mbasClassify decides which spin and head groups to restore (Q13 optimistic check).
func mbasClassify(cur *mbasImage, before, after *mbasImage) (rs mbasSpinGroups, rh mbasHeadGroups, allBefore, bad bool) {
	acc := &mbasGroupAcc{allBefore: true}
	sb, sa, ch := mbasSpinEq(cur.Spin, before.Spin), mbasSpinEq(cur.Spin, after.Spin), mbasSpinEq(before.Spin, after.Spin)
	rs = mbasSpinGroups{
		shade:  acc.add(ch.shade, sb.shade, sa.shade),
		ldr:    acc.add(ch.ldr, sb.ldr, sa.ldr),
		active: acc.add(ch.active, sb.active, sa.active),
		fg:     acc.add(ch.fg, sb.fg, sa.fg),
		spec:   acc.add(ch.spec, sb.spec, sa.spec),
	}
	rh = mbasClassifyHead(cur.Head, before.Head, after.Head, acc)
	return rs, rh, acc.allBefore, acc.bad
}

func mbasRestoreSpin(ctx context.Context, tx *sql.Tx, id uuid.UUID, b mbasSpinImage, actor string, g mbasSpinGroups) error {
	sets := []string{"updated_at = NOW()", "updated_by = $2"}
	args := []any{id, actor}
	next := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if g.shade {
		sets = append(sets,
			"mbs_shade_code = NULLIF("+next(b.ShadeCode)+", '')",
			"mbs_shade_name = NULLIF("+next(b.ShadeName)+", '')",
			"mbs_cc = NULLIF("+next(b.CC)+", '')",
			"mbs_cross_section = NULLIF("+next(b.CrossSection)+", '')")
	}
	if g.ldr {
		sets = append(sets,
			"mbs_run_ldr_pct = "+next(b.RunLDRPct),
			"mbs_ldr_type = COALESCE(NULLIF("+next(b.LDRType)+", ''), 'NOT_CALCULATED')",
			"mbs_ldr_is_actual = "+next(b.LDRIsActual),
			"mbs_ldr_is_fixed = "+next(b.LDRIsFixed))
	}
	if g.spec {
		sets = append(sets, "mbs_denier = "+next(b.Denier), "mbs_filament = "+next(b.Filament))
	}
	if g.active {
		sets = append(sets, "mbs_is_active = "+next(b.IsActive))
	}
	if g.fg {
		sets = append(sets,
			"mbs_fg_product_id = "+next(b.FGProductID),
			"mbs_fg_item_code = NULLIF("+next(b.FGItemCode)+", '')",
			"mbs_fg_link_source = NULLIF("+next(b.FGLinkSource)+", '')")
	}
	if len(sets) == 2 {
		return nil
	}
	//nolint:gosec // SET fragments are constants, values are bound as $n
	q := "UPDATE mst_mb_spin SET " + strings.Join(sets, ", ") + " WHERE mbs_id = $1 AND deleted_at IS NULL"
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("restore spin: %w", err)
	}
	return nil
}

func mbasRestoreHead(ctx context.Context, tx *sql.Tx, id uuid.UUID, b mbasHeadImage, actor string, g mbasHeadGroups) error {
	sets := []string{"updated_at = NOW()", "updated_by = $2"}
	args := []any{id, actor}
	next := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if g.active {
		sets = append(sets, "mbh_is_active = "+next(b.IsActive))
	}
	if g.fg {
		sets = append(sets, "mbh_fg_product_id = "+next(b.FGProductID), "mbh_fg_item_code = NULLIF("+next(b.FGItemCode)+", '')")
	}
	if g.shade {
		sets = append(sets, "mbh_shade_code = NULLIF("+next(b.ShadeCode)+", '')", "mbh_shade_name = NULLIF("+next(b.ShadeName)+", '')")
	}
	if len(sets) == 2 {
		return nil
	}
	//nolint:gosec // SET fragments are constants, values are bound as $n
	q := "UPDATE mst_mb_head SET " + strings.Join(sets, ", ") + " WHERE mbh_id = $1 AND deleted_at IS NULL"
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("restore head: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Clone / generated spin: deactivate (never delete)
// ---------------------------------------------------------------------------

// mbasHeadPair is the head before/after image of the sync step that ran on a created spin.
type mbasHeadPair struct{ before, after mbasHeadImage }

// mbasSyncHeads extracts the head images of a SyncExistingSpin before/after pair (the
// DUPLICATE_SPIN result, or the nested `sync` image of GENERATE_SPIN). Images without a
// head part (not a sync image) yield nil: no head groups to restore.
func mbasSyncHeads(before, after string) *mbasHeadPair {
	var b, a struct {
		Head *mbasHeadImage `json:"head"`
	}
	if json.Unmarshal([]byte(before), &b) != nil || json.Unmarshal([]byte(after), &a) != nil || b.Head == nil || a.Head == nil {
		return nil
	}
	return &mbasHeadPair{before: *b.Head, after: *a.Head}
}

func (w *MBActiveSyncWriter) mbasRevertCreatedSpin(ctx context.Context, tx *sql.Tx, spinID uuid.UUID, actor string, heads *mbasHeadPair, res *mbactivesync.ItemResult) (bool, error) {
	if spinID == uuid.Nil {
		return mbasVerdict(res, mbactivesync.ResultFailed, mbactivesync.ReasonBadPlanItem, "result row has no spin_id/clone_spin_id")
	}
	cur, found, err := readMBASImage(ctx, tx, spinID, true)
	if err != nil {
		return false, err
	}
	if !found {
		return mbasChanged(res, "created spin missing or deleted")
	}
	res.HeadID = cur.Spin.headID.String()
	res.BeforeJSON = cur.json()
	// Head groups first (classification only): a changed head blocks the whole item.
	var rh mbasHeadGroups
	headDone := true
	if heads != nil {
		acc := &mbasGroupAcc{allBefore: true}
		rh = mbasClassifyHead(cur.Head, heads.before, heads.after, acc)
		if acc.bad {
			return mbasChanged(res, "a head column the sync step changed no longer holds its after-image")
		}
		headDone = acc.allBefore
	}
	if !cur.Spin.IsActive && headDone {
		return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyReverted, "created spin already inactive")
	}
	if cur.Spin.IsActive {
		if ok, verdictErr := w.mbasDeactivateCreated(ctx, tx, spinID, actor, res); !ok {
			return false, verdictErr
		}
	}
	if heads != nil && !headDone {
		if err := mbasRestoreHead(ctx, tx, cur.Spin.headID, heads.before, actor, rh); err != nil {
			return false, err
		}
	}
	now, _, err := readMBASImage(ctx, tx, spinID, false)
	if err != nil {
		return false, err
	}
	res.Result, res.Reason = mbactivesync.ResultApplied, ""
	res.AfterJSON = now.json()
	res.Message = "created spin deactivated (not deleted)"
	return true, nil
}

// mbasDeactivateCreated deactivates the created spin unless MB_SP_CODE rows reference it.
// ok=false means a verdict was recorded on res (the bool/error pair is the item result).
func (w *MBActiveSyncWriter) mbasDeactivateCreated(ctx context.Context, tx *sql.Tx, spinID uuid.UUID, actor string, res *mbactivesync.ItemResult) (ok bool, verdict error) {
	var orion sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT mbs_orion_item_code FROM mst_mb_spin WHERE mbs_id = $1`, spinID).Scan(&orion); err != nil {
		return false, fmt.Errorf("read spin orion code: %w", err)
	}
	refs, err := mbasSpinRefs(ctx, tx, spinID, nullableStringPtr(orion))
	if err != nil {
		return false, err
	}
	if refs > 0 {
		res.ManualReason = mbactivesync.ReasonHasRefs
		res.Result, res.Reason = mbactivesync.ResultSkipped, mbactivesync.ReasonHasRefs
		res.Message = fmt.Sprintf("%d MB_SP_CODE row(s) reference the created spin; deactivate it manually", refs)
		return false, nil
	}
	if err := w.mbasDeactivateSpinOn(ctx, tx, spinID, actor); err != nil {
		if errors.Is(err, mbspin.ErrNotFound) {
			res.Result, res.Reason, res.Message = mbactivesync.ResultSkipped, mbactivesync.ReasonChangedSinceApply, "created spin vanished"
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ---------------------------------------------------------------------------
// DEACTIVATE_HEAD: head + exactly the spins the item deactivated
// ---------------------------------------------------------------------------

func mbasRevertDeactivateHead(ctx context.Context, tx *sql.Tx, in mbactivesync.RevertInput, res *mbactivesync.ItemResult) (bool, error) {
	var before, after mbasDeactHeadImage
	if json.Unmarshal([]byte(in.BeforeJSON), &before) != nil || json.Unmarshal([]byte(in.AfterJSON), &after) != nil || in.HeadID == uuid.Nil {
		return mbasVerdict(res, mbactivesync.ResultFailed, mbactivesync.ReasonBadPlanItem, "before_json/after_json/head_id of the result row are not a deactivate-head image")
	}
	head, found, err := readMBASDeactHead(ctx, tx, in.HeadID, true)
	if err != nil {
		return false, err
	}
	if !found || head.deleted || head.superseded {
		return mbasChanged(res, "head missing, deleted or superseded since apply")
	}
	res.BeforeJSON = mbasJSON(mbasDeactHeadImage{Head: *head, Spins: before.Spins})
	headRestore := false
	switch head.IsActive {
	case before.Head.IsActive:
	case after.Head.IsActive:
		headRestore = true
	}
	restore, spinsBefore, msg, err := mbasCheckDeactSpins(ctx, tx, before.Spins)
	if err != nil {
		return false, err
	}
	if msg != "" {
		return mbasChanged(res, msg)
	}
	allBefore := head.IsActive == before.Head.IsActive && spinsBefore
	if allBefore {
		return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyReverted, "")
	}
	return mbasApplyDeactRestore(ctx, tx, in, res, before, restore, headRestore)
}

// mbasApplyDeactRestore writes the DEACTIVATE_HEAD restore (spins, then head) and fills the result images.
func mbasApplyDeactRestore(ctx context.Context, tx *sql.Tx, in mbactivesync.RevertInput, res *mbactivesync.ItemResult, before mbasDeactHeadImage, restore []uuid.UUID, headRestore bool) (bool, error) {
	if conflict, err := mbasReactivateSpins(ctx, tx, restore, in.Actor); err != nil {
		return false, err
	} else if conflict != uuid.Nil {
		return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonFGConflict,
			fmt.Sprintf("reactivating spin %s violates unique (shade, FG)", conflict))
	}
	if headRestore {
		if _, err := tx.ExecContext(ctx, `
			UPDATE mst_mb_head SET mbh_is_active = $2, updated_at = NOW(), updated_by = $3
			 WHERE mbh_id = $1 AND deleted_at IS NULL`, in.HeadID, before.Head.IsActive, in.Actor); err != nil {
			return false, fmt.Errorf("restore head: %w", err)
		}
	}
	now, _, err := readMBASDeactHead(ctx, tx, in.HeadID, false)
	if err != nil {
		return false, err
	}
	nowImg := mbasDeactHeadImage{Head: *now, Spins: make([]mbasLiveSpin, 0, len(before.Spins))}
	for _, s := range before.Spins {
		nowImg.Spins = append(nowImg.Spins, mbasLiveSpin{ID: s.ID, IsActive: s.IsActive})
	}
	res.Result, res.Reason = mbactivesync.ResultApplied, ""
	res.AfterJSON = mbasJSON(nowImg)
	res.Message = fmt.Sprintf("head and %d spin(s) restored", len(restore))
	return true, nil
}

// ---------------------------------------------------------------------------
// SUPERSEDE_HEAD: restore the old head (workflow log rows stay as audit)
// ---------------------------------------------------------------------------

func mbasRevertSupersedeHead(ctx context.Context, tx *sql.Tx, in mbactivesync.RevertInput, res *mbactivesync.ItemResult) (bool, error) {
	var before, after mbasHeadSupersedeImage
	if json.Unmarshal([]byte(in.BeforeJSON), &before) != nil || json.Unmarshal([]byte(in.AfterJSON), &after) != nil || in.HeadID == uuid.Nil {
		return mbasVerdict(res, mbactivesync.ResultFailed, mbactivesync.ReasonBadPlanItem, "before_json/after_json/head_id of the result row are not a head-supersede image")
	}
	cur, found, err := readMBASHeadSupersede(ctx, tx, in.HeadID, true)
	if err != nil {
		return false, err
	}
	if !found || cur.deleted {
		return mbasChanged(res, "head missing or deleted")
	}
	res.BeforeJSON = mbasJSON(cur)
	same := func(a, b *mbasHeadSupersedeImage) bool {
		return a.IsActive == b.IsActive && mbasEqUUID(a.SupersededBy, b.SupersededBy)
	}
	switch {
	case same(cur, &before):
		return mbasVerdict(res, mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyReverted, "")
	case !same(cur, &after):
		return mbasChanged(res, "head supersede state differs from the after-image")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE mst_mb_head
		   SET mbh_is_active = $2, mbh_superseded_by_head_id = $3, mbh_superseded_at = $4, mbh_superseded_by = $5,
		       updated_at = NOW(), updated_by = $6
		 WHERE mbh_id = $1 AND deleted_at IS NULL`,
		in.HeadID, before.IsActive, before.SupersededBy, before.SupersededAt, before.SupersededActor, in.Actor); err != nil {
		return false, fmt.Errorf("restore superseded head: %w", err)
	}
	now, _, err := readMBASHeadSupersede(ctx, tx, in.HeadID, false)
	if err != nil {
		return false, err
	}
	res.Result, res.Reason = mbactivesync.ResultApplied, ""
	res.AfterJSON = mbasJSON(now)
	res.Message = "head restored (SUPERSEDE workflow log row kept as audit)"
	return true, nil
}

func mbasEqUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// mbasCheckDeactSpins compares the live spins of a DEACTIVATE_HEAD item with its before
// image: restore lists the spins to reactivate, allBefore is true when all already hold
// it, msg is set when a spin changed since apply (missing, deleted or superseded).
func mbasCheckDeactSpins(ctx context.Context, tx *sql.Tx, spins []mbasLiveSpin) (restore []uuid.UUID, allBefore bool, msg string, err error) {
	allBefore = true
	for _, s := range spins {
		var active, superseded bool
		qerr := tx.QueryRowContext(ctx, `
			SELECT mbs_is_active, mbs_superseded_by_spin_id IS NOT NULL FROM mst_mb_spin
			 WHERE mbs_id = $1 AND deleted_at IS NULL`+mbasForUpdate, s.ID).Scan(&active, &superseded)
		if errors.Is(qerr, sql.ErrNoRows) {
			return nil, false, fmt.Sprintf("spin %s missing or deleted since apply", s.ID), nil
		}
		if qerr != nil {
			return nil, false, "", fmt.Errorf("read spin to revert: %w", qerr)
		}
		switch {
		case superseded:
			return nil, false, fmt.Sprintf("spin %s was superseded since apply", s.ID), nil
		case active != s.IsActive:
			allBefore = false
			restore = append(restore, s.ID)
		}
	}
	return restore, allBefore, "", nil
}

// mbasReactivateSpins reactivates the spins; conflict is the spin whose reactivation
// violates unique (shade, FG) (uuid.Nil when none).
func mbasReactivateSpins(ctx context.Context, tx *sql.Tx, ids []uuid.UUID, actor string) (conflict uuid.UUID, err error) {
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `
			UPDATE mst_mb_spin SET mbs_is_active = TRUE, updated_at = NOW(), updated_by = $2
			 WHERE mbs_id = $1 AND deleted_at IS NULL`, id, actor); err != nil {
			if isSpinShadeFgViolation(err) || isPGUniqueViolation(err) {
				return id, nil
			}
			return uuid.Nil, fmt.Errorf("reactivate spin: %w", err)
		}
	}
	return uuid.Nil, nil
}
