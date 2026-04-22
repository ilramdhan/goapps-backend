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
	Colourant      string
	CIName         string
	CostPercentage float64
	CostPerKg      float64
	CreatedBy      string
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

	exists, err := h.repo.ExistsHeadByCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("check head code uniqueness: %w", err)
	}
	if exists {
		return nil, rmgroup.ErrCodeAlreadyExists
	}

	head, err := rmgroup.NewHead(code, cmd.Name, cmd.Description, cmd.CostPercentage, cmd.CostPerKg, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	// Carry optional text fields and leading colourant/ciName via Update so the
	// single code path enforces validation and audit stamping.
	if cmd.Colourant != "" || cmd.CIName != "" {
		in := rmgroup.UpdateInput{}
		if cmd.Colourant != "" {
			v := cmd.Colourant
			in.Colourant = &v
		}
		if cmd.CIName != "" {
			v := cmd.CIName
			in.CIName = &v
		}
		if err := head.Update(in, cmd.CreatedBy); err != nil {
			return nil, err
		}
	}

	if err := h.repo.CreateHead(ctx, head); err != nil {
		if errors.Is(err, rmgroup.ErrCodeAlreadyExists) {
			return nil, rmgroup.ErrCodeAlreadyExists
		}
		return nil, fmt.Errorf("persist head: %w", err)
	}

	return head, nil
}
