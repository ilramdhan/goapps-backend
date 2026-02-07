// Package user provides domain logic for User management.
package user

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the interface for user persistence operations.
type Repository interface {
	// User CRUD
	Create(ctx context.Context, user *User, detail *Detail) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error

	// User Detail
	GetDetailByUserID(ctx context.Context, userID uuid.UUID) (*Detail, error)
	UpdateDetail(ctx context.Context, detail *Detail) error

	// Listing
	List(ctx context.Context, params ListParams) ([]*User, int64, error)
	ListWithDetails(ctx context.Context, params ListParams) ([]*WithDetail, int64, error)

	// Bulk operations
	ExistsByUsername(ctx context.Context, username string) (bool, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	ExistsByEmployeeCode(ctx context.Context, code string) (bool, error)

	// Batch
	BatchCreate(ctx context.Context, users []*User, details []*Detail) (int, error)

	// Role and Permission operations
	GetRolesAndPermissions(ctx context.Context, userID uuid.UUID) ([]RoleRef, []PermissionRef, error)
}

// RoleRef represents a reference to a role with minimal fields.
type RoleRef interface {
	ID() uuid.UUID
	Code() string
	Name() string
}

// PermissionRef represents a reference to a permission with minimal fields.
type PermissionRef interface {
	ID() uuid.UUID
	Code() string
}

// WithDetail combines User and Detail for listing.
type WithDetail struct {
	User   *User
	Detail *Detail
	Roles  []RoleInfo
}

// RoleInfo contains minimal role information for user display.
type RoleInfo struct {
	RoleID   uuid.UUID
	RoleCode string
	RoleName string
}

// ListParams contains parameters for listing users.
type ListParams struct {
	Page         int
	PageSize     int
	Search       string
	IsActive     *bool
	SectionID    *uuid.UUID
	DepartmentID *uuid.UUID
	DivisionID   *uuid.UUID
	CompanyID    *uuid.UUID
	SortBy       string
	SortOrder    string
}

// ActiveFilter represents the filter for active status.
type ActiveFilter int

// ActiveFilter values for filtering by active status.
const (
	ActiveFilterAll      ActiveFilter = 0
	ActiveFilterActive   ActiveFilter = 1
	ActiveFilterInactive ActiveFilter = 2
)

// GetIsActive converts ActiveFilter to *bool for database query.
func (f ActiveFilter) GetIsActive() *bool {
	switch f {
	case ActiveFilterActive:
		active := true
		return &active
	case ActiveFilterInactive:
		active := false
		return &active
	default:
		return nil
	}
}
