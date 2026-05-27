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
	ErrFormulaEval         = errors.New("formula evaluation failed")
	ErrCycleDetected       = errors.New("dependency cycle detected")
	ErrChunkRetryExhausted = errors.New("chunk retry attempts exhausted")
	ErrInvalidPeriod       = errors.New("period must be YYYYMM")
)
