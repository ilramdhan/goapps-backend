// Package menu provides domain logic for dynamic menu management.
package menu

import (
	"errors"
	"regexp"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// Domain-specific errors for menu package.
var (
	ErrInvalidMenuCodeFormat = errors.New("menu code must start with a letter and contain only uppercase letters, numbers, and underscores")
	ErrInvalidMenuLevel      = errors.New("menu level must be between 1 and 3")
	ErrRootMustNotHaveParent = errors.New("root level menu must not have a parent")
	ErrChildMustHaveParent   = errors.New("non-root level menu must have a parent")
	ErrHasChildren           = errors.New("cannot delete menu with children")
)

// Menu levels.
const (
	MenuLevelRoot   = 1
	MenuLevelParent = 2
	MenuLevelChild  = 3
)

// Validation regex.
var menuCodeRegex = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Menu represents a menu entity in the 3-level hierarchy.
type Menu struct {
	id          uuid.UUID
	parentID    *uuid.UUID
	code        string
	title       string
	url         string
	iconName    string
	serviceName string
	level       int
	sortOrder   int
	isVisible   bool
	isActive    bool
	audit       shared.AuditInfo
}

// NewMenu creates a new Menu entity with validation.
func NewMenu(
	parentID *uuid.UUID,
	code, title, url, iconName, serviceName string,
	level, sortOrder int,
	isVisible bool,
	createdBy string,
) (*Menu, error) {
	if code == "" {
		return nil, shared.ErrEmptyCode
	}
	if !menuCodeRegex.MatchString(code) {
		return nil, ErrInvalidMenuCodeFormat
	}
	if title == "" {
		return nil, shared.ErrEmptyName
	}
	if level < MenuLevelRoot || level > MenuLevelChild {
		return nil, ErrInvalidMenuLevel
	}
	if level == MenuLevelRoot && parentID != nil {
		return nil, ErrRootMustNotHaveParent
	}
	if level != MenuLevelRoot && parentID == nil {
		return nil, ErrChildMustHaveParent
	}

	return &Menu{
		id:          uuid.New(),
		parentID:    parentID,
		code:        code,
		title:       title,
		url:         url,
		iconName:    iconName,
		serviceName: serviceName,
		level:       level,
		sortOrder:   sortOrder,
		isVisible:   isVisible,
		isActive:    true,
		audit:       shared.NewAuditInfo(createdBy),
	}, nil
}

// ReconstructMenu reconstructs a Menu from persistence.
func ReconstructMenu(
	id uuid.UUID,
	parentID *uuid.UUID,
	code, title, url, iconName, serviceName string,
	level, sortOrder int,
	isVisible, isActive bool,
	audit shared.AuditInfo,
) *Menu {
	return &Menu{
		id:          id,
		parentID:    parentID,
		code:        code,
		title:       title,
		url:         url,
		iconName:    iconName,
		serviceName: serviceName,
		level:       level,
		sortOrder:   sortOrder,
		isVisible:   isVisible,
		isActive:    isActive,
		audit:       audit,
	}
}

// ID returns the menu identifier.
func (m *Menu) ID() uuid.UUID { return m.id }

// ParentID returns the parent menu identifier.
func (m *Menu) ParentID() *uuid.UUID { return m.parentID }

// Code returns the menu code.
func (m *Menu) Code() string { return m.code }

// Title returns the menu title.
func (m *Menu) Title() string { return m.title }

// URL returns the menu URL.
func (m *Menu) URL() string { return m.url }

// IconName returns the menu icon name.
func (m *Menu) IconName() string { return m.iconName }

// ServiceName returns the service name.
func (m *Menu) ServiceName() string { return m.serviceName }

// Level returns the menu level.
func (m *Menu) Level() int { return m.level }

// SortOrder returns the sort order.
func (m *Menu) SortOrder() int { return m.sortOrder }

// IsVisible returns whether the menu is visible.
func (m *Menu) IsVisible() bool { return m.isVisible }

// IsActive returns whether the menu is active.
func (m *Menu) IsActive() bool { return m.isActive }

// Audit returns the audit information.
func (m *Menu) Audit() shared.AuditInfo { return m.audit }

// IsDeleted returns whether the menu has been soft-deleted.
func (m *Menu) IsDeleted() bool { return m.audit.IsDeleted() }

// Update updates menu fields.
func (m *Menu) Update(
	title, url, iconName *string,
	sortOrder *int,
	isVisible, isActive *bool,
	updatedBy string,
) error {
	if m.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if title != nil {
		if *title == "" {
			return shared.ErrEmptyName
		}
		m.title = *title
	}
	if url != nil {
		m.url = *url
	}
	if iconName != nil {
		m.iconName = *iconName
	}
	if sortOrder != nil {
		m.sortOrder = *sortOrder
	}
	if isVisible != nil {
		m.isVisible = *isVisible
	}
	if isActive != nil {
		m.isActive = *isActive
	}
	m.audit.Update(updatedBy)
	return nil
}

// SoftDelete marks the menu as deleted.
func (m *Menu) SoftDelete(deletedBy string) error {
	if m.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	m.isActive = false
	m.audit.SoftDelete(deletedBy)
	return nil
}

// WithChildren represents a menu with its child menus (for tree view).
type WithChildren struct {
	Menu                *Menu
	Children            []*WithChildren
	RequiredPermissions []string
}
