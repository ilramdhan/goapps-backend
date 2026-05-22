package costproductrequest

import "errors"

var (
	// ErrNotFound is returned when a request is missing.
	ErrNotFound = errors.New("cost product request not found")
	// ErrAlreadyExists is returned on request_no collision.
	ErrAlreadyExists = errors.New("cost product request already exists")
	// ErrInvalidTitle / ErrInvalidCustomerName / ErrInvalidClassification are input validation errors.
	ErrInvalidTitle           = errors.New("invalid title")
	ErrInvalidCustomerName    = errors.New("invalid customer_name")
	ErrInvalidClassification  = errors.New("invalid product_classification (must be existing|new)")
	ErrInvalidUrgency         = errors.New("invalid urgency_level (must be low|medium|high)")
	ErrInvalidVerified        = errors.New("invalid verified_classification (must be existing|new)")
	ErrOverrideReasonRequired = errors.New("override reason required when verified ≠ marketing classification")
	ErrInvalidFeasibility     = errors.New("invalid feasibility decision (must be FEASIBLE|NOT_FEASIBLE)")
	ErrFeasibilityNoteMissing = errors.New("feasibility note required when decision = NOT_FEASIBLE")
	ErrInvalidSubstatus       = errors.New("invalid closed_substatus (must be won|lost|cancelled|on_hold)")
	ErrSpecRequired           = errors.New("spec required when product_classification = new")
	ErrSpecNotAllowed         = errors.New("spec not allowed when product_classification = existing")
	ErrInvalidSpec            = errors.New("invalid spec input")
	// ErrInvalidTransition is returned when a state machine transition is rejected.
	ErrInvalidTransition = errors.New("invalid state transition")
	// ErrExistingProductRequired is returned when UseExistingCosting is called
	// without specifying which product master the request reuses.
	ErrExistingProductRequired = errors.New("existing product is required to use existing costing")
)
