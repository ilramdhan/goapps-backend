package demand

import "errors"

// Domain errors for demand operations. Messages are matched by the delivery
// error mapper (contains "not found"/"invalid"/etc.) so keep the wording stable.
var (
	// ErrNotFound is returned when a demand is not found.
	ErrNotFound = errors.New("demand not found")
	// ErrInvalidType is returned when the demand type is not allowed.
	ErrInvalidType = errors.New("invalid demand type")
	// ErrInvalidSubType is returned when the sub-type is not valid for the type.
	ErrInvalidSubType = errors.New("invalid demand sub type for type")
	// ErrInvalidSource is returned when the demand source is not allowed.
	ErrInvalidSource = errors.New("invalid demand source")
	// ErrInvalidGradeReq is returned when the grade requirement is not allowed.
	ErrInvalidGradeReq = errors.New("invalid grade requirement")
	// ErrInvalidQty is returned when a quantity is not a positive value.
	ErrInvalidQty = errors.New("invalid quantity: must be greater than zero")
	// ErrInvalidRemaining is returned when remaining is out of [0, original].
	ErrInvalidRemaining = errors.New("invalid remaining quantity")
	// ErrInvalidMonth is returned when the month is not YYYY-MM.
	ErrInvalidMonth = errors.New("invalid month: must be YYYY-MM")
	// ErrMonthMismatch is returned when the month diverges from the deadline without an override.
	ErrMonthMismatch = errors.New("invalid month: must match the deadline unless month_override is set")
	// ErrInvalidDeadline is returned when the deadline is empty or malformed.
	ErrInvalidDeadline = errors.New("invalid deadline")
	// ErrClausePctRequired is returned when AX/AM pct are missing for AX_AM_CLAUSE.
	ErrClausePctRequired = errors.New("ax_min_pct and am_max_pct required for AX_AM_CLAUSE")
	// ErrInvalidPct is returned when a percentage is out of the 0-100 range.
	ErrInvalidPct = errors.New("invalid percentage: must be between 0 and 100")
	// ErrIllegalTransition is returned when a status transition is not allowed.
	ErrIllegalTransition = errors.New("invalid demand status transition")
	// ErrNotMTS is returned when an MTS-only action targets a non-MTS demand.
	ErrNotMTS = errors.New("invalid operation: demand is not an MTS request")
	// ErrSplitExceedsRemaining is returned when split children exceed remaining.
	// Message is user-facing (Indonesian) per spec §7.1.
	ErrSplitExceedsRemaining = errors.New("ppc: split qty melebihi remaining qty demand")
	// ErrNoSplitChildren is returned when a SPLIT action has no children.
	ErrNoSplitChildren = errors.New("invalid split: at least one child demand required")
	// ErrInvalidCarryAction is returned when the carry-forward action is unknown.
	ErrInvalidCarryAction = errors.New("invalid carry-forward action")
	// ErrNotCarryCandidate is returned when a demand is not eligible for carry-forward.
	ErrNotCarryCandidate = errors.New("invalid operation: demand is not a carry-forward candidate")
	// ErrHasPlanItems is returned when deleting a demand that still has plan items.
	ErrHasPlanItems = errors.New("cannot delete demand: it still has plan items; delete or reassign them first")
	// ErrProductAlreadyMapped is returned when mapping a product onto a demand
	// that already has one (product is locked after being mapped once).
	ErrProductAlreadyMapped = errors.New("invalid operation: demand already has a mapped product")
	// ErrInvalidProduct is returned when a product sys id is not positive.
	ErrInvalidProduct = errors.New("invalid product: cpm_product_sys_id must be greater than zero")
	// ErrLinkReasonRequired is returned when a demand is created with no product
	// but no valid product-link reason to justify the deferred link.
	ErrLinkReasonRequired = errors.New("invalid product link: a demand with no product requires a product_link_reason of AUTO_MATCH_FAILED, AMBIGUOUS or NO_MASTER_YET")
	// ErrLinkReasonNotAllowed is returned when a product-link reason is supplied
	// for a demand that already carries a product.
	ErrLinkReasonNotAllowed = errors.New("invalid product link: product_link_reason is only allowed when the demand has no product")
	// ErrStagingNotUpdatable is returned when a manual product pick targets a
	// staging row that does not exist or has already been pulled into a demand.
	ErrStagingNotUpdatable = errors.New("staging row not found or already pulled to a demand")
	// ErrResolverDegraded is returned by a ProductResolver when the finance
	// connection is unavailable. Staging resolution treats it as a skip, not a
	// failure: the inbox stays listable and pullable with unresolved rows.
	ErrResolverDegraded = errors.New("product resolver unavailable: finance connection is degraded")
)
