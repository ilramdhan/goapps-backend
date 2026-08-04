// Package planitem provides Layer-2 production-plan-item domain logic.
package planitem

// Plan item type values (ppi_type).
const (
	TypeFGDelivery   = "FG_DELIVERY"
	TypeIntermediate = "INTERMEDIATE"
	TypeMTS          = "MTS"
)

// RM source values (ppi_rm_source).
const (
	RMSourceStore   = "STORE"
	RMSourceCaptive = "CAPTIVE"
	RMSourceMixed   = "MIXED"
)

// Plan item status values (ppi_status) — the FR-2 lifecycle.
const (
	StatusDraft      = "DRAFT"
	StatusConfirmed  = "CONFIRMED"
	StatusInProgress = "IN_PROGRESS"
	StatusCompleted  = "COMPLETED"
	StatusClosed     = "CLOSED"
)

// Carry-forward action values (ppi_carry_action). Deliberately narrower than
// the demand's five: the plan-item lifecycle has no DEFERRED state to flip to,
// and a SPLIT would need per-child machine groups and timelines that the demand
// split never carries. Two creating actions plus a closing one is the whole
// vocabulary that maps onto this aggregate.
const (
	CarryActionAsIs    = "CARRY_AS_IS"
	CarryActionPartial = "PARTIAL_CARRY"
	CarryActionCancel  = "CANCEL"
)

// IsValidCarryAction reports whether a is an allowed plan carry-forward action.
func IsValidCarryAction(a string) bool {
	switch a {
	case CarryActionAsIs, CarryActionPartial, CarryActionCancel:
		return true
	default:
		return false
	}
}

// Duration source values (ppi_duration_source). DERIVED rows are recomputed
// from qty/capacity on every quantity edit; MANUAL rows carry a planner
// override and are never recomputed.
const (
	DurationSourceDerived = "DERIVED"
	DurationSourceManual  = "MANUAL"
)

// Planned duration bounds, in inclusive days.
const (
	MinDurationDays int32 = 1
	MaxDurationDays int32 = 60
)

// IntermediateLeadTimeDays is the fixed back-dating applied to a cascade
// intermediate item's deadline relative to its FG parent (Phase-1 constant;
// per-product lead time is a Phase-2 refinement).
const IntermediateLeadTimeDays = 3

// Cascade guards. A route is master data maintained by hand, so it can be
// malformed: self-referential, mutually recursive, or fanned out far wider than
// any real production chain. The walker therefore aborts rather than truncating
// — a silently shortened chain would under-plan production without anyone
// noticing, which is worse than a loud failure.
const (
	// MaxCascadeDepth bounds how many route levels the walker descends. The
	// deepest seeded route is 13 levels; 12 hops of upstream products is the
	// documented ceiling.
	MaxCascadeDepth = 12
	// MaxCascadeItems bounds how many INTERMEDIATE items one FG may generate.
	MaxCascadeItems = 50
)

// IsValidType reports whether t is an allowed plan item type.
func IsValidType(t string) bool {
	switch t {
	case TypeFGDelivery, TypeIntermediate, TypeMTS:
		return true
	default:
		return false
	}
}

// IsValidRMSource reports whether s is an allowed RM source. Empty is allowed
// (unset) at the domain level.
func IsValidRMSource(s string) bool {
	switch s {
	case "", RMSourceStore, RMSourceCaptive, RMSourceMixed:
		return true
	default:
		return false
	}
}

// IsValidStatus reports whether s is an allowed plan item status.
func IsValidStatus(s string) bool {
	switch s {
	case StatusDraft, StatusConfirmed, StatusInProgress, StatusCompleted, StatusClosed:
		return true
	default:
		return false
	}
}

// IsValidDurationSource reports whether s is an allowed duration source.
func IsValidDurationSource(s string) bool {
	switch s {
	case DurationSourceDerived, DurationSourceManual:
		return true
	default:
		return false
	}
}
