// Package machinegroup provides domain logic for PPC machine-group master data.
package machinegroup

import "context"

// Repository defines persistence operations for machine groups.
type Repository interface {
	// Create persists a new machine group and assigns its ID.
	Create(ctx context.Context, entity *MachineGroup) error

	// GetByID retrieves a machine group by its ID.
	GetByID(ctx context.Context, id int64) (*MachineGroup, error)

	// List retrieves machine groups with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*MachineGroup, int64, error)

	// Update persists changes to an existing machine group.
	Update(ctx context.Context, entity *MachineGroup) error

	// Delete removes a machine group by its ID.
	Delete(ctx context.Context, id int64) error
}

// ListFilter contains filtering and pagination options for listing groups.
type ListFilter struct {
	Search    string
	Area      string
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// Validate normalizes pagination and sort defaults.
func (f *ListFilter) Validate() {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 10
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	if f.SortBy == "" {
		f.SortBy = "name"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the SQL offset for pagination.
func (f *ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}
