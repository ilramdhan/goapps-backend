package planitem

import (
	"context"
	"time"
)

// Repository defines persistence operations for plan items + their change log.
type Repository interface {
	// Create persists a new plan item and assigns its ID.
	Create(ctx context.Context, entity *PlanItem) error
	// CreateBatch persists a whole cascade chain in ONE transaction, in slice
	// order, assigning each ID. An item carrying a pending parent index (see
	// PendingParentIndex/ResolvePendingParent) has its parent stamped from the ID
	// assigned to that earlier item, so a chain can be written before any of its
	// IDs exist.
	// A partial cascade is worse than none: any failure rolls the batch back.
	CreateBatch(ctx context.Context, items []*PlanItem) error
	// GetByID retrieves a plan item by its ID.
	GetByID(ctx context.Context, id int64) (*PlanItem, error)
	// List retrieves plan items with filtering and pagination.
	List(ctx context.Context, filter ListFilter) ([]*PlanItem, int64, error)
	// Update persists changes to an existing plan item and appends the given
	// field-level change log entries in the same transaction.
	Update(ctx context.Context, entity *PlanItem, changes []LogEntry) error
	// Delete removes a plan item by its ID.
	Delete(ctx context.Context, id int64) error
	// ListForGantt retrieves plan items for a month/area window for the Gantt view,
	// decorated with machine/WO/lot/changeover fields.
	ListForGantt(ctx context.Context, filter GanttFilter) ([]*GanttRow, error)
}

// LogEntry is one row appended to production_plan_log.
type LogEntry struct {
	Field     string
	Before    string
	After     string
	ChangedBy int64
	Reason    string
}

// ListFilter contains filtering and pagination for listing plan items.
type ListFilter struct {
	Search         string
	Month          string
	Type           string
	Status         string
	MachineGroupID *int64
	DemandID       *int64
	Page           int
	PageSize       int
	SortBy         string
	SortOrder      string
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
		f.SortBy = "sequence"
	}
	if f.SortOrder == "" {
		f.SortOrder = "asc"
	}
}

// Offset returns the SQL offset for pagination.
func (f *ListFilter) Offset() int { return (f.Page - 1) * f.PageSize }

// GanttFilter scopes a Gantt-view query to a month/area window.
type GanttFilter struct {
	Month          string
	Area           string
	MachineGroupID *int64
	FromDate       *time.Time
	ToDate         *time.Time
}
