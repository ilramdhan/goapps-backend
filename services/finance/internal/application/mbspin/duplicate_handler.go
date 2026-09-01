// Package mbspin provides application layer handlers for MB Spin operations.
//
// ⛔ ISOLATION: this file must never import or call the calc-engine v2
// (`internal/application/rmcost`). Duplicating a spin NEVER recalculates a yarn
// product (decision D24) — the chain stops at the child spin.
package mbspin

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// DuplicateCommand is the application-level payload of DuplicateMBSpin.
type DuplicateCommand struct {
	// SourceSpinID is the spin being cloned; it becomes the clone's
	// mbs_parent_spin_id.
	SourceSpinID uuid.UUID
	// HeadID, when non-nil, is the mbh_id the caller believes the source belongs
	// to. It is VERIFIED against the stored value rather than trusted: the
	// gRPC/BFF surface carries mbh_id alongside mbs_id, and a mismatch means the
	// client is acting on a stale tree.
	HeadID *uuid.UUID
	// MgtName overrides the clone's name. nil => source name + " (copy)".
	MgtName *string
	// Denier / Filament override the copied values. nil => copy the source.
	Denier   *float64
	Filament *int
	// OrionItemCode is a NEW ERP item code for the clone.
	//
	// ⚠ Today this is ALWAYS nil: decision D19 requires the clone's
	// mbs_orion_item_code to be NULL, so DuplicateMBSpinRequest deliberately has
	// no field for it and the delivery layer never sets this. It exists so the
	// uniqueness gate is wired on this path too, rather than being a hole that
	// only shows up the day the field is added.
	OrionItemCode *string
	// ActorUserID lands in created_by and mbs_duplicated_by.
	ActorUserID string
}

// DuplicateResult is the outcome of DuplicateMBSpin.
type DuplicateResult struct {
	// Clone is the freshly inserted spin, re-read so the response carries the
	// stored values (including the nulled ERP keys) rather than a guess.
	Clone *mbspin.Entity
	// Output is the persistence-level echo (ids, name, lineage depth).
	Output mbspin.DuplicateOutput
	// Recalc carries the A7 skip list and the D24 impact PREVIEW, both computed
	// against the SOURCE spin. ⛔ Nothing in it is a yarn-product recalculation.
	Recalc *RecalcResult
}

// DuplicateHandler orchestrates the duplicate-spin use case.
type DuplicateHandler struct {
	repo   mbspin.Repository
	recalc *RecalcService
}

// NewDuplicateHandler creates a DuplicateHandler. recalc may be nil, in which
// case the response simply carries no skip list and no impact preview.
func NewDuplicateHandler(repo mbspin.Repository, recalc *RecalcService) *DuplicateHandler {
	return &DuplicateHandler{repo: repo, recalc: recalc}
}

// Handle validates, clones, and previews.
//
// Order matters: the ORION uniqueness gate runs BEFORE DuplicateSpin so a
// rejected code costs no write, and the preview runs AFTER the commit so it
// reports the tree as it actually stands.
//
// ⛔ NO RECALCULATION HAPPENS HERE. A clone copies its source's dozing verbatim,
// so no number moved and there is nothing to recompute — not on the child spins
// (they are only classified and reported), and above all not on yarn products
// (D24). The impact_* figures are counted with a SELECT.
func (h *DuplicateHandler) Handle(ctx context.Context, cmd DuplicateCommand) (*DuplicateResult, error) {
	if cmd.SourceSpinID == uuid.Nil {
		return nil, mbspin.ErrNotFound
	}
	if cmd.ActorUserID == "" {
		return nil, mbspin.ErrEmptyCreatedBy
	}

	source, err := h.repo.GetByID(ctx, cmd.SourceSpinID)
	if err != nil {
		return nil, err
	}
	if source.IsDeleted() {
		return nil, mbspin.ErrAlreadyDeleted
	}
	// Only a root spin (mbs_parent_spin_id IS NULL) may be duplicated: this caps
	// lineage depth at one level and rejects re-duplicating an already-cloned
	// spin, regardless of what the new mbs_parent_spin_id would be. This is a
	// sibling check to AssertNoParentCycle (which guards the self-loop case
	// inside DuplicateSpin) — it inspects the SOURCE's own lineage instead, so it
	// must run before any write, not just before a cycle would form.
	if source.ParentSpinID() != nil {
		return nil, mbspin.ErrAlreadyDuplicated
	}
	if cmd.HeadID != nil && *cmd.HeadID != uuid.Nil && *cmd.HeadID != source.HeadID() {
		return nil, mbspin.ErrHeadNotFound
	}

	// Uniqueness is checked ONLY for a code the caller is actually supplying.
	// An absent or empty code is skipped: 177 legacy codes are shared by 466
	// rows and must keep saving (see ensureOrionItemCodeUnique).
	if err := ensureOrionItemCodeUnique(ctx, h.repo, cmd.OrionItemCode, source.OrionItemCode()); err != nil {
		return nil, err
	}

	out, err := h.repo.DuplicateSpin(ctx, mbspin.DuplicateInput{
		SourceSpinID: cmd.SourceSpinID,
		MgtName:      cmd.MgtName,
		Denier:       cmd.Denier,
		Filament:     cmd.Filament,
		ActorUserID:  cmd.ActorUserID,
	})
	if err != nil {
		return nil, err
	}

	clone, err := h.repo.GetByID(ctx, out.NewSpinID)
	if err != nil {
		return nil, err
	}

	res := &DuplicateResult{Clone: clone, Output: out}

	if h.recalc == nil {
		return res, nil
	}
	// Preview, not recalc: classify the SOURCE spin's direct children (the clone
	// has none yet) and count the products bound to the source. Read-only.
	preview, err := h.recalc.Preview(ctx, source)
	if err != nil {
		return nil, err
	}
	res.Recalc = preview
	return res, nil
}

// ensureOrionItemCodeUnique is the SOLE enforcer of ORION item-code uniqueness:
// migration 000486's unique index was permanently abandoned.
//
// ⚠ It runs ONLY for a value that is genuinely NEW:
//   - candidate == nil  => the caller is not touching the code  => skip
//   - *candidate == ""  => clearing / absent code               => skip
//   - equal to current  => the code did not change              => skip
//
// Those three skips are load-bearing, not defensive noise. 177 legacy codes are
// shared by 466 rows; running the check on an unchanged code would make every
// one of those rows unsavable. Because the check only ever runs on a CHANGED
// code, the row being saved can never be its own match, so no self-exclusion is
// needed.
func ensureOrionItemCodeUnique(ctx context.Context, repo mbspin.Repository, candidate, current *string) error {
	if candidate == nil || *candidate == "" {
		return nil
	}
	if current != nil && *current == *candidate {
		return nil
	}
	exists, err := repo.ExistsByOrionItemCode(ctx, *candidate)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: %q", mbspin.ErrDuplicateOrionItemCode, *candidate)
	}
	return nil
}
