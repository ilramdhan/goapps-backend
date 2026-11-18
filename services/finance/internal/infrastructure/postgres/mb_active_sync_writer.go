package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// MBActiveSyncWriter implements mbactivesync.Writer: one transaction per plan
// item, reusing the existing spin/backfill helpers (design part 2 §5).
type MBActiveSyncWriter struct {
	db   *DB
	spin *MBSpinRepository
}

// NewMBActiveSyncWriter creates the writer.
func NewMBActiveSyncWriter(db *DB) *MBActiveSyncWriter {
	return &MBActiveSyncWriter{db: db, spin: NewMBSpinRepository(db)}
}

var _ mbactivesync.Writer = (*MBActiveSyncWriter)(nil)

// mbasSpinImage is the before/after image of a spin (design part 2 §5.3).
type mbasSpinImage struct {
	ShadeCode    string   `json:"shade_code"`
	ShadeName    string   `json:"shade_name"`
	CC           string   `json:"cc"`
	CrossSection string   `json:"cross_section"`
	RunLDRPct    *float64 `json:"run_ldr_pct"`
	LDRType      string   `json:"ldr_type"`
	LDRIsActual  bool     `json:"ldr_is_actual"`
	LDRIsFixed   bool     `json:"ldr_is_fixed"`
	IsActive     bool     `json:"is_active"`
	FGProductID  *int64   `json:"fg_product_id"`
	FGItemCode   string   `json:"fg_item_code"`
	FGLinkSource string   `json:"fg_link_source"`

	headID     uuid.UUID
	superseded bool
	isRoot     bool
}

// mbasHeadImage is the before/after image of the owning head (Q13: revert restores all of it).
type mbasHeadImage struct {
	IsActive    bool   `json:"mbh_is_active"`
	FGProductID *int64 `json:"mbh_fg_product_id"`
	FGItemCode  string `json:"mbh_fg_item_code"`
	ShadeCode   string `json:"mbh_shade_code"`
	ShadeName   string `json:"mbh_shade_name"`

	deleted    bool
	superseded bool
}

type mbasImage struct {
	Spin mbasSpinImage `json:"spin"`
	Head mbasHeadImage `json:"head"`
}

const mbasSpinImageQuery = `
	SELECT COALESCE(mbs_shade_code, ''), COALESCE(mbs_shade_name, ''), COALESCE(mbs_cc, ''), COALESCE(mbs_cross_section, ''),
	       mbs_run_ldr_pct, COALESCE(mbs_ldr_type, ''), COALESCE(mbs_ldr_is_actual, FALSE), COALESCE(mbs_ldr_is_fixed, FALSE),
	       mbs_is_active, mbs_fg_product_id, COALESCE(mbs_fg_item_code, ''), COALESCE(mbs_fg_link_source, ''),
	       mbs_mbh_id, mbs_superseded_by_spin_id IS NOT NULL, mbs_parent_spin_id IS NULL
	  FROM mst_mb_spin
	 WHERE mbs_id = $1 AND deleted_at IS NULL`

const mbasHeadImageQuery = `
	SELECT mbh_is_active, mbh_fg_product_id, COALESCE(mbh_fg_item_code, ''),
	       COALESCE(mbh_shade_code, ''), COALESCE(mbh_shade_name, ''),
	       deleted_at IS NOT NULL, mbh_superseded_by_head_id IS NOT NULL
	  FROM mst_mb_head
	 WHERE mbh_id = $1`

// readMBASImage reads the spin and its head; lock adds FOR UPDATE to both.
func readMBASImage(ctx context.Context, tx *sql.Tx, spinID uuid.UUID, lock bool) (*mbasImage, bool, error) {
	suffix := ""
	if lock {
		suffix = mbasForUpdate
	}
	var img mbasImage
	s := &img.Spin
	var ldr sql.NullFloat64
	var fg sql.NullInt64
	err := tx.QueryRowContext(ctx, mbasSpinImageQuery+suffix, spinID).Scan(
		&s.ShadeCode, &s.ShadeName, &s.CC, &s.CrossSection, &ldr, &s.LDRType, &s.LDRIsActual, &s.LDRIsFixed,
		&s.IsActive, &fg, &s.FGItemCode, &s.FGLinkSource, &s.headID, &s.superseded, &s.isRoot)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read spin image: %w", err)
	}
	if ldr.Valid {
		v := ldr.Float64
		s.RunLDRPct = &v
	}
	if fg.Valid {
		v := fg.Int64
		s.FGProductID = &v
	}
	h := &img.Head
	var hfg sql.NullInt64
	err = tx.QueryRowContext(ctx, mbasHeadImageQuery+suffix, s.headID).Scan(
		&h.IsActive, &hfg, &h.FGItemCode, &h.ShadeCode, &h.ShadeName, &h.deleted, &h.superseded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read head image: %w", err)
	}
	if hfg.Valid {
		v := hfg.Int64
		h.FGProductID = &v
	}
	return &img, true, nil
}

