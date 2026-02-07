// Package role provides domain logic for Role and Permission management.
package role

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the interface for role persistence operations.
type Repository interface {
	Create(ctx context.Context, role *Role) error
	GetByID(ctx context.Context, id uuid.UUID) (*Role, error)
	GetByCode(ctx context.Context, code string) (*Role, error)
	Update(ctx context.Context, role *Role) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error
	List(ctx context.Context, params ListParams) ([]*Role, int64, error)
	ExistsByCode(ctx context.Context, code string) (bool, error)
	BatchCreate(ctx context.Context, roles []*Role) (int, error)

	// Role-Permission assignment
	AssignPermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, assignedBy string) error
	RemovePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
	GetPermissions(ctx context.Context, roleID uuid.UUID) ([]*Permission, error)
	GetRolesByPermission(ctx context.Context, permissionID uuid.UUID) ([]*Role, error)
}

// PermissionRepository defines the interface for permission persistence operations.
type PermissionRepository interface {
	Create(ctx context.Context, permission *Permission) error
	GetByID(ctx context.Context, id uuid.UUID) (*Permission, error)
	GetByCode(ctx context.Context, code string) (*Permission, error)
	Update(ctx context.Context, permission *Permission) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error
	List(ctx context.Context, params PermissionListParams) ([]*Permission, int64, error)
	ExistsByCode(ctx context.Context, code string) (bool, error)
	BatchCreate(ctx context.Context, permissions []*Permission) (int, error)

	// Grouped by service/module
	GetByService(ctx context.Context, serviceName string, includeInactive bool) ([]*ServicePermissions, error)
}

// UserRoleRepository handles user-role assignments.
type UserRoleRepository interface {
	AssignRoles(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID, assignedBy string) error
	RemoveRoles(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID) error
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]*Role, error)
	GetUsersWithRole(ctx context.Context, roleID uuid.UUID) ([]uuid.UUID, error)
}

// UserPermissionRepository handles direct user-permission assignments.
type UserPermissionRepository interface {
	AssignPermissions(ctx context.Context, userID uuid.UUID, permissionIDs []uuid.UUID, assignedBy string) error
	RemovePermissions(ctx context.Context, userID uuid.UUID, permissionIDs []uuid.UUID) error
	GetUserDirectPermissions(ctx context.Context, userID uuid.UUID) ([]*Permission, error)
	GetEffectivePermissions(ctx context.Context, userID uuid.UUID) ([]*Permission, error) // Roles + Direct
}

// ListParams contains parameters for listing roles.
type ListParams struct {
	Page      int
	PageSize  int
	Search    string
	IsActive  *bool
	IsSystem  *bool
	SortBy    string
	SortOrder string
}

// PermissionListParams contains parameters for listing permissions.
type PermissionListParams struct {
	Page        int
	PageSize    int
	Search      string
	IsActive    *bool
	ServiceName string
	ModuleName  string
	ActionType  string
	SortBy      string
	SortOrder   string
}

// ServicePermissions groups permissions by service and module.
type ServicePermissions struct {
	ServiceName string
	Modules     []*ModulePermissions
}

// ModulePermissions groups permissions within a module.
type ModulePermissions struct {
	ModuleName  string
	Permissions []*Permission
}
