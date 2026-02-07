// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// CreateCommand represents the create user command.
type CreateCommand struct {
	Username     string
	Email        string
	PasswordHash string
	EmployeeCode string
	FullName     string
	FirstName    string
	LastName     string
	CreatedBy    string
}

// CreateHandler handles the create user command.
type CreateHandler struct {
	repo user.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo user.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create user command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*user.User, error) {
	// 1. Check for duplicate username.
	existsUsername, err := h.repo.ExistsByUsername(ctx, cmd.Username)
	if err != nil {
		return nil, err
	}
	if existsUsername {
		return nil, shared.ErrAlreadyExists
	}

	// 2. Check for duplicate email.
	existsEmail, err := h.repo.ExistsByEmail(ctx, cmd.Email)
	if err != nil {
		return nil, err
	}
	if existsEmail {
		return nil, shared.ErrAlreadyExists
	}

	// 3. Check for duplicate employee code.
	if cmd.EmployeeCode != "" {
		existsCode, err := h.repo.ExistsByEmployeeCode(ctx, cmd.EmployeeCode)
		if err != nil {
			return nil, err
		}
		if existsCode {
			return nil, shared.ErrAlreadyExists
		}
	}

	// 4. Validate password hash is not empty (hashing should be done upstream).
	if cmd.PasswordHash == "" {
		return nil, user.ErrEmptyPassword
	}

	// 5. Create user domain entity.
	entity, err := user.NewUser(cmd.Username, cmd.Email, cmd.PasswordHash, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}

	// 6. Create user detail domain entity.
	detail, err := user.NewDetail(
		entity.ID(),
		nil,
		cmd.EmployeeCode,
		cmd.FullName,
		cmd.FirstName,
		cmd.LastName,
		cmd.CreatedBy,
	)
	if err != nil {
		return nil, err
	}

	// 7. Persist user and detail together.
	if err := h.repo.Create(ctx, entity, detail); err != nil {
		return nil, err
	}

	return entity, nil
}
