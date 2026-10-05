package erpintegration

// transitions is the allowed-transition table of design Part 1 §5.3. Any
// (from, to) pair not listed is rejected with ErrInvalidTransition.
var transitions = map[BatchStatus]map[BatchStatus]struct{}{
	// LoadDemand OK; Abandon.
	StatusDraft: set(StatusDemandLoaded, StatusFailed),
	// reload; Coverage (0 blocking gaps); Abandon.
	StatusDemandLoaded: set(StatusDemandLoaded, StatusCovered, StatusFailed),
	// reload; re-coverage; Derive; Abandon.
	StatusCovered: set(StatusDemandLoaded, StatusCovered, StatusDerived, StatusFailed),
	// reload; re-derive; Validate; Abandon.
	StatusDerived: set(StatusDemandLoaded, StatusDerived, StatusValidated, StatusFailed),
	// reload; re-derive (clears the ack); Push commit; Abandon.
	StatusValidated: set(StatusDemandLoaded, StatusDerived, StatusPushed, StatusFailed),
	// valuation refused / UNKNOWN pending; VALUATE_ADJ SUCCESS; Abandon (PG only).
	StatusPushed: set(StatusPushed, StatusValuated, StatusFailed),
	// recon with DIFF; Recon all MATCH; newer batch VALUATED; RESTORE_ADJ.
	StatusValuated: set(StatusValuated, StatusReconciled, StatusSuperseded, StatusFailed),
	// re-recon shows DIFF; newer batch VALUATED; RESTORE_ADJ; LOCK_BATCH.
	StatusReconciled: set(StatusValuated, StatusSuperseded, StatusFailed, StatusLocked),
	// Terminal: LOCKED, FAILED, SUPERSEDED have no outgoing transitions.
}

func set(ss ...BatchStatus) map[BatchStatus]struct{} {
	m := make(map[BatchStatus]struct{}, len(ss))
	for _, s := range ss {
		m[s] = struct{}{}
	}
	return m
}

// CanTransition reports whether the §5.3 table allows from -> to. Unknown
// statuses are never allowed.
func CanTransition(from, to BatchStatus) bool {
	_, ok := transitions[from][to]
	return ok
}

// Invalidation lists the batch-owned artifacts a transition makes obsolete.
// The application step deletes/replaces them in the same PG transaction as the
// status update (replace-per-batch pattern, plan-04 P3-T2).
type Invalidation struct {
	// Demand rows are replaced (load / reload).
	Demand bool
	// Coverage rows are cleared or replaced.
	Coverage bool
	// StdRows (derived standard cost rows) are cleared or replaced.
	StdRows bool
	// RuleSnapshot, rule hash and control totals are cleared.
	RuleSnapshot bool
	// WarningsAck (V-05 / V-08w acknowledgement) is cleared.
	WarningsAck bool
}

// Any reports whether at least one artifact is invalidated.
func (i Invalidation) Any() bool {
	return i.Demand || i.Coverage || i.StdRows || i.RuleSnapshot || i.WarningsAck
}

// InvalidationFor returns what the (from, to) transition invalidates.
// Re-running a step moves the batch back to that step's status and every
// downstream artifact is invalidated (plan-04 P3-T1 step 2). Disallowed
// pairs return the zero value.
func InvalidationFor(from, to BatchStatus) Invalidation {
	if !CanTransition(from, to) {
		return Invalidation{}
	}
	switch to {
	case StatusDemandLoaded:
		// Load or reload: demand replaced, everything downstream cleared.
		return Invalidation{Demand: true, Coverage: true, StdRows: true, RuleSnapshot: true, WarningsAck: true}
	case StatusCovered:
		// Coverage or re-coverage: coverage replaced; std rows cannot exist
		// yet at COVERED but are cleared defensively.
		return Invalidation{Coverage: true, StdRows: true, RuleSnapshot: true, WarningsAck: true}
	case StatusDerived:
		// Derive or re-derive: std rows, snapshot and totals replaced; the ack
		// belongs to the previous derivation.
		return Invalidation{StdRows: true, RuleSnapshot: true, WarningsAck: true}
	default:
		return Invalidation{}
	}
}
