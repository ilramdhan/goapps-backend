// Package employeegroup provides domain logic for Employee Group management.
package employeegroup

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines persistence operations for employee groups.
type Repository interface {
	Create(ctx context.Context, eg *EmployeeGroup) error
	GetByID(ctx context.Context, id uuid.UUID) (*EmployeeGroup, error)
	GetByCode(ctx context.Context, code string) (*EmployeeGroup, error)
	Update(ctx context.Context, eg *EmployeeGroup) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error
	List(ctx context.Context, params ListParams) ([]*EmployeeGroup, int64, error)
	ListAll(ctx context.Context, filter ExportFilter) ([]*EmployeeGroup, error)
	ExistsByCode(ctx context.Context, code string) (bool, error)
	BatchCreate(ctx context.Context, items []*EmployeeGroup) (int, error)
}

// ExportFilter filters records for export (all optional).
type ExportFilter struct {
	IsActive *bool
}

// ListParams contains parameters for listing employee groups.
type ListParams struct {
	Page      int
	PageSize  int
	Search    string
	IsActive  *bool
	SortBy    string
	SortOrder string
}
