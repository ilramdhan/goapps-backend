// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/companymapping"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// CreateCommand represents the create user command.
type CreateCommand struct {
	Username         string
	Email            string
	PasswordHash     string
	EmployeeCode     string
	FullName         string
	FirstName        string
	LastName         string
	Phone            string
	Position         string
	Address          string
	EmployeeLevelID  *string
	EmployeeGroupID  *string
	CompanyMappingID *string
	CreatedBy        string
}

// CreateHandler handles the create user command.
type CreateHandler struct {
	repo        user.Repository
	mappingRepo companymapping.Repository // optional; nil disables primary-mapping assignment
}

// NewCreateHandler creates a new CreateHandler. mappingRepo can be nil for
// callers that do not need primary mapping assignment on create.
func NewCreateHandler(repo user.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// NewCreateHandlerWithMapping creates a CreateHandler that can also assign a
// primary company mapping during user creation.
func NewCreateHandlerWithMapping(repo user.Repository, mappingRepo companymapping.Repository) *CreateHandler {
	return &CreateHandler{repo: repo, mappingRepo: mappingRepo}
}

// Handle executes the create user command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*user.User, error) {
	if err := h.checkDuplicates(ctx, cmd); err != nil {
		return nil, err
	}
	if cmd.PasswordHash == "" {
		return nil, user.ErrEmptyPassword
	}

	entity, err := user.NewUser(cmd.Username, cmd.Email, cmd.PasswordHash, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	if err := h.applyEmployeeRefs(entity, cmd); err != nil {
		return nil, err
	}

	detail, err := user.NewDetail(
		entity.ID(), nil,
		cmd.EmployeeCode, cmd.FullName, cmd.FirstName, cmd.LastName,
		cmd.Phone, cmd.Position, cmd.Address, cmd.CreatedBy,
	)
	if err != nil {
		return nil, err
	}

	if err := h.repo.Create(ctx, entity, detail); err != nil {
		return nil, err
	}

	if err := h.assignPrimaryMapping(ctx, entity.ID(), cmd); err != nil {
		return nil, err
	}

	return entity, nil
}

func (h *CreateHandler) checkDuplicates(ctx context.Context, cmd CreateCommand) error {
	existsUsername, err := h.repo.ExistsByUsername(ctx, cmd.Username)
	if err != nil {
		return err
	}
	if existsUsername {
		return shared.ErrAlreadyExists
	}
	existsEmail, err := h.repo.ExistsByEmail(ctx, cmd.Email)
	if err != nil {
		return err
	}
	if existsEmail {
		return shared.ErrAlreadyExists
	}
	if cmd.EmployeeCode == "" {
		return nil
	}
	existsCode, err := h.repo.ExistsByEmployeeCode(ctx, cmd.EmployeeCode)
	if err != nil {
		return err
	}
	if existsCode {
		return shared.ErrAlreadyExists
	}
	return nil
}

func (h *CreateHandler) applyEmployeeRefs(entity *user.User, cmd CreateCommand) error {
	if cmd.EmployeeLevelID != nil && *cmd.EmployeeLevelID != "" {
		id, err := uuid.Parse(*cmd.EmployeeLevelID)
		if err != nil {
			return err
		}
		if err := entity.SetEmployeeLevel(&id, ""); err != nil {
			return err
		}
	}
	if cmd.EmployeeGroupID != nil && *cmd.EmployeeGroupID != "" {
		id, err := uuid.Parse(*cmd.EmployeeGroupID)
		if err != nil {
			return err
		}
		if err := entity.SetEmployeeGroup(&id, ""); err != nil {
			return err
		}
	}
	return nil
}

func (h *CreateHandler) assignPrimaryMapping(ctx context.Context, userID uuid.UUID, cmd CreateCommand) error {
	if h.mappingRepo == nil || cmd.CompanyMappingID == nil || *cmd.CompanyMappingID == "" {
		return nil
	}
	mappingID, err := uuid.Parse(*cmd.CompanyMappingID)
	if err != nil {
		return err
	}
	return h.mappingRepo.AssignToUser(ctx, userID, mappingID, true, cmd.CreatedBy)
}
