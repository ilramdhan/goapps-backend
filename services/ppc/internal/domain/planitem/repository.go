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
	// ListCarryCandidates returns plan items in sourceMonth whose status still
	// permits carrying, each decorated with the work-order coverage that decides
	// how much of it is actually left to carry, and with whether targetMonth
	// already holds a row carried from it.
	ListCarryCandidates(ctx context.Context, sourceMonth, targetMonth string) ([]*CarryCandidate, error)
	// CarryCoverage answers the same coverage/duplicate questions for one plan
	// item, so a carry can be validated server-side without trusting the list.
	CarryCoverage(ctx context.Context, planItemID int64, targetMonth string) (Coverage, error)
}

// Coverage is how much of a plan item's quantity is already spoken for, from
// the two directions it can be spent.
//
// A plan item's qty_target is the planner's claim on its demand. That claim is
// consumed in two ways, and BOTH must be debited or the same quantity gets
// planned twice:
//
//  1. A work order raised against it (wo_plan_item_link.wpl_qty_contribution)
//     commits part of the claim to production.
//  2. Carrying part of it into another month creates a new plan item holding
//     that quantity there. The source row survives for audit — it is
//     deliberately not closed — but the quantity it handed over is no longer
//     its to spend, or a work order raised against the source afterwards would
//     re-book production the carried child already plans.
type Coverage struct {
	// QtyCovered is SUM(wpl_qty_contribution) over the item's work orders,
	// excluding work orders that were cancelled or rejected — those consumed
	// nothing and their share is legitimately still carryable.
	QtyCovered float64
	// WorkOrderCount counts the same non-void work orders.
	WorkOrderCount int32
	// QtyCarriedAway is SUM(qty_target) over every plan item anywhere that names
	// this one as its carry source. Deliberately NOT scoped to one target month:
	// a source-scoped debit is what stops Aug→Sep followed by Aug→Oct handing out
	// the same quantity twice.
	QtyCarriedAway float64
	// CarriedToMonths lists the distinct months this item has been carried into,
	// ascending. Empty when it has never been carried. Months, not ids — this is
	// what the UI shows.
	CarriedToMonths []string
}

// IsCarried reports whether this item has been carried into any month.
func (c Coverage) IsCarried() bool { return len(c.CarriedToMonths) > 0 }

// CarryCandidate is one plan item offered at month start, with its coverage.
type CarryCandidate struct {
	Item     *PlanItem
	Coverage Coverage
	// DemandLabel names the demand this item serves in words a planner
	// recognizes — its contract number, falling back to its type. Empty when the
	// item serves a parent plan item rather than a demand directly. Never an id.
	DemandLabel string
}

// QtyUncovered is the quantity of a candidate still free to carry — what
// CARRY_AS_IS carries. Both debits in Coverage are applied: quantity already
// committed to work orders, and quantity already handed to a carried child in
// any month. Subtracting only the first would let Aug→Sep followed by Aug→Oct
// carry the same quantity twice, which is the double-count S-2.2 forbids.
// Clamped at zero: an over-contributing work order must never produce a
// negative carry.
func (c *CarryCandidate) QtyUncovered() float64 {
	remaining := c.Item.QtyTarget() - c.Coverage.QtyCovered - c.Coverage.QtyCarriedAway
	if remaining < 0 {
		return 0
	}
	return remaining
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
