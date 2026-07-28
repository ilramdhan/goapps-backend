package planitem

import "errors"

// Domain errors for plan-item operations.
var (
	// ErrNotFound is returned when a plan item is not found.
	ErrNotFound = errors.New("plan item not found")
	// ErrInvalidType is returned when the plan item type is not allowed.
	ErrInvalidType = errors.New("invalid plan item type")
	// ErrInvalidRMSource is returned when the RM source is not allowed.
	ErrInvalidRMSource = errors.New("invalid RM source")
	// ErrInvalidStatus is returned when the status is not allowed.
	ErrInvalidStatus = errors.New("invalid plan item status")
	// ErrInvalidQty is returned when the target quantity is not positive.
	ErrInvalidQty = errors.New("invalid quantity: must be greater than zero")
	// ErrInvalidDeadline is returned when the deadline is empty.
	ErrInvalidDeadline = errors.New("invalid deadline")
	// ErrInvalidMonth is returned when the month is not YYYY-MM.
	ErrInvalidMonth = errors.New("invalid month: must be YYYY-MM")
	// ErrDemandOrParentRequired is returned when neither demand nor parent is set.
	ErrDemandOrParentRequired = errors.New("invalid plan item: demand_id or parent_item_id is required")
	// ErrDemandAndParentSet is returned when both demand and parent are set.
	ErrDemandAndParentSet = errors.New("invalid plan item: only one of demand_id or parent_item_id may be set")
	// ErrMachineGroupRequired is returned when the machine group is missing.
	ErrMachineGroupRequired = errors.New("invalid plan item: machine_group_id is required")
	// ErrInvalidDuration is returned when the planned duration is below one day.
	ErrInvalidDuration = errors.New("invalid planned duration: must be at least 1 day")
	// ErrStartAfterDeadline is returned when the planned start is later than the deadline.
	ErrStartAfterDeadline = errors.New("invalid timeline: planned start date must not be after the deadline")
	// ErrTimelineMismatch is returned when the supplied start date and duration contradict each other.
	ErrTimelineMismatch = errors.New("invalid timeline: planned start date and duration are inconsistent with the deadline")
	// ErrMonthMismatch is returned when the month diverges from the deadline without an override.
	ErrMonthMismatch = errors.New("invalid month: must match the deadline unless month_override is set")
	// ErrInvalidDurationSource is returned when the duration source is not allowed.
	ErrInvalidDurationSource = errors.New("invalid duration source")
	// ErrInvalidPendingParent is returned when a batch-relative parent index is negative.
	ErrInvalidPendingParent = errors.New("invalid plan item: pending parent index must not be negative")
	// ErrDemandProductNotLinked is returned when a plan item is created from a
	// demand that still has no finance product. Planning against an unknown
	// product would commit machines and materials to nothing.
	ErrDemandProductNotLinked = errors.New("invalid plan item: demand has no linked product yet; link the product to the demand first")
	// ErrIllegalTransition is returned when a status transition is not allowed.
	ErrIllegalTransition = errors.New("invalid plan item status transition")
)

// CascadeError reports a route walk that had to be abandoned, naming the
// product whose route triggered the guard. The offending product is carried as
// its code (never a raw sys id) so the message can be shown to a planner.
type CascadeError struct {
	Reason       string
	ProductLabel string
}

// Error implements the error interface.
func (e *CascadeError) Error() string {
	return "cannot cascade route: " + e.Reason + " at product " + e.ProductLabel
}

// NewCascadeError builds a cascade abort naming the offending product.
func NewCascadeError(reason, productLabel string) *CascadeError {
	if productLabel == "" {
		productLabel = "(unknown product)"
	}
	return &CascadeError{Reason: reason, ProductLabel: productLabel}
}
