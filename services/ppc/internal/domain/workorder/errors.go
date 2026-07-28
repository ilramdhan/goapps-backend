package workorder

import "errors"

// Domain errors for work-order operations.
var (
	// ErrNotFound is returned when a work order is not found.
	ErrNotFound = errors.New("work order not found")
	// ErrInvalidArea is returned when the area code is not TXT/SPG/TWT.
	ErrInvalidArea = errors.New("invalid area: must be TXT, SPG, or TWT")
	// ErrInvalidQty is returned when the target quantity is not positive.
	ErrInvalidQty = errors.New("invalid quantity: must be greater than zero")
	// ErrInvalidDeadline is returned when the deadline is empty.
	ErrInvalidDeadline = errors.New("invalid deadline")
	// ErrEmptyLot is returned when the lot number is empty.
	ErrEmptyLot = errors.New("lot number cannot be empty")
	// ErrInvalidProdCategory is returned when the production category is unknown.
	ErrInvalidProdCategory = errors.New("invalid production category")
	// ErrInvalidMachine is returned when the machine id is not positive.
	ErrInvalidMachine = errors.New("invalid machine id")
	// ErrInvalidPlanItem is returned when a work order covers no valid plan item.
	ErrInvalidPlanItem = errors.New("invalid plan item: a work order must cover at least one plan item with a positive quantity")
	// ErrDuplicatePlanItemLink is returned when the same plan item is listed
	// twice for one work order.
	ErrDuplicatePlanItemLink = errors.New("invalid merge: the same plan item is listed more than once")
	// ErrAnchorNotLinked is returned when the anchor plan item is missing from
	// the link set.
	ErrAnchorNotLinked = errors.New("invalid merge: the anchor plan item is not among the linked plan items")
	// ErrPlanItemAlreadyLinked is returned when a plan item is already covered
	// by another work order — a plan item can never be double-linked.
	ErrPlanItemAlreadyLinked = errors.New("invalid merge: plan item is already linked to a work order")
	// ErrNotMergeable is returned when a plan item does not satisfy the merge
	// predicate against the anchor (product, machine group, shade, deadline).
	ErrNotMergeable = errors.New("invalid merge: plan item is not mergeable with the anchor")
	// ErrInvalidRoute is returned when the route snapshot reference is invalid.
	ErrInvalidRoute = errors.New("invalid route: crh_head_id and crh_version required")
	// ErrInvalidRefType is returned when a WO reference type is not TEMPLATE/CONTINUATION.
	ErrInvalidRefType = errors.New("invalid reference type: must be TEMPLATE or CONTINUATION")
	// ErrIllegalTransition is returned when a status transition is not allowed.
	ErrIllegalTransition = errors.New("invalid work order status transition")
	// ErrInvalidApprovalSide is returned when an approval side is not PC/PM.
	ErrInvalidApprovalSide = errors.New("invalid approval side: must be PC or PM")
	// ErrNotSubmitted is returned when an approval targets a non-submitted WO.
	ErrNotSubmitted = errors.New("invalid operation: work order is not submitted")
	// ErrPCApprovalRequired is returned when PM approves before PC (sequential
	// approval, PRD v1.2).
	ErrPCApprovalRequired = errors.New("invalid operation: PC approval required before PM")
	// ErrMachineAreaMismatch is returned when the machine area does not match
	// the WO area code.
	ErrMachineAreaMismatch = errors.New("invalid machine: area does not match work order area")
	// ErrLotNotFound is returned when the WO lot is not in lot_master.
	ErrLotNotFound = errors.New("invalid lot: not found in lot master")
	// ErrLotSpecUnavailable is returned when a lot cannot be generated because
	// the product's item/shade codes or standard bobbin weights are unknown.
	// Registering a lot without them would leave the ETL unable to convert
	// bobbin counts into kilograms, so the work order is rejected instead.
	ErrLotSpecUnavailable = errors.New("cannot generate lot: product item/shade codes or standard bobbin weights are unavailable — register the lot in lot master first and enter it manually")
	// ErrLotGenerationUnavailable is returned when lot generation is requested
	// but no lot provisioner is configured.
	ErrLotGenerationUnavailable = errors.New("cannot generate lot: lot number generation is not available — enter a lot number registered in lot master")
	// ErrEmptyReason is returned when a required reason (reject/adjust) is empty.
	ErrEmptyReason = errors.New("reason cannot be empty")
	// ErrNotEditable is returned when a header edit targets a non-DRAFT WO
	// without a revision reason.
	ErrNotEditable = errors.New("invalid operation: work order is not editable")
	// ErrActualNotFound is returned when a production actual row is not found.
	ErrActualNotFound = errors.New("work order production actual not found")
	// ErrNotDeletable is returned when deleting a non-DRAFT work order is attempted.
	ErrNotDeletable = errors.New("cannot delete work order: only DRAFT work orders can be deleted")
)
