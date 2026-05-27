// Package user provides application layer handlers for User operations.
package user

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/companymapping"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// GetCompanyMappingsQuery fetches all company mappings assigned to a user.
type GetCompanyMappingsQuery struct {
	UserID string
}

// GetCompanyMappingsResult contains the list of assignments and the primary id.
type GetCompanyMappingsResult struct {
	Assignments []companymapping.UserAssignment
	PrimaryID   *uuid.UUID
}

// GetCompanyMappingsHandler handles GetUserCompanyMappings queries.
type GetCompanyMappingsHandler struct {
	repo companymapping.Repository
}

// NewGetCompanyMappingsHandler returns a new handler.
func NewGetCompanyMappingsHandler(repo companymapping.Repository) *GetCompanyMappingsHandler {
	return &GetCompanyMappingsHandler{repo: repo}
}

// Handle executes the query.
func (h *GetCompanyMappingsHandler) Handle(ctx context.Context, q GetCompanyMappingsQuery) (*GetCompanyMappingsResult, error) {
	userID, err := uuid.Parse(q.UserID)
	if err != nil {
		return nil, shared.ErrNotFound
	}
	assignments, primary, err := h.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &GetCompanyMappingsResult{Assignments: assignments, PrimaryID: primary}, nil
}
