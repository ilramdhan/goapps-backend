// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/companymapping"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// AssignCompanyMappingCommand assigns a company mapping to a user.
type AssignCompanyMappingCommand struct {
	UserID           string
	CompanyMappingID string
	IsPrimary        bool
	AssignedBy       string
}

// AssignCompanyMappingHandler handles AssignUserCompanyMapping commands.
type AssignCompanyMappingHandler struct {
	repo companymapping.Repository
}

// NewAssignCompanyMappingHandler returns a new handler.
func NewAssignCompanyMappingHandler(repo companymapping.Repository) *AssignCompanyMappingHandler {
	return &AssignCompanyMappingHandler{repo: repo}
}

// Handle executes the command.
func (h *AssignCompanyMappingHandler) Handle(ctx context.Context, cmd AssignCompanyMappingCommand) error {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return shared.ErrNotFound
	}
	mappingID, err := uuid.Parse(cmd.CompanyMappingID)
	if err != nil {
		return shared.ErrNotFound
	}
	return h.repo.AssignToUser(ctx, userID, mappingID, cmd.IsPrimary, cmd.AssignedBy)
}
