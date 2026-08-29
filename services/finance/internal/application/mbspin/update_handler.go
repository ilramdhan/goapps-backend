// Package mbspin provides application layer handlers for MB Spin operations.
package mbspin

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// UpdateCommand represents the update MB Spin command.
type UpdateCommand struct {
	ID              uuid.UUID
	MgtName         *string
	MBCosting       *string
	Denier          *float64
	Filament        *int
	Dozing          *float64
	CC              *string
	CostRateMkt     *float64
	MBSStatus       *string
	MBSLdrPrsn      *float64
	MBSRunLdrPct    *float64
	MBSFinalProduct *string
	// LDRIsFixed / DozingIsFixed mark a value as human-entered actual (recalc rule #3).
	// nil leaves the stored marker untouched.
	LDRIsFixed    *bool
	DozingIsFixed *bool
	IsActive      *bool
	// LDRAdjustmentPct sets the manual LDR adjustment. nil leaves it unchanged.
	// Rejected by the domain (ErrLDRLockedActual) if the spin is locked as Actual.
	LDRAdjustmentPct *float64
	// LDRLockActual is a lock/unlock instruction: true locks as Actual, false
	// unlocks, nil is a no-op. Applied BEFORE LDRAdjustmentPct in the same call,
	// so a single request may unlock and then set a new adjustment together.
	LDRLockActual *bool
	// OrionItemCode is a NEW ERP item code for this spin.
	//
	// ⚠ Today it is always nil: UpdateMBSpinRequest carries no such field, so the
	// delivery layer never sets it. It is wired anyway so the uniqueness gate
	// exists on this path from the start rather than being retrofitted the day
	// the proto field appears.
	OrionItemCode *string
	// VSNumber sets the VS reference number. nil leaves it unchanged.
	VSNumber  *string
	UpdatedBy string
}

// UpdateHandler handles the UpdateMBSpin command.
type UpdateHandler struct {
	repo   mbspin.Repository
	recalc *RecalcService
}

// NewUpdateHandler creates a new UpdateHandler with no child-recalc wiring.
// Updates behave exactly as before: the parent is saved, children are untouched.
func NewUpdateHandler(repo mbspin.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// NewUpdateHandlerWithRecalc creates an UpdateHandler that cascades a dozing
// change to the parent's DIRECT children (A6/A7, one level only — R13).
func NewUpdateHandlerWithRecalc(repo mbspin.Repository, recalc *RecalcService) *UpdateHandler {
	return &UpdateHandler{repo: repo, recalc: recalc}
}

// UpdateResult is the outcome of an update that may have cascaded.
type UpdateResult struct {
	// Entity is the saved parent spin.
	Entity *mbspin.Entity
	// Recalc is the child-recalc outcome, nil when no recalc ran — either
	// because no recalc-triggering field changed, or because this handler was
	// built without recalc wiring.
	Recalc *RecalcResult
}

// Handle executes the update MB Spin command and returns the saved entity.
// Kept for callers that do not care about the cascade.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*mbspin.Entity, error) {
	res, err := h.HandleWithRecalc(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return res.Entity, nil
}

// HandleWithRecalc executes the update and, when a recalc-triggering field
// actually CHANGED, cascades the new dozing to the parent's direct children.
//
// ⛔ The trigger is narrow ON PURPOSE: only mbs_denier, mbs_filament and
// mbs_dozing arm it, and only when the incoming value DIFFERS from the stored
// one. Renaming a spin, flipping is_active, editing cc / cost_rate_mkt /
// status / final_product — none of those recalculate anything.
//
// ⛔ ONE LEVEL (R13): the cascade calls RecalcService.Apply exactly once, for
// this parent. It never walks into grandchildren.
//
// ⛔ ZERO yarn products (D24): the cascade stops at the child spin.
func (h *UpdateHandler) HandleWithRecalc(ctx context.Context, cmd UpdateCommand) (*UpdateResult, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := ensureOrionItemCodeUnique(ctx, h.repo, cmd.OrionItemCode, entity.OrionItemCode()); err != nil {
		return nil, err
	}

	// Snapshot BEFORE the mutation: the decision "did a recalc-triggering field
	// change" cannot be made after entity.Update has overwritten the values.
	triggers := recalcTriggered(entity, cmd)
	oldDozing := copyFloat(entity.Dozing())

	if err := entity.Update(mbspin.UpdateInput{
		MgtName:         cmd.MgtName,
		MBCosting:       cmd.MBCosting,
		Denier:          cmd.Denier,
		Filament:        cmd.Filament,
		Dozing:          cmd.Dozing,
		CC:              cmd.CC,
		CostRateMkt:     cmd.CostRateMkt,
		MBSStatus:       cmd.MBSStatus,
		MBSLdrPrsn:      cmd.MBSLdrPrsn,
		MBSRunLdrPct:    cmd.MBSRunLdrPct,
		MBSFinalProduct: cmd.MBSFinalProduct,
		LDRIsFixed:      cmd.LDRIsFixed,
		DozingIsFixed:   cmd.DozingIsFixed,
		IsActive:        cmd.IsActive,
		VSNumber:        cmd.VSNumber,
	}, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	// Lock/unlock is applied BEFORE the adjustment value on purpose: a single
	// request may carry both an unlock instruction and a new adjustment, and
	// unlocking first is what makes SetLDRAdjustment accept the new value in
	// the same call instead of rejecting it with ErrLDRLockedActual.
	if cmd.LDRLockActual != nil {
		if *cmd.LDRLockActual {
			entity.LockLDRActual()
		} else {
			entity.UnlockLDRActual()
		}
	}
	if cmd.LDRAdjustmentPct != nil {
		if err := entity.SetLDRAdjustment(cmd.LDRAdjustmentPct); err != nil {
			return nil, err
		}
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	res := &UpdateResult{Entity: entity}
	if !triggers || h.recalc == nil {
		return res, nil
	}

	recalcRes, err := h.recalc.Apply(ctx, ApplyInput{
		Parent:    entity,
		OldDozing: oldDozing,
		Actor:     cmd.UpdatedBy,
	})
	if err != nil {
		return nil, err
	}
	res.Recalc = recalcRes
	return res, nil
}

// recalcTriggered reports whether this command CHANGES one of the three fields
// that feed formula C-1 on the children: mbs_denier, mbs_filament, mbs_dozing.
//
// A nil field means "leave unchanged" and never triggers. An identical value
// never triggers either — resubmitting an unchanged form must not rewrite the
// children's dozing or burn an audit row.
func recalcTriggered(current *mbspin.Entity, cmd UpdateCommand) bool {
	if cmd.Denier != nil && !sameFloat(current.Denier(), cmd.Denier) {
		return true
	}
	if cmd.Filament != nil && !sameInt(current.Filament(), cmd.Filament) {
		return true
	}
	if cmd.Dozing != nil && !sameFloat(current.Dozing(), cmd.Dozing) {
		return true
	}
	return false
}

// sameFloat compares two optional floats; a nil stored value differs from any
// supplied value (filling in a blank IS a change).
func sameFloat(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// copyFloat detaches an optional float from the entity so the "old" value
// survives the in-place mutation that follows.
func copyFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}
