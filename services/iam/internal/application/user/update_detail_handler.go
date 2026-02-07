// Package user provides application layer handlers for User operations.
package user

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// UpdateDetailCommand represents the update user detail command.
type UpdateDetailCommand struct {
	UserID         string
	SectionID      *uuid.UUID
	FullName       *string
	FirstName      *string
	LastName       *string
	Phone          *string
	ProfilePicture *string
	Position       *string
	DateOfBirth    *time.Time
	Address        *string
	ExtraData      map[string]interface{}
	UpdatedBy      string
}

// UpdateDetailHandler handles the update user detail command.
type UpdateDetailHandler struct {
	repo user.Repository
}

// NewUpdateDetailHandler creates a new UpdateDetailHandler.
func NewUpdateDetailHandler(repo user.Repository) *UpdateDetailHandler {
	return &UpdateDetailHandler{repo: repo}
}

// Handle executes the update user detail command.
func (h *UpdateDetailHandler) Handle(ctx context.Context, cmd UpdateDetailCommand) (*user.Detail, error) {
	// 1. Parse user ID.
	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return nil, shared.ErrNotFound
	}

	// 2. Get existing detail.
	detail, err := h.repo.GetDetailByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 3. Update domain entity.
	if err := detail.Update(
		cmd.SectionID,
		cmd.FullName,
		cmd.FirstName,
		cmd.LastName,
		cmd.Phone,
		cmd.ProfilePicture,
		cmd.Position,
		cmd.DateOfBirth,
		cmd.Address,
		cmd.ExtraData,
		cmd.UpdatedBy,
	); err != nil {
		return nil, err
	}

	// 4. Persist.
	if err := h.repo.UpdateDetail(ctx, detail); err != nil {
		return nil, err
	}

	return detail, nil
}
