// Package menu provides application layer services for menu management.
package menu

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/menu"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// ErrConflict indicates a duplicate entity.
var ErrConflict = errors.New("menu already exists")

// Service provides menu management operations.
type Service struct {
	menuRepo menu.Repository
}

// NewService creates a new menu service.
func NewService(menuRepo menu.Repository) *Service {
	return &Service{
		menuRepo: menuRepo,
	}
}

// CreateMenuInput represents input for creating a menu.
type CreateMenuInput struct {
	ParentID    *uuid.UUID
	Code        string
	Title       string
	URL         string
	IconName    string
	ServiceName string
	Level       int
	SortOrder   int
	IsVisible   bool
	CreatedBy   string
}

// CreateMenu creates a new menu item.
func (s *Service) CreateMenu(ctx context.Context, input CreateMenuInput) (*menu.Menu, error) {
	// Check if code already exists
	exists, err := s.menuRepo.ExistsByCode(ctx, input.Code)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing menu: %w", err)
	}
	if exists {
		return nil, ErrConflict
	}

	// Verify parent exists if provided
	if input.ParentID != nil {
		_, err := s.menuRepo.GetByID(ctx, *input.ParentID)
		if err != nil {
			return nil, fmt.Errorf("invalid parent menu: %w", err)
		}
	}

	m, err := menu.NewMenu(
		input.ParentID,
		input.Code,
		input.Title,
		input.URL,
		input.IconName,
		input.ServiceName,
		input.Level,
		input.SortOrder,
		input.IsVisible,
		input.CreatedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create menu entity: %w", err)
	}

	if err := s.menuRepo.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("failed to save menu: %w", err)
	}

	return m, nil
}

// GetMenuByID retrieves a menu by ID.
func (s *Service) GetMenuByID(ctx context.Context, id uuid.UUID) (*menu.Menu, error) {
	return s.menuRepo.GetByID(ctx, id)
}

// GetMenuByCode retrieves a menu by code.
func (s *Service) GetMenuByCode(ctx context.Context, code string) (*menu.Menu, error) {
	return s.menuRepo.GetByCode(ctx, code)
}

// UpdateMenuInput represents input for updating a menu.
type UpdateMenuInput struct {
	ID        uuid.UUID
	Title     *string
	URL       *string
	IconName  *string
	SortOrder *int
	IsVisible *bool
	IsActive  *bool
	UpdatedBy string
}

// UpdateMenu updates an existing menu.
func (s *Service) UpdateMenu(ctx context.Context, input UpdateMenuInput) (*menu.Menu, error) {
	m, err := s.menuRepo.GetByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get menu: %w", err)
	}

	if err := m.Update(input.Title, input.URL, input.IconName, input.SortOrder, input.IsVisible, input.IsActive, input.UpdatedBy); err != nil {
		return nil, fmt.Errorf("failed to update menu entity: %w", err)
	}

	if err := s.menuRepo.Update(ctx, m); err != nil {
		return nil, fmt.Errorf("failed to save menu: %w", err)
	}

	return m, nil
}

// DeleteMenu soft deletes a menu.
func (s *Service) DeleteMenu(ctx context.Context, id uuid.UUID, deletedBy string) error {
	// Check if menu has children
	hasChildren, err := s.menuRepo.HasChildren(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to check children: %w", err)
	}
	if hasChildren {
		return menu.ErrHasChildren
	}

	return s.menuRepo.Delete(ctx, id, deletedBy)
}

// DeleteMenuWithChildren deletes a menu and all its children.
func (s *Service) DeleteMenuWithChildren(ctx context.Context, id uuid.UUID, deletedBy string) (int, error) {
	return s.menuRepo.DeleteWithChildren(ctx, id, deletedBy)
}

// ListMenus lists menus with pagination.
func (s *Service) ListMenus(ctx context.Context, params menu.ListParams) ([]*menu.Menu, int64, error) {
	return s.menuRepo.List(ctx, params)
}

// GetMenuTree retrieves the complete menu tree for a service.
func (s *Service) GetMenuTree(ctx context.Context, serviceName string, includeInactive, includeHidden bool) ([]*menu.WithChildren, error) {
	return s.menuRepo.GetTree(ctx, serviceName, includeInactive, includeHidden)
}

// GetMenuTreeForUser retrieves the menu tree accessible by a specific user.
func (s *Service) GetMenuTreeForUser(ctx context.Context, userID uuid.UUID, serviceName string) ([]*menu.WithChildren, error) {
	return s.menuRepo.GetTreeForUser(ctx, userID, serviceName)
}

// AssignPermissions assigns permissions to a menu.
func (s *Service) AssignPermissions(ctx context.Context, menuID uuid.UUID, permissionIDs []uuid.UUID, assignedBy string) error {
	// Verify menu exists
	_, err := s.menuRepo.GetByID(ctx, menuID)
	if err != nil {
		return fmt.Errorf("invalid menu: %w", err)
	}

	return s.menuRepo.AssignPermissions(ctx, menuID, permissionIDs, assignedBy)
}

// RemovePermissions removes permissions from a menu.
func (s *Service) RemovePermissions(ctx context.Context, menuID uuid.UUID, permissionIDs []uuid.UUID) error {
	return s.menuRepo.RemovePermissions(ctx, menuID, permissionIDs)
}

// ReorderMenus reorders menus within a parent.
func (s *Service) ReorderMenus(ctx context.Context, parentID *uuid.UUID, menuIDs []uuid.UUID) error {
	return s.menuRepo.Reorder(ctx, parentID, menuIDs)
}