func (i *mbasImage) json() string {
	b, err := json.Marshal(i)
	if err != nil {
		return ""
	}
	return string(b)
}

// mbasNeeds is the set of changes still required against the locked live state.
type mbasNeeds struct {
	shade, ldr, actSpin, actHead, headShade, fg bool
}

func (n mbasNeeds) any() bool {
	return n.shade || n.ldr || n.actSpin || n.actHead || n.headShade || n.fg
}

func mbasComputeNeeds(in mbactivesync.SyncSpinInput, img *mbasImage) mbasNeeds {
	sp := img.Spin
	n := mbasNeeds{
		shade:     in.Has(mbactivesync.SubSetShade) && mbasShadeDiffers(sp.ShadeCode, sp.ShadeName, in),
		actSpin:   in.Has(mbactivesync.SubActSpin) && !sp.IsActive,
		actHead:   in.Has(mbactivesync.SubActHead) && !img.Head.IsActive,
		headShade: mbasHeadShadeNeeded(in, img),
	}
	if in.Has(mbactivesync.SubSetLDR) && in.TargetLDR != nil {
		n.ldr = sp.RunLDRPct == nil ||
			mbactivesync.Fix4(*sp.RunLDRPct) != mbactivesync.Fix4(*in.TargetLDR) ||
			sp.LDRType != mbspin.LDRTypeActual || !sp.LDRIsActual || !sp.LDRIsFixed
	}
	if in.Has(mbactivesync.SubSetFG) && in.TargetFG != nil {
		n.fg = sp.FGProductID == nil || *sp.FGProductID != *in.TargetFG
	}
	return n
}

func mbasShadeDiffers(code, name string, in mbactivesync.SyncSpinInput) bool {
	if mbactivesync.NormShadeExact(code) != mbactivesync.NormShadeExact(in.TargetShadeCode) {
		return true
	}
	t := trimSpaceStr(in.TargetShadeName)
	return t != "" && trimSpaceStr(name) != t
}

func mbasHeadShadeNeeded(in mbactivesync.SyncSpinInput, img *mbasImage) bool {
	if !img.Spin.isRoot || (!in.Has(mbactivesync.SubSetHeadShade) && !in.Has(mbactivesync.SubSetShade)) ||
		mbactivesync.NormShadeLoose(in.TargetShadeCode) == "" {
		return false
	}
	return mbasShadeDiffers(img.Head.ShadeCode, img.Head.ShadeName, in)
}

func trimSpaceStr(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// SyncExistingSpin implements mbactivesync.Writer (design part 2 §5.3), all in one tx.
// Any SKIPPED/FAILED outcome rolls back the whole item; a non-nil error is a DB failure.
func (w *MBActiveSyncWriter) SyncExistingSpin(ctx context.Context, in mbactivesync.SyncSpinInput) (res mbactivesync.ItemResult, err error) {
	res.SpinID = in.SpinID.String()
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return res, fmt.Errorf("begin tx: %w", err)
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

	out, done, err := w.syncExistingSpinTx(ctx, tx, in, &res)
	if err != nil {
		return res, err
	}
	if done {
		return out, nil
	}
	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("commit: %w", err)
	}
	committed = true
	return out, nil
}

func mbasFail(res *mbactivesync.ItemResult, status, reason, msg string) (mbactivesync.ItemResult, bool, error) {
	res.Result, res.Reason, res.Message = status, reason, msg
	return *res, true, nil
}

// mbasPrecheck evaluates the locked state; done=true ends the item without writing.
func mbasPrecheck(in mbactivesync.SyncSpinInput, before *mbasImage, found bool, res *mbactivesync.ItemResult) (mbasNeeds, mbactivesync.ItemResult, bool) {
	fail := func(status, reason, msg string) (mbasNeeds, mbactivesync.ItemResult, bool) {
		res.Result, res.Reason, res.Message = status, reason, msg
		return mbasNeeds{}, *res, true
	}
	if !found || before.Head.deleted {
		return fail(mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "spin or head missing or deleted")
	}
	res.HeadID = before.Spin.headID.String()
	res.BeforeJSON = before.json()
	if before.Spin.superseded {
		return fail(mbactivesync.ResultSkipped, mbactivesync.ReasonSpinSuperseded, "spin is superseded (use un-supersede)")
	}
	if before.Head.superseded {
		return fail(mbactivesync.ResultSkipped, mbactivesync.ReasonHeadSuperseded, "head is superseded")
	}
	needs := mbasComputeNeeds(in, before)
	if st, reason, msg := mbasBlocked(in, before, needs); reason != "" {
		return fail(st, reason, msg)
	}
	return needs, *res, false
}

