// Package companymapping provides domain logic for Company Mapping management.
package companymapping

import (
	"context"

	"github.com/google/uuid"
)

// ListParams filters and paginates a list query.
type ListParams struct {
	Page         int
	PageSize     int
	Search       string
	CompanyID    *uuid.UUID
	DivisionID   *uuid.UUID
	DepartmentID *uuid.UUID
	SectionID    *uuid.UUID
	IsActive     *bool
	SortBy       string
	SortOrder    string
}

// UserAssignment represents a single user → mapping link with primary flag.
type UserAssignment struct {
	Mapping   *CompanyMapping
	IsPrimary bool
}

// Repository defines persistence operations for company mappings.
type Repository interface {
	// CRUD on the mapping entity itself.
	Create(ctx context.Context, m *CompanyMapping) error
	GetByID(ctx context.Context, id uuid.UUID) (*CompanyMapping, error)
	Update(ctx context.Context, m *CompanyMapping) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error
	List(ctx context.Context, params ListParams) ([]*CompanyMapping, int64, error)
	ExistsByCode(ctx context.Context, code string) (bool, error)

	// User ↔ mapping junction operations.
	AssignToUser(ctx context.Context, userID, mappingID uuid.UUID, isPrimary bool, assignedBy string) error
	RemoveFromUser(ctx context.Context, userID, mappingID uuid.UUID) error
	ListByUser(ctx context.Context, userID uuid.UUID) ([]UserAssignment, *uuid.UUID, error)
}
