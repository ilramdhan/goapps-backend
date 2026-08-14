// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// ShadeInput is one desired additional shade supplied on create or update. The header carries
// shade #1, so SeqNo is 2 or 3 (spec section 4.2).
type ShadeInput struct {
	SeqNo     int32
	ShadeCode string
	ShadeName string
}

// CreateCommand represents the create MB Head command. The 11 fields required by spec
// section 2.1 are plain values: omitting one is an error, not a "leave unset" signal.
type CreateCommand struct {
	MBCosting       string
	OracleSysID     *string
	MgtName         string
	Denier          float64
	Filament        int
	Dozing          *float64
	MBHCheckStatus  *string
	MBHStatus       *string
	MBHLdrPrsn      float64
	MBHFinalProduct string
	MBHCode         *string
	CreatedBy       string
	IsBoughtout     bool
	DevCode         string
	VsNumber        string
	NoOfProcess     string
	ShadeCode       string
	ShadeName       string
	CrossSection    string
	LustureCode     string
	MachineID       *uuid.UUID
	Shades          []ShadeInput
}

// CreateHandler handles the CreateMBHead command.
type CreateHandler struct {
	repo        mbhead.Repository
	noOfProcess noOfProcessValidator
}

// NewCreateHandler creates a new CreateHandler. paramRepo backs the no-of-process membership
// check against the live mst_mb_param_option set (spec section 2.3).
func NewCreateHandler(repo mbhead.Repository, paramRepo mbparam.Repository) *CreateHandler {
	return &CreateHandler{repo: repo, noOfProcess: noOfProcessValidator{paramRepo: paramRepo}}
}

// Handle executes the create MB Head command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*mbhead.Entity, error) {
	exists, err := h.repo.ExistsByMBCosting(ctx, cmd.MBCosting)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, mbhead.ErrAlreadyExists
	}

	entity, err := mbhead.New(mbhead.NewInput{
		MBCosting:      cmd.MBCosting,
		MgtName:        cmd.MgtName,
		DevCode:        cmd.DevCode,
		VsNumber:       cmd.VsNumber,
		NoOfProcess:    cmd.NoOfProcess,
		ShadeCode:      cmd.ShadeCode,
		ShadeName:      cmd.ShadeName,
		CrossSection:   cmd.CrossSection,
		FinalProduct:   cmd.MBHFinalProduct,
		Denier:         cmd.Denier,
		Filament:       cmd.Filament,
		LdrPrsn:        cmd.MBHLdrPrsn,
		OracleSysID:    cmd.OracleSysID,
		Dozing:         cmd.Dozing,
		MBHCheckStatus: cmd.MBHCheckStatus,
		MBHStatus:      cmd.MBHStatus,
		MBHCode:        cmd.MBHCode,
		LustureCode:    cmd.LustureCode,
		MachineID:      cmd.MachineID,
		CreatedBy:      cmd.CreatedBy,
		IsBoughtout:    cmd.IsBoughtout,
	})
	if err != nil {
		return nil, err
	}

	shades, err := buildShades(entity, cmd.Shades, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	if err := h.noOfProcess.validate(ctx, cmd.NoOfProcess); err != nil {
		return nil, err
	}

	if err := checkMBHeadUniqueness(ctx, h.repo, cmd.DevCode, cmd.VsNumber, nil); err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	if len(shades) > 0 {
		if err := h.repo.ReplaceShades(ctx, entity.ID(), shades, cmd.CreatedBy); err != nil {
			return nil, err
		}
	}

	return entity, nil
}

// checkMBHeadUniqueness runs the application-layer dev-code and vs-number pre-checks (spec
// section 3.2), shared by the create and update handlers. excludeID omits the row being
// updated so it cannot collide with itself.
func checkMBHeadUniqueness(
	ctx context.Context, repo mbhead.Repository, devCode, vsNumber string, excludeID *uuid.UUID,
) error {
	dup, err := repo.ExistsByDevCode(ctx, devCode, excludeID)
	if err != nil {
		return err
	}
	if dup {
		return mbhead.ErrDevCodeAlreadyExists
	}

	dup, err = repo.ExistsByVsNumber(ctx, vsNumber, excludeID)
	if err != nil {
		return err
	}
	if dup {
		return mbhead.ErrVsNumberAlreadyExists
	}
	return nil
}

// buildShades converts the supplied shade inputs into validated domain children and attaches
// them to the entity. An empty slice clears all children (replace-on-save, spec section 4.4).
func buildShades(entity *mbhead.Entity, inputs []ShadeInput, actorUserID string) ([]*mbhead.Shade, error) {
	shades := make([]*mbhead.Shade, 0, len(inputs))
	for _, in := range inputs {
		s, err := mbhead.NewShade(entity.ID(), in.SeqNo, in.ShadeCode, in.ShadeName, actorUserID)
		if err != nil {
			return nil, err
		}
		shades = append(shades, s)
	}
	if err := entity.ReplaceShades(shades); err != nil {
		return nil, err
	}
	return shades, nil
}
