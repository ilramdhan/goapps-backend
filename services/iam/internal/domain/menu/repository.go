// Package menu provides domain logic for dynamic menu management.
package menu

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/role"
)

// Repository defines the interface for menu persistence operations.
type Repository interface {
	Create(ctx context.Context, menu *Menu) error
	GetByID(ctx context.Context, id uuid.UUID) (*Menu, error)
	GetByCode(ctx context.Context, code string) (*Menu, error)
	Update(ctx context.Context, menu *Menu) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error
	DeleteWithChildren(ctx context.Context, id uuid.UUID, deletedBy string) (int, error)
	List(ctx context.Context, params ListParams) ([]*Menu, int64, error)
	ExistsByCode(ctx context.Context, code string) (bool, error)
	HasChildren(ctx context.Context, id uuid.UUID) (bool, error)
	BatchCreate(ctx context.Context, menus []*Menu) (int, error)

	// Menu tree operations
	GetTree(ctx context.Context, serviceName string, includeInactive, includeHidden bool) ([]*WithChildren, error)
	GetTreeForUser(ctx context.Context, userID uuid.UUID, serviceName string) ([]*WithChildren, error)

	// Menu-Permission assignment
	AssignPermissions(ctx context.Context, menuID uuid.UUID, permissionIDs []uuid.UUID, assignedBy string) error
	RemovePermissions(ctx context.Context, menuID uuid.UUID, permissionIDs []uuid.UUID) error
	GetPermissions(ctx context.Context, menuID uuid.UUID) ([]*role.Permission, error)

	// Reorder
	Reorder(ctx context.Context, parentID *uuid.UUID, menuIDs []uuid.UUID) error
}

// ListParams contains parameters for listing menus.
type ListParams struct {
	Page        int
	PageSize    int
	Search      string
	IsActive    *bool
	IsVisible   *bool
	ServiceName string
	Level       *int
	ParentID    *uuid.UUID
	SortBy      string
	SortOrder   string
}
