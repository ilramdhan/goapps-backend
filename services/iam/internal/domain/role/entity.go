// Package role provides domain logic for Role and Permission management.
package role

import (
	"errors"
	"regexp"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// Domain-specific errors for role package.
var (
	ErrInvalidRoleCodeFormat       = errors.New("role code must start with a letter and contain only uppercase letters, numbers, and underscores")
	ErrInvalidPermissionCodeFormat = errors.New("permission code must follow format: service.module.entity.action")
	ErrInvalidActionType           = errors.New("action type must be one of: view, create, update, delete, export, import")
	ErrSystemRoleDelete            = errors.New("system roles cannot be deleted")
	ErrSystemRoleModify            = errors.New("system role code cannot be modified")
)

// Validation regexes.
var (
	roleCodeRegex       = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	permissionCodeRegex = regexp.MustCompile(`^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$`)
)

// Valid action types.
var validActionTypes = map[string]bool{
	"view":   true,
	"create": true,
	"update": true,
	"delete": true,
	"export": true,
	"import": true,
}

// =============================================================================
// ROLE
// =============================================================================

// Role represents a role entity.
type Role struct {
	id          uuid.UUID
	code        string
	name        string
	description string
	isSystem    bool
	isActive    bool
	audit       shared.AuditInfo
}

// NewRole creates a new Role entity.
func NewRole(code, name, description, createdBy string) (*Role, error) {
	if code == "" {
		return nil, shared.ErrEmptyCode
	}
	if !roleCodeRegex.MatchString(code) {
		return nil, ErrInvalidRoleCodeFormat
	}
	if name == "" {
		return nil, shared.ErrEmptyName
	}

	return &Role{
		id:          uuid.New(),
		code:        code,
		name:        name,
		description: description,
		isSystem:    false,
		isActive:    true,
		audit:       shared.NewAuditInfo(createdBy),
	}, nil
}

// ReconstructRole reconstructs a Role from persistence.
func ReconstructRole(id uuid.UUID, code, name, description string, isSystem, isActive bool, audit shared.AuditInfo) *Role {
	return &Role{
		id:          id,
		code:        code,
		name:        name,
		description: description,
		isSystem:    isSystem,
		isActive:    isActive,
		audit:       audit,
	}
}

// ID returns the role identifier.
func (r *Role) ID() uuid.UUID { return r.id }

// Code returns the role code.
func (r *Role) Code() string { return r.code }

// Name returns the role name.
func (r *Role) Name() string { return r.name }

// Description returns the role description.
func (r *Role) Description() string { return r.description }

// IsSystem returns whether the role is a system role.
func (r *Role) IsSystem() bool { return r.isSystem }

// IsActive returns whether the role is active.
func (r *Role) IsActive() bool { return r.isActive }

// Audit returns the audit information.
func (r *Role) Audit() shared.AuditInfo { return r.audit }

// IsDeleted returns whether the role has been soft-deleted.
func (r *Role) IsDeleted() bool { return r.audit.IsDeleted() }

// Update updates mutable role fields.
func (r *Role) Update(name, description *string, isActive *bool, updatedBy string) error {
	if r.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if name != nil {
		if *name == "" {
			return shared.ErrEmptyName
		}
		r.name = *name
	}
	if description != nil {
		r.description = *description
	}
	if isActive != nil {
		r.isActive = *isActive
	}
	r.audit.Update(updatedBy)
	return nil
}

// SoftDelete marks the role as soft-deleted.
func (r *Role) SoftDelete(deletedBy string) error {
	if r.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if r.isSystem {
		return ErrSystemRoleDelete
	}
	r.isActive = false
	r.audit.SoftDelete(deletedBy)
	return nil
}

// =============================================================================
// PERMISSION
// =============================================================================

// Permission represents a permission entity.
type Permission struct {
	id          uuid.UUID
	code        string
	name        string
	description string
	serviceName string
	moduleName  string
	actionType  string
	isActive    bool
	audit       shared.AuditInfo
}

// NewPermission creates a new Permission entity.
func NewPermission(code, name, description, serviceName, moduleName, actionType, createdBy string) (*Permission, error) {
	if code == "" {
		return nil, shared.ErrEmptyCode
	}
	if !permissionCodeRegex.MatchString(code) {
		return nil, ErrInvalidPermissionCodeFormat
	}
	if name == "" {
		return nil, shared.ErrEmptyName
	}
	if !validActionTypes[actionType] {
		return nil, ErrInvalidActionType
	}

	return &Permission{
		id:          uuid.New(),
		code:        code,
		name:        name,
		description: description,
		serviceName: serviceName,
		moduleName:  moduleName,
		actionType:  actionType,
		isActive:    true,
		audit:       shared.NewAuditInfo(createdBy),
	}, nil
}

// ReconstructPermission reconstructs a Permission from persistence.
func ReconstructPermission(id uuid.UUID, code, name, description, serviceName, moduleName, actionType string, isActive bool, audit shared.AuditInfo) *Permission {
	return &Permission{
		id:          id,
		code:        code,
		name:        name,
		description: description,
		serviceName: serviceName,
		moduleName:  moduleName,
		actionType:  actionType,
		isActive:    isActive,
		audit:       audit,
	}
}

// ID returns the permission identifier.
func (p *Permission) ID() uuid.UUID { return p.id }

// Code returns the permission code.
func (p *Permission) Code() string { return p.code }

// Name returns the permission name.
func (p *Permission) Name() string { return p.name }

// Description returns the permission description.
func (p *Permission) Description() string { return p.description }

// ServiceName returns the service name.
func (p *Permission) ServiceName() string { return p.serviceName }

// ModuleName returns the module name.
func (p *Permission) ModuleName() string { return p.moduleName }

// ActionType returns the action type.
func (p *Permission) ActionType() string { return p.actionType }

// IsActive returns whether the permission is active.
func (p *Permission) IsActive() bool { return p.isActive }

// Audit returns the audit information.
func (p *Permission) Audit() shared.AuditInfo { return p.audit }

// Update updates mutable permission fields.
func (p *Permission) Update(name, description *string, isActive *bool, updatedBy string) error {
	if name != nil {
		if *name == "" {
			return shared.ErrEmptyName
		}
		p.name = *name
	}
	if description != nil {
		p.description = *description
	}
	if isActive != nil {
		p.isActive = *isActive
	}
	p.audit.Update(updatedBy)
	return nil
}
