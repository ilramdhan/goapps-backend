package commonlot

import "errors"

// Domain sentinel errors for common lots.
var (
	// ErrNotFound is returned when a common lot does not exist.
	ErrNotFound = errors.New("common lot not found")
	// ErrAlreadyExists is returned when the new ERP lot number is already taken.
	ErrAlreadyExists = errors.New("common lot number already exists")
	// ErrEmptyLotNo is returned when the ERP lot number is blank.
	ErrEmptyLotNo = errors.New("common lot number cannot be empty")
	// ErrNoComponents is returned when a common lot has no components.
	ErrNoComponents = errors.New("common lot requires at least one component")
	// ErrEmptyOriginalLot is returned when a component's original lot is blank.
	ErrEmptyOriginalLot = errors.New("component original lot cannot be empty")
	// ErrNegativeBobbin is returned when a component bobbin count is negative.
	ErrNegativeBobbin = errors.New("component bobbin count cannot be negative")
	// ErrNegativeQty is returned when a component quantity is negative.
	ErrNegativeQty = errors.New("component quantity cannot be negative")
)
