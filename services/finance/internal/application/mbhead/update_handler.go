// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// UpdateCommand represents the update MB Head command.
type UpdateCommand struct {
	ID        uuid.UUID
	MBCosting *string
	MgtName   *string
	Denier    *float64
	Filament  *int
	Dozing    *float64
	// ⛔ MBHCheckStatus removed (§11 item 106): the frozen Oracle trace is not a
	// writable command field on any path. The gRPC layer rejects requests carrying it.
	MBHStatus       *string
	MBHLdrPrsn      *float64
	MBHRunLdrPct    *float64
	MBHFinalProduct *string
	MBHCode         *string
	IsActive        *bool
	DevCode         *string
	ShadeCode       *string
	ShadeName       *string
	CrossSection    *string
	LustureCode     *string
	MachineID       *uuid.UUID
	UpdatedBy       string

	// VSNumber / NoOfProcess: nil means "absent from the payload" and leaves the
	// stored value untouched (D13). ⛔ No default is applied on update either — the
	// no_of_process default is still an open user decision (U-B, plan §11 item 70).
	VSNumber    *string
	NoOfProcess *string

	// AdditionalShades is only consulted when ReplaceAdditionalShades is true.
	AdditionalShades []mbhead.Shade

	// ReplaceAdditionalShades is the explicit opt-in for rewriting the shade rows.
	//
	// 🔴 When false or absent the stored shades are LEFT ALONE. Without this gate
	// the legacy edit form — which never sends additional_shades — would wipe every
	// shade row on each save.
	ReplaceAdditionalShades bool
}

// UpdateHandler handles the UpdateMBHead command.
type UpdateHandler struct {
	repo mbhead.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo mbhead.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update MB Head command.
//
// ⛔ The domain's recipe required-field check is NOT invoked here (nor anywhere
// else): 573 legacy heads still carry a NULL cross-section column (plan K6) and
// must remain editable. Required-field enforcement is blocked behind the P4
// backfill.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := h.assertVSNumberFree(ctx, entity, cmd.VSNumber); err != nil {
		return nil, err
	}

	if err := h.assertDevCodeFree(ctx, entity, cmd.DevCode); err != nil {
		return nil, err
	}

	oldCosting := entity.MBCosting()
	costingChanged := mbCostingChanged(oldCosting, cmd.MBCosting)
	mbCosting := cmd.MBCosting
	if costingChanged {
		if err := h.assertMBCostingFree(ctx, *cmd.MBCosting); err != nil {
			return nil, err
		}
	} else {
		// A trim-equal value is a no-op: never let a whitespace-only edit drift the
		// head away from the cost product / spin rows that copied its costing.
		mbCosting = nil
	}

	if err := entity.Update(mbhead.UpdateInput{
		MBCosting:       mbCosting,
		MgtName:         cmd.MgtName,
		Denier:          cmd.Denier,
		Filament:        cmd.Filament,
		Dozing:          cmd.Dozing,
		MBHStatus:       cmd.MBHStatus,
		MBHLdrPrsn:      cmd.MBHLdrPrsn,
		MBHRunLdrPct:    cmd.MBHRunLdrPct,
		MBHFinalProduct: cmd.MBHFinalProduct,
		MBHCode:         cmd.MBHCode,
		IsActive:        cmd.IsActive,
		DevCode:         cmd.DevCode,
		ShadeCode:       cmd.ShadeCode,
		ShadeName:       cmd.ShadeName,
		CrossSection:    cmd.CrossSection,
		LustureCode:     cmd.LustureCode,
		MachineID:       cmd.MachineID,
		VSNumber:        cmd.VSNumber,
		NoOfProcess:     cmd.NoOfProcess,
	}, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if cmd.ReplaceAdditionalShades {
		if err := entity.SetAdditionalShades(cmd.AdditionalShades); err != nil {
			return nil, err
		}
	}

	if costingChanged {
		// The rename must reach the auto-generated cost product name and the root
		// MB Spin rows in the same transaction as the head itself.
		if err := h.repo.UpdateWithCostingRename(ctx, entity, oldCosting); err != nil {
			return nil, err
		}
	} else if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	if cmd.ReplaceAdditionalShades {
		if err := h.repo.ReplaceShades(ctx, entity.ID(), entity.AdditionalShades(), cmd.UpdatedBy); err != nil {
			return nil, err
		}
	}

	return entity, nil
}

// mbCostingChanged reports whether an incoming MB Costing really renames the head.
// nil (field absent) and a value equal to the stored one after trimming are no-ops.
func mbCostingChanged(current string, incoming *string) bool {
	if incoming == nil {
		return false
	}
	return strings.TrimSpace(*incoming) != strings.TrimSpace(current)
}

// assertMBCostingFree rejects a rename onto an MB Costing another live head already
// uses (unique index uix_mst_mb_head_mb_costing). Only called when the value changes,
// so the head's own row can never collide with itself.
func (h *UpdateHandler) assertMBCostingFree(ctx context.Context, incoming string) error {
	taken, err := h.repo.ExistsByMBCosting(ctx, incoming)
	if err != nil {
		return err
	}
	if taken {
		return mbhead.ErrAlreadyExists
	}
	return nil
}

// assertVSNumberFree checks VS Number uniqueness ONLY when the value actually
// changes (B9). An unchanged value is never re-checked, so the 177 legacy heads
// carrying '0' — and the two sharing '16728' — stay editable. '0' and "" are
// exempt outright. See CreateHandler.assertVSNumberFree for the full rationale.
func (h *UpdateHandler) assertVSNumberFree(ctx context.Context, entity *mbhead.Entity, incoming *string) error {
	if !vsNumberNeedsUniquenessCheck(incoming) {
		return nil
	}
	if current := entity.VSNumber(); current != nil && *current == *incoming {
		return nil
	}
	taken, err := h.repo.ExistsByVSNumber(ctx, *incoming, entity.ID())
	if err != nil {
		return err
	}
	if taken {
		return mbhead.ErrDuplicateVSNumber
	}
	return nil
}

// assertDevCodeFree checks Dev Code uniqueness ONLY when the value actually
// changes (U-D, 2026-08-22), mirroring assertVSNumberFree. An unchanged value is
// never re-checked, so a head that already shares a legacy Dev Code with another
// head stays editable — the user's rule is "unique for new data, leave legacy
// duplicates alone". nil (field absent from the payload) and "" are exempt.
func (h *UpdateHandler) assertDevCodeFree(ctx context.Context, entity *mbhead.Entity, incoming *string) error {
	if incoming == nil || !devCodeNeedsUniquenessCheck(*incoming) {
		return nil
	}
	if entity.DevCode() == *incoming {
		return nil
	}
	taken, err := h.repo.ExistsByDevCode(ctx, *incoming, entity.ID())
	if err != nil {
		return err
	}
	if taken {
		return mbhead.ErrDuplicateDevCode
	}
	return nil
}
