// Package superbacostsp provides application layer handlers for the Superba Cost SP master.
package superbacostsp

import (
	"context"
	"errors"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/superbacostsp"
)

// CreateCommand is the create (MANUAL row) command.
type CreateCommand struct {
	LegacySysID int64
	ShadeCode   string
	ColourName  *string
	OldValue    float64
	NewValue    *float64
	IsActive    bool
	CreatedBy   string
}

// CreateHandler handles CreateSuperbaCostSp.
type CreateHandler struct{ repo domain.Repository }

// NewCreateHandler creates a CreateHandler.
func NewCreateHandler(repo domain.Repository) *CreateHandler { return &CreateHandler{repo: repo} }

// Handle executes the command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*domain.Entry, error) {
	existing, err := h.repo.GetByLegacySysID(ctx, cmd.LegacySysID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		return nil, domain.ErrDuplicateLegacySysID
	}
	entity, err := domain.New(domain.NewParams{
		LegacySysID: cmd.LegacySysID, ShadeCode: cmd.ShadeCode, ColourName: cmd.ColourName,
		OldValue: cmd.OldValue, NewValue: cmd.NewValue, IsActive: cmd.IsActive, CreatedBy: cmd.CreatedBy,
	})
	if err != nil {
		return nil, err
	}
	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}
