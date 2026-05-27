// Package rmgroup provides application layer handlers for RM group head and detail operations.
package rmgroup

import (
	"context"
	"errors"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// CreateCommand carries the inputs for creating a new RM group head.
// Flag selectors default to CONS at the domain layer and can be updated afterwards.
type CreateCommand struct {
	Code           string
	Name           string
	Description    string
	Colorant       string
	CIName         string
	CostPercentage float64
	CostPerKg      float64
	CreatedBy      string
	// V2 optional marketing inputs.
	MarketingFreightRate    *float64
	MarketingAntiDumpingPct *float64
	MarketingDefaultValue   *float64
	ValuationFlag           string // "" / "AUTO" / "CR" / ...
	MarketingFlag           string
}

// CreateHandler handles CreateHead commands.
type CreateHandler struct {
	repo rmgroup.Repository
}

// NewCreateHandler builds a CreateHandler.
func NewCreateHandler(repo rmgroup.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle validates the command, ensures the code is unique, and persists a new head.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*rmgroup.Head, error) {
	code, err := rmgroup.NewCode(cmd.Code)
	if err != nil {
		return nil, err
	}
	if err := h.assertCodeUnique(ctx, code); err != nil {
		return nil, err
	}
	head, err := rmgroup.NewHead(code, cmd.Name, cmd.Description, cmd.CostPercentage, cmd.CostPerKg, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := applyCreateOptionalText(head, cmd); err != nil {
		return nil, err
	}
	if err := applyCreateMarketingInputs(head, cmd); err != nil {
		return nil, err
	}
	if err := h.repo.CreateHead(ctx, head); err != nil {
		if errors.Is(err, rmgroup.ErrCodeAlreadyExists) {
			return nil, rmgroup.ErrCodeAlreadyExists
		}
		return nil, fmt.Errorf("persist head: %w", err)
	}
	return head, nil
}

func (h *CreateHandler) assertCodeUnique(ctx context.Context, code rmgroup.Code) error {
	exists, err := h.repo.ExistsHeadByCode(ctx, code)
	if err != nil {
		return fmt.Errorf("check head code uniqueness: %w", err)
	}
	if exists {
		return rmgroup.ErrCodeAlreadyExists
	}
	return nil
}

// applyCreateOptionalText carries optional colorant / ciName via Update so the
// single code path enforces validation and audit stamping.
func applyCreateOptionalText(head *rmgroup.Head, cmd CreateCommand) error {
	if cmd.Colorant == "" && cmd.CIName == "" {
		return nil
	}
	in := rmgroup.UpdateInput{}
	if cmd.Colorant != "" {
		v := cmd.Colorant
		in.Colorant = &v
	}
	if cmd.CIName != "" {
		v := cmd.CIName
		in.CIName = &v
	}
	return head.Update(in, cmd.CreatedBy)
}

func hasV2CreateMarketingPatch(cmd CreateCommand) bool {
	return cmd.MarketingFreightRate != nil || cmd.MarketingAntiDumpingPct != nil ||
		cmd.MarketingDefaultValue != nil || cmd.ValuationFlag != "" || cmd.MarketingFlag != ""
}

func applyCreateMarketingInputs(head *rmgroup.Head, cmd CreateCommand) error {
	if !hasV2CreateMarketingPatch(cmd) {
		return nil
	}
	valFlag, err := rmgroup.ParseValuationFlag(cmd.ValuationFlag)
	if err != nil {
		return err
	}
	mktFlag, err := rmgroup.ParseMarketingFlag(cmd.MarketingFlag)
	if err != nil {
		return err
	}
	return head.AttachMarketingInputs(rmgroup.MarketingInputs{
		FreightRate:    cmd.MarketingFreightRate,
		AntiDumpingPct: cmd.MarketingAntiDumpingPct,
		DefaultValue:   cmd.MarketingDefaultValue,
		ValuationFlag:  valFlag,
		MarketingFlag:  mktFlag,
	})
}
