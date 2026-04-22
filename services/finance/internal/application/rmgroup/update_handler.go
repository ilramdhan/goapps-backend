// Package rmgroup provides application layer handlers for RM group head and detail operations.
package rmgroup

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// UpdateCommand is the partial-update command for a head. Pointer fields stay nil
// when the caller wants to leave them unchanged. The three ClearInitVal flags
// force the corresponding init_val columns to NULL.
type UpdateCommand struct {
	HeadID string

	Name           *string
	Description    *string
	Colorant       *string
	CIName         *string
	CostPercentage *float64
	CostPerKg      *float64

	FlagValuation  *string
	FlagMarketing  *string
	FlagSimulation *string

	InitValValuation  *float64
	InitValMarketing  *float64
	InitValSimulation *float64

	ClearInitValValuation  bool
	ClearInitValMarketing  bool
	ClearInitValSimulation bool

	IsActive *bool

	UpdatedBy string
}

// UpdateHandler handles UpdateHead commands.
type UpdateHandler struct {
	repo rmgroup.Repository
}

// NewUpdateHandler builds an UpdateHandler.
func NewUpdateHandler(repo rmgroup.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle parses the ID, loads the head, applies the partial update, and persists.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*rmgroup.Head, error) {
	id, err := uuid.Parse(cmd.HeadID)
	if err != nil {
		return nil, rmgroup.ErrNotFound
	}

	head, err := h.repo.GetHeadByID(ctx, id)
	if err != nil {
		return nil, err
	}

	in, err := buildHeadUpdateInput(cmd)
	if err != nil {
		return nil, err
	}
	if err := head.Update(in, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.repo.UpdateHead(ctx, head); err != nil {
		return nil, fmt.Errorf("persist head update: %w", err)
	}
	return head, nil
}

// buildHeadUpdateInput maps command pointers to the domain UpdateInput, parsing
// the three optional flag strings into typed Flag values.
func buildHeadUpdateInput(cmd UpdateCommand) (rmgroup.UpdateInput, error) {
	in := rmgroup.UpdateInput{
		Name:                   cmd.Name,
		Description:            cmd.Description,
		Colorant:               cmd.Colorant,
		CIName:                 cmd.CIName,
		CostPercentage:         cmd.CostPercentage,
		CostPerKg:              cmd.CostPerKg,
		InitValValuation:       cmd.InitValValuation,
		InitValMarketing:       cmd.InitValMarketing,
		InitValSimulation:      cmd.InitValSimulation,
		ClearInitValValuation:  cmd.ClearInitValValuation,
		ClearInitValMarketing:  cmd.ClearInitValMarketing,
		ClearInitValSimulation: cmd.ClearInitValSimulation,
		IsActive:               cmd.IsActive,
	}

	if err := assignFlag(&in.FlagValuation, cmd.FlagValuation); err != nil {
		return in, err
	}
	if err := assignFlag(&in.FlagMarketing, cmd.FlagMarketing); err != nil {
		return in, err
	}
	if err := assignFlag(&in.FlagSimulation, cmd.FlagSimulation); err != nil {
		return in, err
	}
	return in, nil
}

func assignFlag(target **rmgroup.Flag, raw *string) error {
	if raw == nil {
		return nil
	}
	flag, err := rmgroup.ParseFlag(*raw)
	if err != nil {
		return err
	}
	*target = &flag
	return nil
}
