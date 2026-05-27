// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/companymapping"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// UpdateCommand represents the update user command.
type UpdateCommand struct {
	UserID           string
	Email            *string
	IsActive         *bool
	EmployeeLevelID  *string
	EmployeeGroupID  *string
	CompanyMappingID *string
	UpdatedBy        string
}

// UpdateHandler handles the update user command.
type UpdateHandler struct {
	repo        user.Repository
	mappingRepo companymapping.Repository // optional
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo user.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// NewUpdateHandlerWithMapping creates an UpdateHandler that can also assign a
// primary company mapping.
func NewUpdateHandlerWithMapping(repo user.Repository, mappingRepo companymapping.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo, mappingRepo: mappingRepo}
}

// Handle executes the update user command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*user.User, error) {
	id, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, shared.ErrNotFound
	}

	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if cmd.Email != nil {
		existingByEmail, gErr := h.repo.GetByEmail(ctx, *cmd.Email)
		if gErr == nil && existingByEmail.ID() != entity.ID() {
			return nil, shared.ErrAlreadyExists
		}
	}

	if err := entity.Update(cmd.Email, cmd.IsActive, cmd.UpdatedBy); err != nil {
		return nil, err
	}

	if err := h.applyEmployeeRefs(entity, cmd); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	if err := h.assignPrimaryMapping(ctx, entity.ID(), cmd); err != nil {
		return nil, err
	}

	return entity, nil
}

func (h *UpdateHandler) applyEmployeeRefs(entity *user.User, cmd UpdateCommand) error {
	if cmd.EmployeeLevelID != nil {
		levelID, err := parseOptionalEmployeeID(*cmd.EmployeeLevelID)
		if err != nil {
			return err
		}
		if err := entity.SetEmployeeLevel(levelID, cmd.UpdatedBy); err != nil {
			return err
		}
	}
	if cmd.EmployeeGroupID != nil {
		groupID, err := parseOptionalEmployeeID(*cmd.EmployeeGroupID)
		if err != nil {
			return err
		}
		if err := entity.SetEmployeeGroup(groupID, cmd.UpdatedBy); err != nil {
			return err
		}
	}
	return nil
}

// parseOptionalEmployeeID parses an optional UUID string. Empty string clears
// the reference (returns nil pointer + nil error).
func parseOptionalEmployeeID(s string) (*uuid.UUID, error) {
	if s == "" {
		return nil, nil //nolint:nilnil // intentional clear sentinel
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (h *UpdateHandler) assignPrimaryMapping(ctx context.Context, userID uuid.UUID, cmd UpdateCommand) error {
	if h.mappingRepo == nil || cmd.CompanyMappingID == nil || *cmd.CompanyMappingID == "" {
		return nil
	}
	mappingID, err := uuid.Parse(*cmd.CompanyMappingID)
	if err != nil {
		return err
	}
	return h.mappingRepo.AssignToUser(ctx, userID, mappingID, true, cmd.UpdatedBy)
}
