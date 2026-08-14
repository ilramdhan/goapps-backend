// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// UpdateCommand represents the update MB Head command.
//
// Per the Phase 2 proto change the 12 required fields lost explicit presence and now arrive
// full-replace: they are plain values, always supplied, never a "leave alone" signal. The
// remaining fields stay pointer-typed and keep patch semantics. Shades are replace-on-save —
// an empty slice clears all children (spec section 4.4).
type UpdateCommand struct {
	ID              uuid.UUID
	MBCosting       *string
	MgtName         string
	Denier          float64
	Filament        int
	Dozing          *float64
	MBHCheckStatus  *string
	MBHStatus       *string
	MBHLdrPrsn      float64
	MBHFinalProduct string
	MBHCode         *string
	IsActive        *bool
	DevCode         string
	VsNumber        string
	NoOfProcess     string
	ShadeCode       string
	ShadeName       string
	CrossSection    string
	LustureCode     *string
	MachineID       *uuid.UUID
	Shades          []ShadeInput
	UpdatedBy       string
}

// UpdateHandler handles the UpdateMBHead command.
type UpdateHandler struct {
	repo        mbhead.Repository
	noOfProcess noOfProcessValidator
}

// NewUpdateHandler creates a new UpdateHandler. paramRepo backs the no-of-process membership
// check against the live mst_mb_param_option set (spec section 2.3).
func NewUpdateHandler(repo mbhead.Repository, paramRepo mbparam.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo, noOfProcess: noOfProcessValidator{paramRepo: paramRepo}}
}

// Handle executes the update MB Head command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(mbhead.UpdateInput{
		MBCosting:       cmd.MBCosting,
		MgtName:         &cmd.MgtName,
		Denier:          &cmd.Denier,
		Filament:        &cmd.Filament,
		Dozing:          cmd.Dozing,
		MBHCheckStatus:  cmd.MBHCheckStatus,
		MBHStatus:       cmd.MBHStatus,
		MBHLdrPrsn:      &cmd.MBHLdrPrsn,
		MBHFinalProduct: &cmd.MBHFinalProduct,
		MBHCode:         cmd.MBHCode,
		IsActive:        cmd.IsActive,
		DevCode:         &cmd.DevCode,
		VsNumber:        &cmd.VsNumber,
		NoOfProcess:     &cmd.NoOfProcess,
		ShadeCode:       &cmd.ShadeCode,
		ShadeName:       &cmd.ShadeName,
		CrossSection:    &cmd.CrossSection,
		LustureCode:     cmd.LustureCode,
		MachineID:       cmd.MachineID,
	}, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	shades, err := buildShades(entity, cmd.Shades, cmd.UpdatedBy)
	if err != nil {
		return nil, err
	}

	if err := h.noOfProcess.validate(ctx, cmd.NoOfProcess); err != nil {
		return nil, err
	}

	if err := checkMBHeadUniqueness(ctx, h.repo, cmd.DevCode, cmd.VsNumber, &cmd.ID); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	if err := h.repo.ReplaceShades(ctx, entity.ID(), shades, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	return entity, nil
}
