// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/companymapping"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// RemoveCompanyMappingCommand removes a company mapping from a user.
type RemoveCompanyMappingCommand struct {
	UserID           string
	CompanyMappingID string
}

// RemoveCompanyMappingHandler handles RemoveUserCompanyMapping commands.
type RemoveCompanyMappingHandler struct {
	repo companymapping.Repository
}

// NewRemoveCompanyMappingHandler returns a new handler.
func NewRemoveCompanyMappingHandler(repo companymapping.Repository) *RemoveCompanyMappingHandler {
	return &RemoveCompanyMappingHandler{repo: repo}
}

// Handle executes the command.
func (h *RemoveCompanyMappingHandler) Handle(ctx context.Context, cmd RemoveCompanyMappingCommand) error {
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return shared.ErrNotFound
	}
	mappingID, err := uuid.Parse(cmd.CompanyMappingID)
	if err != nil {
		return shared.ErrNotFound
	}
	return h.repo.RemoveFromUser(ctx, userID, mappingID)
}
