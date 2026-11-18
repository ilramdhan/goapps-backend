package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// DUPLICATE_SPIN / GENERATE_SPIN role=DUP (design part 2 §5.4): tx 1 is the EXISTING
// duplicate use case (injected SpinDuplicator → mbspin.DuplicateHandler, its own tx),
// tx 2 is SyncExistingSpin on the clone. Idempotency: the clone recorded for the row
// (--prev-results) or a deterministic claim among the source's clones is reused, so a
// rerun after a failed tx 2 never creates a second clone.

// WithDuplicator sets the duplicate-spin use case used by DuplicateToTarget (tx 1).
func (w *MBActiveSyncWriter) WithDuplicator(d mbactivesync.SpinDuplicator) *MBActiveSyncWriter {
	w.dup = d
	return w
}

// mbasDupSource is the part of the source spin DuplicateToTarget needs.
type mbasDupSource struct {
	name     string
	shade    string
	denier   *float64
	filament *int
	isRoot   bool
}

const mbasDupSourceQuery = `
	SELECT mbs_mgt_name, COALESCE(mbs_shade_code, ''), mbs_denier, mbs_filament, mbs_parent_spin_id IS NULL
	  FROM mst_mb_spin
	 WHERE mbs_id = $1 AND deleted_at IS NULL`

func (w *MBActiveSyncWriter) readDupSource(ctx context.Context, id uuid.UUID) (*mbasDupSource, bool, error) {
	var s mbasDupSource
	var den sql.NullFloat64
	var fil sql.NullInt64
	err := w.db.QueryRowContext(ctx, mbasDupSourceQuery, id).Scan(&s.name, &s.shade, &den, &fil, &s.isRoot)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read duplicate source: %w", err)
	}
	if den.Valid {
		v := den.Float64
		s.denier = &v
	}
	if fil.Valid {
		v := int(fil.Int64)
		s.filament = &v
	}
	return &s, true, nil
}

// mbasCloneCand is one existing clone of the source (claim fallback).
type mbasCloneCand struct {
	id uuid.UUID
	fg *int64
}

// mbasCloneCandidatesQuery lists the source's non-deleted, non-superseded clones made
// by actor with the given den/fil (numeric(10,2) / integer, NULL-safe), oldest first.
const mbasCloneCandidatesQuery = `
	SELECT mbs_id, mbs_fg_product_id
	  FROM mst_mb_spin
	 WHERE mbs_parent_spin_id = $1 AND mbs_duplicated_by = $2
	   AND deleted_at IS NULL AND mbs_superseded_by_spin_id IS NULL
	   AND mbs_denier IS NOT DISTINCT FROM ROUND(CAST($3 AS numeric), 2)
	   AND mbs_filament IS NOT DISTINCT FROM CAST($4 AS integer)
	 ORDER BY created_at ASC, mbs_id ASC`

