// Package organization provides domain logic for organization hierarchy management.
package organization

import (
	"context"

	"github.com/google/uuid"
)

// CompanyRepository defines the interface for company persistence operations.
type CompanyRepository interface {
	Create(ctx context.Context, company *Company) error
	GetByID(ctx context.Context, id uuid.UUID) (*Company, error)
	GetByCode(ctx context.Context, code string) (*Company, error)
	Update(ctx context.Context, company *Company) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error
	List(ctx context.Context, params ListParams) ([]*Company, int64, error)
	ExistsByCode(ctx context.Context, code string) (bool, error)
	BatchCreate(ctx context.Context, companies []*Company) (int, error)
}

// DivisionRepository defines the interface for division persistence operations.
type DivisionRepository interface {
	Create(ctx context.Context, division *Division) error
	GetByID(ctx context.Context, id uuid.UUID) (*Division, error)
	GetByCode(ctx context.Context, code string) (*Division, error)
	Update(ctx context.Context, division *Division) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error
	List(ctx context.Context, params DivisionListParams) ([]*Division, int64, error)
	ExistsByCode(ctx context.Context, code string) (bool, error)
	BatchCreate(ctx context.Context, divisions []*Division) (int, error)
}

// DepartmentRepository defines the interface for department persistence operations.
type DepartmentRepository interface {
	Create(ctx context.Context, department *Department) error
	GetByID(ctx context.Context, id uuid.UUID) (*Department, error)
	GetByCode(ctx context.Context, code string) (*Department, error)
	Update(ctx context.Context, department *Department) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error
	List(ctx context.Context, params DepartmentListParams) ([]*Department, int64, error)
	ExistsByCode(ctx context.Context, code string) (bool, error)
	BatchCreate(ctx context.Context, departments []*Department) (int, error)
}

// SectionRepository defines the interface for section persistence operations.
type SectionRepository interface {
	Create(ctx context.Context, section *Section) error
	GetByID(ctx context.Context, id uuid.UUID) (*Section, error)
	GetByCode(ctx context.Context, code string) (*Section, error)
	Update(ctx context.Context, section *Section) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error
	List(ctx context.Context, params SectionListParams) ([]*Section, int64, error)
	ExistsByCode(ctx context.Context, code string) (bool, error)
	BatchCreate(ctx context.Context, sections []*Section) (int, error)
}

// ListParams contains common parameters for listing.
type ListParams struct {
	Page      int
	PageSize  int
	Search    string
	IsActive  *bool
	SortBy    string
	SortOrder string
}

// DivisionListParams extends ListParams with company filter.
type DivisionListParams struct {
	ListParams
	CompanyID *uuid.UUID
}

// DepartmentListParams extends ListParams with division and company filters.
type DepartmentListParams struct {
	ListParams
	DivisionID *uuid.UUID
	CompanyID  *uuid.UUID
}

// SectionListParams extends ListParams with department, division, and company filters.
type SectionListParams struct {
	ListParams
	DepartmentID *uuid.UUID
	DivisionID   *uuid.UUID
	CompanyID    *uuid.UUID
}

// TreeNode represents a node in the organization hierarchy tree.
type TreeNode struct {
	ID       uuid.UUID
	ParentID *uuid.UUID
	Type     string // company, division, department, section
	Code     string
	Name     string
	IsActive bool
	Children []*TreeNode
}

// TreeRepository provides methods for building organization tree.
type TreeRepository interface {
	GetTree(ctx context.Context, companyID *uuid.UUID, includeInactive bool) ([]*TreeNode, error)
}
