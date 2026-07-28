// Package demand provides Layer-1 production-demand domain logic.
package demand

// Demand type values (pd_type). Constrained by the chk_pd_type CHECK.
const (
	TypeContract = "CONTRACT"
	TypeMTS      = "MTS"
	TypeSample   = "SAMPLE"
)

// Demand sub-type values (pd_sub_type).
const (
	SubTypeCFExport  = "CF_EXPORT"
	SubTypeNewExport = "NEW_EXPORT"
	SubTypeLocal     = "LOCAL"
	SubTypeInternal  = "INTERNAL"
)

// Demand source values (pd_source).
const (
	SourceOrionPull    = "ORION_PULL"
	SourceManual       = "MANUAL"
	SourceMTSApproved  = "MTS_APPROVED"
	SourceCarryForward = "CARRY_FORWARD"
)

// Carry-forward action values (pd_carry_action).
const (
	CarryActionAsIs    = "CARRY_AS_IS"
	CarryActionSplit   = "SPLIT"
	CarryActionDefer   = "DEFER"
	CarryActionPartial = "PARTIAL_CARRY"
	CarryActionCancel  = "CANCEL"
)

// Grade-requirement values (pd_grade_requirement).
const (
	GradeReqAXOnly     = "AX_ONLY"
	GradeReqAXAMClause = "AX_AM_CLAUSE"
	GradeReqNone       = "NONE"
)

// Demand status values (pd_status) — the FR-1 lifecycle.
const (
	StatusPendingConfirmation = "PENDING_CONFIRMATION"
	StatusConfirmed           = "CONFIRMED"
	StatusInProduction        = "IN_PRODUCTION"
	StatusPartial             = "PARTIAL"
	StatusFulfilled           = "FULFILLED"
	StatusCancelled           = "CANCELLED"
	StatusCarriedOver         = "CARRIED_OVER"
	StatusDeferred            = "DEFERRED"
	StatusSplit               = "SPLIT"
	// StatusPendingProductLink marks a demand saved without a product — an
	// MTS/SAMPLE raised before its finance master exists, or an Orion pull whose
	// product could not be resolved. Its only way out is being linked.
	StatusPendingProductLink = "PENDING_PRODUCT_LINK"
)

// Product-link reason values (pd_product_link_reason). Recorded only while the
// demand is PENDING_PRODUCT_LINK, so "intentionally unresolved" stays
// distinguishable from "resolution failed".
const (
	// LinkReasonAutoMatchFailed means the Orion row matched no finance product.
	LinkReasonAutoMatchFailed = "AUTO_MATCH_FAILED"
	// LinkReasonAmbiguous means the Orion row matched several finance products.
	LinkReasonAmbiguous = "AMBIGUOUS"
	// LinkReasonNoMasterYet means the demand was raised on purpose before its
	// finance cost-product-master row existed (MTS / SAMPLE).
	LinkReasonNoMasterYet = "NO_MASTER_YET"
)

// IsValidLinkReason reports whether r is an allowed product-link reason.
func IsValidLinkReason(r string) bool {
	switch r {
	case LinkReasonAutoMatchFailed, LinkReasonAmbiguous, LinkReasonNoMasterYet:
		return true
	default:
		return false
	}
}

// IsValidType reports whether t is an allowed demand type.
func IsValidType(t string) bool {
	switch t {
	case TypeContract, TypeMTS, TypeSample:
		return true
	default:
		return false
	}
}

// IsValidSource reports whether s is an allowed demand source.
func IsValidSource(s string) bool {
	switch s {
	case SourceOrionPull, SourceManual, SourceMTSApproved, SourceCarryForward:
		return true
	default:
		return false
	}
}

// IsValidGradeReq reports whether g is an allowed grade requirement.
func IsValidGradeReq(g string) bool {
	switch g {
	case GradeReqAXOnly, GradeReqAXAMClause, GradeReqNone:
		return true
	default:
		return false
	}
}

// subTypeAllowedFor reports whether sub is valid for the given demand type.
// An empty sub-type is allowed only for SAMPLE.
func subTypeAllowedFor(demandType, sub string) bool {
	switch demandType {
	case TypeContract:
		return sub == SubTypeCFExport || sub == SubTypeNewExport || sub == SubTypeLocal
	case TypeMTS:
		return sub == SubTypeInternal
	case TypeSample:
		return sub == ""
	default:
		return false
	}
}