// mbasBlocked returns the skip/fail verdict (reason != "") for the locked state.
func mbasBlocked(in mbactivesync.SyncSpinInput, before *mbasImage, needs mbasNeeds) (status, reason, msg string) {
	// Q12 re-check: a ROOT head shade that differs beyond format must not be touched.
	if before.Spin.isRoot && (in.Has(mbactivesync.SubSetHeadShade) || in.Has(mbactivesync.SubSetShade)) {
		hl := mbactivesync.NormShadeLoose(before.Head.ShadeCode)
		if hl != "" && hl != mbactivesync.NormShadeLoose(in.TargetShadeCode) {
			return mbactivesync.ResultSkipped, mbactivesync.ReasonHeadShadeDiffers, "head shade differs from target beyond format"
		}
	}
	if !needs.any() {
		return mbactivesync.ResultSkipped, mbactivesync.ReasonAlreadyInTarget, ""
	}
	if needs.ldr && (in.TargetLDR == nil || *in.TargetLDR <= 0 || math.IsNaN(*in.TargetLDR)) {
		return mbactivesync.ResultFailed, mbactivesync.ReasonInvalidLDR, "target LDR missing or <= 0"
	}
	if needs.fg && before.Spin.isRoot && before.Head.FGProductID != nil && *before.Head.FGProductID != *in.TargetFG {
		return mbactivesync.ResultFailed, mbactivesync.ReasonFGFollowsRecipe, "head FG differs from target; edit recipe manually"
	}
	return "", "", ""
}

// syncExistingSpinTx returns done=true when the item ended without a commit (skip/fail).
func (w *MBActiveSyncWriter) syncExistingSpinTx(ctx context.Context, tx *sql.Tx, in mbactivesync.SyncSpinInput, res *mbactivesync.ItemResult) (mbactivesync.ItemResult, bool, error) {
	before, found, err := readMBASImage(ctx, tx, in.SpinID, true)
	if err != nil {
		return *res, false, err
	}
	needs, out, done := mbasPrecheck(in, before, found, res)
	if done {
		return out, true, nil
	}
	if out2, done, err := w.writeSpinFields(ctx, tx, in, needs, res); err != nil || done {
		return out2, done, err
	}
	if out2, done, err := w.writeHead(ctx, tx, in, needs, before.Spin.headID, res); err != nil || done {
		return out2, done, err
	}
	if needs.fg {
		if out2, done, err := mbasApplyFG(ctx, tx, in, res); err != nil || done {
			return out2, done, err
		}
	}
	after, _, err := readMBASImage(ctx, tx, in.SpinID, false)
	if err != nil {
		return *res, false, err
	}
	res.Result, res.Reason = mbactivesync.ResultApplied, ""
	res.AfterJSON = after.json()
	return *res, false, nil
}

// writeHead applies the head-side changes (shade without cascade, activation).
func (w *MBActiveSyncWriter) writeHead(ctx context.Context, tx *sql.Tx, in mbactivesync.SyncSpinInput, n mbasNeeds, headID uuid.UUID, res *mbactivesync.ItemResult) (mbactivesync.ItemResult, bool, error) {
	if n.headShade {
		if err := mbasUpdateHeadShade(ctx, tx, headID, in); err != nil {
			return *res, false, err
		}
	}
	if n.actHead {
		cnt, err := mbasActivateHead(ctx, tx, headID, in.Actor)
		if err != nil {
			return *res, false, err
		}
		if cnt != 1 {
			return mbasFail(res, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "head not activatable (deleted or superseded)")
		}
	}
	return *res, false, nil
}

// writeSpinFields applies shade / actual LDR / activation through the entity and the existing updateOn.
func (w *MBActiveSyncWriter) writeSpinFields(ctx context.Context, tx *sql.Tx, in mbactivesync.SyncSpinInput, n mbasNeeds, res *mbactivesync.ItemResult) (mbactivesync.ItemResult, bool, error) {
	if !n.shade && !n.ldr && !n.actSpin {
		return *res, false, nil
	}
	entity, err := loadSpinForRepointOn(ctx, tx, in.SpinID)
	if err != nil {
		if errors.Is(err, mbspin.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
			return mbasFail(res, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "spin vanished")
		}
		return *res, false, err
	}
	if err := mbasMutateEntity(entity, in, n); err != nil {
		if errors.Is(err, mbspin.ErrInvalidActualLDR) {
			return mbasFail(res, mbactivesync.ResultFailed, mbactivesync.ReasonInvalidLDR, err.Error())
		}
		return *res, false, err
	}
	if err := w.spin.updateOn(ctx, tx, entity); err != nil {
		switch {
		case errors.Is(err, mbspin.ErrSpinShadeFgExists), errors.Is(err, mbspin.ErrAlreadyExists):
			return mbasFail(res, mbactivesync.ResultSkipped, mbactivesync.ReasonFGConflict, "unique (shade, FG) violation")
		case errors.Is(err, mbspin.ErrNotFound):
			return mbasFail(res, mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "spin vanished")
		}
		return *res, false, err
	}
	return *res, false, nil
}

