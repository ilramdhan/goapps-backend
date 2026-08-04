// Package workorder provides Layer-3 work-order domain logic.
package workorder

// WO status values (wo_status) — the v1.2 sequential lifecycle.
const (
	StatusDraft      = "DRAFT"
	StatusSubmitted  = "SUBMITTED"
	StatusPCApproved = "PC_APPROVED"
	StatusApproved   = "APPROVED"
	StatusScheduled  = "SCHEDULED"
	StatusChangeover = "CHANGEOVER"
	StatusRunning    = "RUNNING"
	StatusCompleted  = "COMPLETED"
	StatusClosed     = "CLOSED"
	StatusRejected   = "REJECTED"
	StatusCancelled  = "CANCELLED"
)

// VoidStatuses are the work-order states that consumed none of their plan
// item's quantity. A cancelled or rejected WO never produced anything, so its
// qty_contribution must not count as coverage against the plan item — otherwise
// a rejected work order would permanently suppress the quantity it claimed from
// ever being carried into a new month.
var VoidStatuses = []string{StatusCancelled, StatusRejected}

// Production category values (wo_prod_category).
const (
	ProdCategoryNormal   = "NORMAL"
	ProdCategoryBToB     = "B_TO_B"
	ProdCategoryAPQ      = "APQ"
	ProdCategoryTrial    = "TRIAL"
	ProdCategorySmallLot = "SMALL_LOT"
)

// WO reference types (wo_ref_type).
const (
	RefTypeTemplate     = "TEMPLATE"     // Duplicate: soft copy of PPC params, new lot, no binding.
	RefTypeContinuation = "CONTINUATION" // Continuation: hard link inheriting demand + params.
)

// Approval sides for the sequential PC/PM approval.
const (
	ApprovalSidePC = "PC"
	ApprovalSidePM = "PM"
)

// Well-known parameter codes pinned for the efficiency engine (PRD v1.2). These
// are resolved to concrete param ids at runtime; the efficiency calc looks them
// up by these stable codes rather than by free-form name so it never breaks when
// operators add or rename parameters.
const (
	WellKnownDenier       = "DENIER"
	WellKnownYarnSpeed    = "YS"
	WellKnownNoOfPosition = "NO_OF_POSITION"
	WellKnownStdWeight    = "STD_WEIGHT"
)

// WellKnownCodes is the ordered set of efficiency-critical parameter codes.
var WellKnownCodes = []string{WellKnownDenier, WellKnownYarnSpeed, WellKnownNoOfPosition, WellKnownStdWeight}

// IsWellKnownCode reports whether code is an efficiency-critical parameter code.
func IsWellKnownCode(code string) bool {
	for _, c := range WellKnownCodes {
		if c == code {
			return true
		}
	}
	return false
}

// IsValidProdCategory reports whether c is an allowed production category.
// Empty is allowed (defaults to NORMAL at construction).
func IsValidProdCategory(c string) bool {
	switch c {
	case "", ProdCategoryNormal, ProdCategoryBToB, ProdCategoryAPQ, ProdCategoryTrial, ProdCategorySmallLot:
		return true
	default:
		return false
	}
}

// IsValidRefType reports whether t is an allowed WO reference type.
func IsValidRefType(t string) bool {
	switch t {
	case RefTypeTemplate, RefTypeContinuation:
		return true
	default:
		return false
	}
}
