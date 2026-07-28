package changeover

import "errors"

// Sentinel domain errors for changeover events.
var (
	ErrNotFound          = errors.New("changeover event not found")
	ErrMissingWO         = errors.New("changeover requires both from and to work orders")
	ErrMissingMachine    = errors.New("changeover requires a machine")
	ErrNoComponents      = errors.New("changeover requires at least one component")
	ErrInvalidTransition = errors.New("invalid changeover status transition")
	ErrNegativeActual    = errors.New("changeover actual duration and waste must be non-negative")
)