// mbasMutateEntity applies the needed domain setters (K1/K2) to the loaded entity.
func mbasMutateEntity(entity *mbspin.Entity, in mbactivesync.SyncSpinInput, n mbasNeeds) error {
	if n.shade {
		code := in.TargetShadeCode
		name := entity.ShadeName()
		if t := trimSpaceStr(in.TargetShadeName); t != "" {
			name = &t
		}
		cs := entity.CrossSection()
		if in.TargetCS != "" {
			c := in.TargetCS
			cs = &c
		}
		if err := entity.SetShade(&code, name, cs, in.Actor); err != nil {
			return fmt.Errorf("set shade: %w", err)
		}
	}
	if n.ldr {
		if err := entity.SetActualLDR(*in.TargetLDR, in.Actor); err != nil {
			return fmt.Errorf("set actual ldr: %w", err)
		}
	}
	if n.actSpin {
		active := true
		if err := entity.Update(mbspin.UpdateInput{IsActive: &active}, in.Actor); err != nil {
			return fmt.Errorf("activate spin: %w", err)
		}
	}
	return nil
}

// mbasUpdateHeadShade writes the head shade WITHOUT cascade (Q12/Q14); an empty target name keeps the current one.
func mbasUpdateHeadShade(ctx context.Context, tx *sql.Tx, headID uuid.UUID, in mbactivesync.SyncSpinInput) error {
	const q = `
		UPDATE mst_mb_head
		   SET mbh_shade_code = $2, mbh_shade_name = COALESCE(NULLIF($3, ''), mbh_shade_name),
		       updated_at = NOW(), updated_by = $4
		 WHERE mbh_id = $1 AND deleted_at IS NULL`
	if _, err := tx.ExecContext(ctx, q, headID, in.TargetShadeCode, trimSpaceStr(in.TargetShadeName), in.Actor); err != nil {
		return fmt.Errorf("update head shade: %w", err)
	}
	return nil
}

// mbasActivateHead sets only mbh_is_active (Q13), bypassing ErrHeadLocked and the cascade.
func mbasActivateHead(ctx context.Context, tx *sql.Tx, headID uuid.UUID, actor string) (int64, error) {
	const q = `
		UPDATE mst_mb_head
		   SET mbh_is_active = TRUE, updated_at = NOW(), updated_by = $2
		 WHERE mbh_id = $1 AND deleted_at IS NULL AND mbh_superseded_by_head_id IS NULL`
	r, err := tx.ExecContext(ctx, q, headID, actor)
	if err != nil {
		return 0, fmt.Errorf("activate head: %w", err)
	}
	n, err := r.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("activate head rows: %w", err)
	}
	return n, nil
}

// mbasApplyFG links the FG through the existing backfill helper; non-applied outcomes end the item.
func mbasApplyFG(ctx context.Context, tx *sql.Tx, in mbactivesync.SyncSpinInput, res *mbactivesync.ItemResult) (mbactivesync.ItemResult, bool, error) {
	out, err := applyBackfillInTx(ctx, tx, mbspin.BackfillApplyRequest{
		SpinID: in.SpinID.String(), PoyProductSysID: *in.TargetFG, Actor: in.Actor,
	})
	if err != nil {
		return *res, false, err
	}
	if out.Result == mbspin.BackfillResultApplied {
		return *res, false, nil
	}
	switch out.Reason {
	case mbspin.BackfillReasonAlreadyLinked:
		return *res, false, nil // success
	case mbspin.BackfillReasonConflictExistingLive, mbspin.BackfillReasonConflictSameTarget, mbspin.BackfillReasonConflictUnique:
		return mbasFail(res, mbactivesync.ResultSkipped, mbactivesync.ReasonFGConflict, out.Message)
	case mbspin.BackfillReasonFgAlreadySet:
		return mbasFail(res, mbactivesync.ResultFailed, mbactivesync.ReasonFGMismatch, out.Message)
	case mbspin.BackfillReasonPoyInvalid:
		return mbasFail(res, mbactivesync.ResultFailed, mbactivesync.ReasonPOYInvalid, out.Message)
	case mbspin.BackfillReasonShadeMismatch:
		return mbasFail(res, mbactivesync.ResultFailed, mbactivesync.ReasonShadeMismatch, out.Message)
	default:
		return mbasFail(res, mbactivesync.ResultFailed, out.Reason, out.Message)
	}
}