func (w *MBActiveSyncWriter) cloneCandidates(ctx context.Context, source uuid.UUID, actor string, den *float64, fil *int) (_ []mbasCloneCand, err error) {
	var denArg, filArg any
	if den != nil {
		denArg = *den
	}
	if fil != nil {
		filArg = *fil
	}
	rows, err := w.db.QueryContext(ctx, mbasCloneCandidatesQuery, source, actor, denArg, filArg)
	if err != nil {
		return nil, fmt.Errorf("list clone candidates: %w", err)
	}
	defer closeRows(rows)
	var out []mbasCloneCand
	for rows.Next() {
		var c mbasCloneCand
		var fg sql.NullInt64
		if err := rows.Scan(&c.id, &fg); err != nil {
			return nil, fmt.Errorf("scan clone candidate: %w", err)
		}
		if fg.Valid {
			v := fg.Int64
			c.fg = &v
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate clone candidates: %w", err)
	}
	return out, nil
}

// mbasPickClone is the deterministic claim (§5.4): among unclaimed candidates (oldest
// first) take the one already holding the target FG, else the first without FG (a clone
// whose tx 2 never finished). A clone holding another FG belongs to another row.
func mbasPickClone(cands []mbasCloneCand, exclude []uuid.UUID, targetFG *int64) uuid.UUID {
	skip := make(map[uuid.UUID]bool, len(exclude))
	for _, id := range exclude {
		skip[id] = true
	}
	firstFree := uuid.Nil
	for _, c := range cands {
		if skip[c.id] {
			continue
		}
		if c.fg == nil {
			if firstFree == uuid.Nil {
				firstFree = c.id
			}
			continue
		}
		if targetFG != nil && *c.fg == *targetFG {
			return c.id
		}
	}
	return firstFree
}

// DuplicateToTarget implements mbactivesync.Writer (design part 2 §5.4).
func (w *MBActiveSyncWriter) DuplicateToTarget(ctx context.Context, in mbactivesync.DuplicateTargetInput) (mbactivesync.ItemResult, error) {
	actor := in.Target.Actor
	res := mbactivesync.ItemResult{SpinID: in.SourceSpinID.String()}
	src, found, err := w.readDupSource(ctx, in.SourceSpinID)
	if err != nil {
		return res, err
	}
	if !found || !src.isRoot {
		res.Result, res.Reason, res.Message = mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, "duplicate source missing, deleted or not a root spin"
		return res, nil
	}
	den, fil := in.TargetDen, in.TargetFil
	if den == nil {
		den = src.denier
	}
	if fil == nil {
		fil = src.filament
	}

	clone, claimed, out, done, err := w.resolveClone(ctx, in, src, den, fil)
	if err != nil || done {
		return out, err
	}
	if !claimed {
		partial := mbactivesync.ItemResult{Result: mbactivesync.ResultPartial, SpinID: clone.String(), CloneSpinID: clone.String(),
			Message: "clone created (tx 1); target state pending (tx 2)"}
		if in.OnCloned != nil {
			if cbErr := in.OnCloned(partial); cbErr != nil {
				fail := w.partialFailure(ctx, clone, actor, "result flush failed before tx 2")
				return fail, fmt.Errorf("record partial clone %s: %w", clone, cbErr)
			}
		}
	}
	return w.syncClone(ctx, in, clone, claimed)
}

// resolveClone returns the clone for the row: --prev-results id, a claimed existing clone
// (claimed=true) or a fresh one from tx 1. done=true ends the item with out.
func (w *MBActiveSyncWriter) resolveClone(ctx context.Context, in mbactivesync.DuplicateTargetInput, src *mbasDupSource,
	den *float64, fil *int,
) (clone uuid.UUID, claimed bool, out mbactivesync.ItemResult, done bool, err error) {
	out = mbactivesync.ItemResult{SpinID: in.SourceSpinID.String()}
	actor := in.Target.Actor
	if in.CloneSpinID != uuid.Nil {
		ok, cerr := w.isCloneOf(ctx, in.CloneSpinID, in.SourceSpinID)
		if cerr != nil {
			return uuid.Nil, false, out, false, cerr
		}
		if !ok {
			out.Result, out.Reason, out.CloneSpinID = mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan, in.CloneSpinID.String()
			out.Message = "clone from --prev-results is missing, deleted or not a clone of the source"
			return uuid.Nil, false, out, true, nil
		}
		return in.CloneSpinID, true, out, false, nil
	}
	cands, err := w.cloneCandidates(ctx, in.SourceSpinID, actor, den, fil)
	if err != nil {
		return uuid.Nil, false, out, false, err
	}
	if id := mbasPickClone(cands, in.ExcludeClones, in.Target.TargetFG); id != uuid.Nil {
		return id, true, out, false, nil
	}
	if w.dup == nil {
		return uuid.Nil, false, out, false, errors.New("duplicate-to-target: no spin duplicator wired")
	}
	name := src.name // Q10: clone keeps the source name (no " (copy)")
	req := mbactivesync.DuplicateSpinRequest{SourceSpinID: in.SourceSpinID, MgtName: &name, Denier: in.TargetDen, Filament: in.TargetFil, Actor: actor}
	if in.Target.Has(mbactivesync.SubSetFG) && in.Target.TargetFG != nil && mbactivesync.NormShadeExact(src.shade) != "" &&
		mbactivesync.NormShadeExact(src.shade) == mbactivesync.NormShadeExact(in.Target.TargetShadeCode) {
		fg := *in.Target.TargetFG // prepareCloneFg validates against the SOURCE shade: only when it already is the target
		req.FgProductID = &fg
	}
	id, derr := w.dup.Duplicate(ctx, req)
	if derr != nil {
		if st, reason := mbasMapDuplicateErr(derr); reason != "" {
			out.Result, out.Reason, out.Message = st, reason, derr.Error()
			return uuid.Nil, false, out, true, nil
		}
		return uuid.Nil, false, out, false, fmt.Errorf("duplicate spin: %w", derr)
	}
	return id, false, out, false, nil
}

// mbasMapDuplicateErr maps the duplicate use-case domain errors to an item verdict
// ("" reason = unexpected, surfaced as a DB error).
func mbasMapDuplicateErr(err error) (status, reason string) {
	switch {
	case errors.Is(err, mbspin.ErrSpinShadeFgExists):
		return mbactivesync.ResultSkipped, mbactivesync.ReasonFGConflict
	case errors.Is(err, mbspin.ErrFgNotPoy):
		return mbactivesync.ResultFailed, mbactivesync.ReasonPOYInvalid
	case errors.Is(err, mbspin.ErrFgShadeMismatch):
		return mbactivesync.ResultFailed, mbactivesync.ReasonShadeMismatch
	case errors.Is(err, mbspin.ErrNotFound), errors.Is(err, mbspin.ErrAlreadyDeleted),
		errors.Is(err, mbspin.ErrAlreadyDuplicated), errors.Is(err, mbspin.ErrParentCycle):
		return mbactivesync.ResultFailed, mbactivesync.ReasonStalePlan
	}
	return "", ""
}

func (w *MBActiveSyncWriter) isCloneOf(ctx context.Context, clone, source uuid.UUID) (bool, error) {
	var ok bool
	err := w.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM mst_mb_spin
		               WHERE mbs_id = $1 AND mbs_parent_spin_id = $2 AND deleted_at IS NULL)`, clone, source).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check clone: %w", err)
	}
	return ok, nil
}

// syncClone runs tx 2 (SyncExistingSpin on the clone, ACT_SPIN added so a clone the Q1
// guard deactivated is reactivated) and maps its outcome.
func (w *MBActiveSyncWriter) syncClone(ctx context.Context, in mbactivesync.DuplicateTargetInput, clone uuid.UUID, claimed bool) (mbactivesync.ItemResult, error) {
	target := in.Target
	target.SpinID = clone
	target.SubActions = append(append([]string{}, in.Target.SubActions...), mbactivesync.SubActSpin)
	res, err := w.SyncExistingSpin(ctx, target)
	res.SpinID, res.CloneSpinID = clone.String(), clone.String()
	if err != nil {
		fail := w.partialFailure(ctx, clone, target.Actor, "tx 2 database error: "+err.Error())
		fail.BeforeJSON = res.BeforeJSON
		return fail, err
	}
	switch {
	case res.Result == mbactivesync.ResultApplied:
		return res, nil
	case res.Result == mbactivesync.ResultSkipped && res.Reason == mbactivesync.ReasonAlreadyInTarget:
		if !claimed {
			res.Result, res.Reason = mbactivesync.ResultApplied, "" // fresh clone already in target (same shade, FG at tx 1)
		}
		return res, nil
	}
	fail := w.partialFailure(ctx, clone, target.Actor, fmt.Sprintf("tx 2 %s %s: %s", res.Result, res.Reason, res.Message))
	fail.BeforeJSON = res.BeforeJSON
	return fail, nil
}

// partialFailure builds FAILED partial_clone and applies the Q1 guard: a clone left
// active on an inactive head is deactivated (own tx) and listed in manual-items.
func (w *MBActiveSyncWriter) partialFailure(ctx context.Context, clone uuid.UUID, actor, msg string) mbactivesync.ItemResult {
	res := mbactivesync.ItemResult{Result: mbactivesync.ResultFailed, Reason: mbactivesync.ReasonPartialClone,
		SpinID: clone.String(), CloneSpinID: clone.String(), Message: msg}
	headID, deactivated, err := w.guardInactiveHeadClone(ctx, clone, actor)
	if headID != uuid.Nil {
		res.HeadID = headID.String()
	}
	switch {
	case err != nil:
		res.Message += "; Q1 guard failed: " + err.Error()
		res.ManualReason = mbactivesync.ReasonPartialCloneInactiveHead
	case deactivated:
		res.Message += "; clone deactivated (head inactive, Q1)"
		res.ManualReason = mbactivesync.ReasonPartialCloneInactiveHead
	}
	return res
}

// guardInactiveHeadClone deactivates the clone when its head is inactive (one tx).
func (w *MBActiveSyncWriter) guardInactiveHeadClone(ctx context.Context, clone uuid.UUID, actor string) (headID uuid.UUID, deactivated bool, err error) {
	var res mbactivesync.ItemResult
	_, err = w.mbasItemTx(ctx, &res, func(tx *sql.Tx) (bool, error) {
		img, found, rerr := readMBASImage(ctx, tx, clone, true)
		if rerr != nil || !found {
			return false, rerr
		}
		headID = img.Spin.headID
		if img.Head.IsActive || !img.Spin.IsActive || img.Spin.superseded {
			return false, nil
		}
		if derr := w.mbasDeactivateSpinOn(ctx, tx, clone, actor); derr != nil {
			return false, derr
		}
		deactivated = true
		return true, nil
	})
	if err != nil {
		deactivated = false
	}
	return headID, deactivated, err
}
