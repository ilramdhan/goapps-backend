package costcalc

import "errors"

// Sentinel errors for the cost calculation engine domain.
var (
	ErrJobNotFound         = errors.New("calc job not found")
	ErrJobAlreadyRunning   = errors.New("calc job already running for scope")
	ErrJobInvalidStatus    = errors.New("calc job state transition not allowed from current status")
	ErrCostNotFound        = errors.New("cost result not found")
	ErrCostAlreadyInFlight = errors.New("cost result already in active job")
	ErrCostInvalidStatus   = errors.New("cost result state transition not allowed from current status")
	ErrMissingCAPPValue    = errors.New("missing CAPP value")
	ErrMissingRMCost       = errors.New("missing RM cost for item")
	ErrMissingUpstreamCost = errors.New("missing upstream product cost")
	ErrMissingMBCost       = errors.New("missing MB cost lookup value")
	// ErrMissingSpinFixedCost is returned when no active mst_spin_fixed_cost row
	// exists at or before the requested period. Proceeding would zero-fill the POY
	// spin pool and silently understate fixed cost rather than fail.
	ErrMissingSpinFixedCost = errors.New("no active spin fixed cost master row at or before the requested period")
	ErrFormulaEval          = errors.New("formula evaluation failed")
	ErrCycleDetected        = errors.New("dependency cycle detected")
	ErrChunkRetryExhausted  = errors.New("chunk retry attempts exhausted")
	ErrInvalidPeriod        = errors.New("period must be YYYYMM")
)
