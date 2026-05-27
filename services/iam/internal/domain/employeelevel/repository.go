// Package employeelevel provides domain logic for Employee Level management.
package employeelevel

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines persistence operations for employee levels.
type Repository interface {
	Create(ctx context.Context, el *EmployeeLevel) error
	GetByID(ctx context.Context, id uuid.UUID) (*EmployeeLevel, error)
	GetByCode(ctx context.Context, code string) (*EmployeeLevel, error)
	Update(ctx context.Context, el *EmployeeLevel) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy string) error
	List(ctx context.Context, params ListParams) ([]*EmployeeLevel, int64, error)
	ListAll(ctx context.Context, filter ExportFilter) ([]*EmployeeLevel, error)
	ExistsByCode(ctx context.Context, code string) (bool, error)
	BatchCreate(ctx context.Context, items []*EmployeeLevel) (int, error)
}

// ExportFilter filters records for export (all optional).
type ExportFilter struct {
	IsActive *bool
	Type     *Type
	Workflow *Workflow
}

// WorkflowHistoryRepository persists workflow transition audit records.
type WorkflowHistoryRepository interface {
	Record(ctx context.Context, entry *WorkflowHistory) error
}

// WorkflowHistory records a single workflow transition.
type WorkflowHistory struct {
	ID         uuid.UUID
	EntityType string
	EntityID   uuid.UUID
	FromState  int32
	ToState    int32
	Action     string
	UserID     string
	Notes      string
}

// ListParams contains parameters for listing employee levels.
type ListParams struct {
	Page      int
	PageSize  int
	Search    string
	IsActive  *bool
	Type      *Type
	Workflow  *Workflow
	SortBy    string
	SortOrder string
}
