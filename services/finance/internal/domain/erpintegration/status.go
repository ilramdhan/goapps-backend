package erpintegration

import "fmt"

// BatchStatus is the lifecycle status of a Batch (design Part 1 §5.3). It is
// stored in cst_erp_int_batch.ceib_status (CHECK: the 11 values below).
type BatchStatus string

// The 11 batch statuses (design §5.3).
const (
	StatusDraft        BatchStatus = "DRAFT"
	StatusDemandLoaded BatchStatus = "DEMAND_LOADED"
	StatusCovered      BatchStatus = "COVERED"
	StatusDerived      BatchStatus = "DERIVED"
	StatusValidated    BatchStatus = "VALIDATED"
	StatusPushed       BatchStatus = "PUSHED"
	StatusValuated     BatchStatus = "VALUATED"
	StatusReconciled   BatchStatus = "RECONCILED"
	StatusLocked       BatchStatus = "LOCKED"
	StatusFailed       BatchStatus = "FAILED"
	StatusSuperseded   BatchStatus = "SUPERSEDED"
)

// allStatuses lists every status in lifecycle order.
var allStatuses = []BatchStatus{
	StatusDraft, StatusDemandLoaded, StatusCovered, StatusDerived, StatusValidated,
	StatusPushed, StatusValuated, StatusReconciled, StatusLocked, StatusFailed, StatusSuperseded,
}

// AllBatchStatuses returns the 11 statuses in lifecycle order (a copy).
func AllBatchStatuses() []BatchStatus {
	out := make([]BatchStatus, len(allStatuses))
	copy(out, allStatuses)
	return out
}

// ParseBatchStatus parses an exact (upper-case) status value and rejects
// anything else with ErrInvalidBatchStatus.
func ParseBatchStatus(s string) (BatchStatus, error) {
	st := BatchStatus(s)
	if !st.IsValid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidBatchStatus, s)
	}
	return st, nil
}

// IsValid reports whether s is one of the 11 statuses.
func (s BatchStatus) IsValid() bool {
	for _, v := range allStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// String returns the stored value.
func (s BatchStatus) String() string { return string(s) }

// IsTerminal reports whether no transition leaves s. LOCKED and SUPERSEDED
// are terminal; FAILED is terminal too: a re-run needs a new batch.
func (s BatchStatus) IsTerminal() bool {
	return s == StatusLocked || s == StatusSuperseded || s == StatusFailed
}

// IsActive reports whether s counts toward uix_ceib_active (C-8): at most one
// LIVE batch per period is VALUATED, RECONCILED or LOCKED.
func (s BatchStatus) IsActive() bool {
	return s == StatusValuated || s == StatusReconciled || s == StatusLocked
}

// IsInFlight reports whether s counts toward uix_ceib_inflight: DRAFT through
// PUSHED.
func (s BatchStatus) IsInFlight() bool {
	switch s {
	case StatusDraft, StatusDemandLoaded, StatusCovered, StatusDerived, StatusValidated, StatusPushed:
		return true
	default:
		return false
	}
}

// AllowedInShadow reports whether a SHADOW batch may hold status s
// (chk_ceib_shadow_status; User decision 2026-09-29 U-4).
func (s BatchStatus) AllowedInShadow() bool {
	switch s {
	case StatusDraft, StatusDemandLoaded, StatusCovered, StatusDerived, StatusValidated, StatusFailed:
		return true
	default:
		return false
	}
}

// BatchMode distinguishes a real batch from a historical backtest
// (ceib_mode; User decision 2026-09-29 U-4).
type BatchMode string

// Batch modes.
const (
	ModeLive   BatchMode = "LIVE"
	ModeShadow BatchMode = "SHADOW"
)

// ParseBatchMode parses LIVE or SHADOW.
func ParseBatchMode(s string) (BatchMode, error) {
	m := BatchMode(s)
	if m != ModeLive && m != ModeShadow {
		return "", fmt.Errorf("%w: %q", ErrInvalidBatchMode, s)
	}
	return m, nil
}

// String returns the stored value.
func (m BatchMode) String() string { return string(m) }
