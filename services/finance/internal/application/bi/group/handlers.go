// Package group provides application-layer handlers for BI DashboardGroup CRUD.
package group

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	groupdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/group"
)

// CreateCommand is the payload for CreateHandler.
type CreateCommand struct {
	Code         string
	Name         string
	Description  string
	Icon         string
	DisplayOrder int
	IsActive     bool
	CreatedBy    uuid.UUID
}

// CreateHandler creates a new group.
type CreateHandler struct{ repo groupdomain.Repository }

// NewCreateHandler constructs a CreateHandler.
func NewCreateHandler(r groupdomain.Repository) *CreateHandler { return &CreateHandler{repo: r} }

// Handle executes the create-group use case.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*groupdomain.Group, error) {
	g, err := groupdomain.NewGroup(groupdomain.NewGroupParams{
		Code: cmd.Code, Name: cmd.Name, Description: cmd.Description, Icon: cmd.Icon,
		DisplayOrder: cmd.DisplayOrder, IsActive: cmd.IsActive, CreatedBy: cmd.CreatedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("validate group: %w", err)
	}
	if err := h.repo.Create(ctx, g); err != nil {
		return nil, fmt.Errorf("persist group: %w", err)
	}
	return g, nil
}

// ListHandler lists groups.
type ListHandler struct{ repo groupdomain.Repository }

// NewListHandler constructs a ListHandler.
func NewListHandler(r groupdomain.Repository) *ListHandler { return &ListHandler{repo: r} }

// Handle lists groups.
func (h *ListHandler) Handle(ctx context.Context, includeInactive bool) ([]*groupdomain.Group, error) {
	return h.repo.List(ctx, includeInactive)
}

// UpdateCommand is the payload for UpdateHandler.
type UpdateCommand struct {
	ID           uuid.UUID
	Name         *string
	Description  *string
	Icon         *string
	DisplayOrder *int
	IsActive     *bool
	UpdatedBy    uuid.UUID
}

// UpdateHandler mutates a group.
type UpdateHandler struct{ repo groupdomain.Repository }

// NewUpdateHandler constructs an UpdateHandler.
func NewUpdateHandler(r groupdomain.Repository) *UpdateHandler { return &UpdateHandler{repo: r} }

// Handle executes the update.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*groupdomain.Group, error) {
	g, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := g.Update(groupdomain.UpdateParams{
		Name: cmd.Name, Description: cmd.Description, Icon: cmd.Icon,
		DisplayOrder: cmd.DisplayOrder, IsActive: cmd.IsActive, UpdatedBy: cmd.UpdatedBy,
	}); err != nil {
		return nil, fmt.Errorf("apply update: %w", err)
	}
	if err := h.repo.Update(ctx, g); err != nil {
		return nil, fmt.Errorf("persist update: %w", err)
	}
	return g, nil
}

// DeleteHandler removes a group (refuses when in use).
type DeleteHandler struct{ repo groupdomain.Repository }

// NewDeleteHandler constructs a DeleteHandler.
func NewDeleteHandler(r groupdomain.Repository) *DeleteHandler { return &DeleteHandler{repo: r} }

// Handle executes the delete.
func (h *DeleteHandler) Handle(ctx context.Context, id uuid.UUID) error {
	return h.repo.Delete(ctx, id)
}
