package costproductrequest

import "errors"

var (
	// ErrNotFound is returned when a request is missing.
	ErrNotFound = errors.New("cost product request not found")
	// ErrAlreadyExists is returned on request_no collision.
	ErrAlreadyExists = errors.New("cost product request already exists")
	// ErrInvalidTitle is returned for a blank or oversized title.
	ErrInvalidTitle = errors.New("invalid title")
	// ErrInvalidCustomerName is returned for a blank or oversized customer name.
	ErrInvalidCustomerName = errors.New("invalid customer_name")
	// ErrInvalidClassification is returned for a bad product_classification.
	ErrInvalidClassification = errors.New("invalid product_classification (must be existing|new)")
	// ErrInvalidUrgency is returned for a bad urgency_level.
	ErrInvalidUrgency = errors.New("invalid urgency_level (must be low|medium|high)")
	// ErrInvalidVerified is returned for a bad verified_classification.
	ErrInvalidVerified = errors.New("invalid verified_classification (must be existing|new)")
	// ErrOverrideReasonRequired is returned when an override reason is missing.
	ErrOverrideReasonRequired = errors.New("override reason required when verified ≠ marketing classification")
	// ErrInvalidFeasibility is returned for a bad feasibility decision.
	ErrInvalidFeasibility = errors.New("invalid feasibility decision (must be FEASIBLE|NOT_FEASIBLE)")
	// ErrFeasibilityNoteMissing is returned when a NOT_FEASIBLE decision lacks a note.
	ErrFeasibilityNoteMissing = errors.New("feasibility note required when decision = NOT_FEASIBLE")
	// ErrInvalidSubstatus is returned for a bad closed_substatus.
	ErrInvalidSubstatus = errors.New("invalid closed_substatus (must be won|lost|cancelled|on_hold)")
	// ErrSpecRequired is returned when a new-product request omits its spec.
	ErrSpecRequired = errors.New("spec required when product_classification = new")
	// ErrSpecNotAllowed is returned when an existing-product request carries a spec.
	ErrSpecNotAllowed = errors.New("spec not allowed when product_classification = existing")
	// ErrInvalidSpec is returned for an invalid spec input.
	ErrInvalidSpec = errors.New("invalid spec input")
	// ErrInvalidTransition is returned when a state machine transition is rejected.
	ErrInvalidTransition = errors.New("invalid state transition")
	// ErrExistingProductRequired is returned when UseExistingCosting is called
	// without specifying which product master the request reuses.
	ErrExistingProductRequired = errors.New("existing product is required to use existing costing")
)
