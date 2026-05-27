// Package costproductrequest implements the Phase A request aggregate root
// (PRD §7.1.1 CPR_ + §7.1.2 CPS_). State machine is hard-coded per G3 hybrid.
package costproductrequest

import (
	"fmt"
	"strings"
	"time"
)

// allowedUrgency / allowedClassification / allowedSubstatus checks.
var (
	allowedClassification = map[string]struct{}{ClassExisting: {}, ClassNew: {}}
	allowedUrgency        = map[string]struct{}{UrgencyLow: {}, UrgencyMedium: {}, UrgencyHigh: {}}
	allowedSubstatus      = map[string]struct{}{ClosedWon: {}, ClosedLost: {}, ClosedCancelled: {}, ClosedOnHold: {}}
)

// Request is the aggregate root.
type Request struct {
	requestID                    int64
	requestNo                    string // assigned by repo via generate_cost_request_no()
	requestTypeID                int32
	title                        string
	description                  string
	customerName                 string
	customerCode                 string
	productClassification        string
	verifiedClassification       string
	classificationOverrideReason string
	targetVolume                 string
	targetPriceRange             string
	urgencyLevel                 string
	neededByDate                 string // YYYY-MM-DD; empty = unset
	status                       string
	closedSubstatus              string
	feasibilityDecision          string
	feasibilityNote              string
	feasibilityBy                string
	feasibilityAt                *time.Time
	rejectReason                 string
	cancelReason                 string
	assignedToUserID             string
	requesterUserID              string
	// When UseExistingCosting is invoked, points to the reused product master.
	existingProductSysID int64
	// LinkedRouteHeadID is the FK to the unified routing head currently attached
	// to this request (0 = unlinked). Set by LinkRoute, cleared by UnlinkRoute.
	linkedRouteHeadID int64
	createdAt         time.Time
	updatedAt         time.Time

	// Optional embedded spec (when productClassification = new).
	spec *Spec
}

// NewInput is the create-time input.
type NewInput struct {
	RequestTypeID         int32
	Title                 string
	Description           string
	CustomerName          string
	CustomerCode          string
	ProductClassification string
	TargetVolume          string
	TargetPriceRange      string
	UrgencyLevel          string
	NeededByDate          string
	RequesterUserID       string
	Spec                  *SpecInput // required iff classification = new
}

// New constructs a request in the DRAFT state.
func New(in NewInput) (*Request, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, ErrInvalidTitle
	}
	if strings.TrimSpace(in.CustomerName) == "" {
		return nil, ErrInvalidCustomerName
	}
	if _, ok := allowedClassification[in.ProductClassification]; !ok {
		return nil, ErrInvalidClassification
	}
	urgency := in.UrgencyLevel
	if urgency == "" {
		urgency = UrgencyMedium
	}
	if _, ok := allowedUrgency[urgency]; !ok {
		return nil, ErrInvalidUrgency
	}
	// Spec presence rule.
	if in.ProductClassification == ClassNew && in.Spec == nil {
		return nil, ErrSpecRequired
	}
	if in.ProductClassification == ClassExisting && in.Spec != nil {
		return nil, ErrSpecNotAllowed
	}
	if in.Spec != nil {
		if err := in.Spec.Validate(); err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	r := &Request{
		requestTypeID:         in.RequestTypeID,
		title:                 strings.TrimSpace(in.Title),
		description:           strings.TrimSpace(in.Description),
		customerName:          strings.TrimSpace(in.CustomerName),
		customerCode:          strings.TrimSpace(in.CustomerCode),
		productClassification: in.ProductClassification,
		targetVolume:          strings.TrimSpace(in.TargetVolume),
		targetPriceRange:      strings.TrimSpace(in.TargetPriceRange),
		urgencyLevel:          urgency,
		neededByDate:          strings.TrimSpace(in.NeededByDate),
		status:                StatusDraft,
		requesterUserID:       in.RequesterUserID,
		createdAt:             now,
		updatedAt:             now,
	}
	if in.Spec != nil {
		s := in.Spec.ToSpec(in.RequesterUserID)
		r.spec = &s
	}
	return r, nil
}

// ReconstructInput rebuilds a Request from persistence (no validation).
type ReconstructInput struct {
	RequestID                    int64
	RequestNo                    string
	RequestTypeID                int32
	Title                        string
	Description                  string
	CustomerName                 string
	CustomerCode                 string
	ProductClassification        string
	VerifiedClassification       string
	ClassificationOverrideReason string
	TargetVolume                 string
	TargetPriceRange             string
	UrgencyLevel                 string
	NeededByDate                 string
	Status                       string
	ClosedSubstatus              string
	FeasibilityDecision          string
	FeasibilityNote              string
	FeasibilityBy                string
	FeasibilityAt                *time.Time
	RejectReason                 string
	CancelReason                 string
	AssignedToUserID             string
	RequesterUserID              string
	ExistingProductSysID         int64
	LinkedRouteHeadID            int64
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
	Spec                         *Spec
}

// Reconstruct rebuilds an aggregate from a persistence row.
func Reconstruct(in ReconstructInput) *Request {
	return &Request{
		requestID:                    in.RequestID,
		requestNo:                    in.RequestNo,
		requestTypeID:                in.RequestTypeID,
		title:                        in.Title,
		description:                  in.Description,
		customerName:                 in.CustomerName,
		customerCode:                 in.CustomerCode,
		productClassification:        in.ProductClassification,
		verifiedClassification:       in.VerifiedClassification,
		classificationOverrideReason: in.ClassificationOverrideReason,
		targetVolume:                 in.TargetVolume,
		targetPriceRange:             in.TargetPriceRange,
		urgencyLevel:                 in.UrgencyLevel,
		neededByDate:                 in.NeededByDate,
		status:                       in.Status,
		closedSubstatus:              in.ClosedSubstatus,
		feasibilityDecision:          in.FeasibilityDecision,
		feasibilityNote:              in.FeasibilityNote,
		feasibilityBy:                in.FeasibilityBy,
		feasibilityAt:                in.FeasibilityAt,
		rejectReason:                 in.RejectReason,
		cancelReason:                 in.CancelReason,
		assignedToUserID:             in.AssignedToUserID,
		requesterUserID:              in.RequesterUserID,
		existingProductSysID:         in.ExistingProductSysID,
		linkedRouteHeadID:            in.LinkedRouteHeadID,
		createdAt:                    in.CreatedAt,
		updatedAt:                    in.UpdatedAt,
		spec:                         in.Spec,
	}
}

// SetIDs is called by the repo after INSERT to assign DB-generated values.
func (r *Request) SetIDs(requestID int64, requestNo string) {
	r.requestID = requestID
	r.requestNo = requestNo
}

// SetSpecID is called by the repo after the spec row is INSERT-ed.
func (r *Request) SetSpecID(specID int64) {
	if r.spec != nil {
		r.spec.SpecID = specID
	}
}

// =============================================================================
// CRUD (DRAFT-only) update.
// =============================================================================

// UpdateInput is the DRAFT-mode update payload.
type UpdateInput struct {
	Title                 string
	Description           string
	CustomerName          string
	CustomerCode          string
	ProductClassification string
	TargetVolume          string
	TargetPriceRange      string
	UrgencyLevel          string
	NeededByDate          string
	Spec                  *SpecInput
}

// Update mutates DRAFT fields. Allowed only while status = DRAFT.
func (r *Request) Update(in UpdateInput) error {
	if r.status != StatusDraft {
		return ErrInvalidTransition
	}
	if strings.TrimSpace(in.Title) == "" {
		return ErrInvalidTitle
	}
	if strings.TrimSpace(in.CustomerName) == "" {
		return ErrInvalidCustomerName
	}
	if _, ok := allowedClassification[in.ProductClassification]; !ok {
		return ErrInvalidClassification
	}
	urgency := in.UrgencyLevel
	if urgency == "" {
		urgency = UrgencyMedium
	}
	if _, ok := allowedUrgency[urgency]; !ok {
		return ErrInvalidUrgency
	}
	if in.ProductClassification == ClassNew && in.Spec == nil {
		return ErrSpecRequired
	}
	if in.ProductClassification == ClassExisting && in.Spec != nil {
		return ErrSpecNotAllowed
	}
	if in.Spec != nil {
		if err := in.Spec.Validate(); err != nil {
			return err
		}
	}
	r.title = strings.TrimSpace(in.Title)
	r.description = strings.TrimSpace(in.Description)
	r.customerName = strings.TrimSpace(in.CustomerName)
	r.customerCode = strings.TrimSpace(in.CustomerCode)
	r.productClassification = in.ProductClassification
	r.targetVolume = strings.TrimSpace(in.TargetVolume)
	r.targetPriceRange = strings.TrimSpace(in.TargetPriceRange)
	r.urgencyLevel = urgency
	r.neededByDate = strings.TrimSpace(in.NeededByDate)
	r.touch()
	if in.Spec == nil {
		r.spec = nil
		return nil
	}
	creator := r.requesterUserID
	if r.spec != nil {
		creator = r.spec.CreatedBy
	}
	s := in.Spec.ToSpec(creator)
	r.spec = &s
	return nil
}

// =============================================================================
// State transitions (hard-coded per G3).
// =============================================================================

// Submit transitions DRAFT → SUBMITTED.
func (r *Request) Submit() error {
	if !canTransition(r.status, StatusSubmitted) {
		return ErrInvalidTransition
	}
	r.status = StatusSubmitted
	r.touch()
	return nil
}

// StartReview transitions SUBMITTED → UNDER_REVIEW.
func (r *Request) StartReview() error {
	if !canTransition(r.status, StatusUnderReview) {
		return ErrInvalidTransition
	}
	r.status = StatusUnderReview
	r.touch()
	return nil
}

// VerifyClassification sets verified_classification + (required) override_reason if it differs.
// Does NOT advance state on its own.
func (r *Request) VerifyClassification(verified, reason string) error {
	if _, ok := allowedClassification[verified]; !ok {
		return ErrInvalidVerified
	}
	if verified != r.productClassification && strings.TrimSpace(reason) == "" {
		return ErrOverrideReasonRequired
	}
	r.verifiedClassification = verified
	if verified != r.productClassification {
		r.classificationOverrideReason = strings.TrimSpace(reason)
	} else {
		r.classificationOverrideReason = ""
	}
	r.touch()
	return nil
}

// DecideFeasibility transitions UNDER_REVIEW → ROUTING_DEFINED (FEASIBLE) or REJECTED (NOT_FEASIBLE).
func (r *Request) DecideFeasibility(decision, note, actor string) error {
	if r.status != StatusUnderReview {
		return ErrInvalidTransition
	}
	switch decision {
	case FeasibilityFeasible:
		if !canTransition(r.status, StatusRoutingDefined) {
			return ErrInvalidTransition
		}
		r.feasibilityDecision = FeasibilityFeasible
		r.feasibilityNote = strings.TrimSpace(note)
		r.feasibilityBy = actor
		now := time.Now().UTC()
		r.feasibilityAt = &now
		r.status = StatusRoutingDefined
	case FeasibilityNotFeasible:
		if strings.TrimSpace(note) == "" {
			return ErrFeasibilityNoteMissing
		}
		if !canTransition(r.status, StatusRejected) {
			return ErrInvalidTransition
		}
		r.feasibilityDecision = FeasibilityNotFeasible
		r.feasibilityNote = strings.TrimSpace(note)
		r.feasibilityBy = actor
		now := time.Now().UTC()
		r.feasibilityAt = &now
		r.status = StatusRejected
		r.rejectReason = strings.TrimSpace(note)
	default:
		return ErrInvalidFeasibility
	}
	r.touch()
	return nil
}

// UseExistingCosting transitions UNDER_REVIEW → QUOTE_READY (verified must be existing).
// existingProductSysID is recorded so the QUOTE_READY state traces back to a
// concrete cost_product_master.
func (r *Request) UseExistingCosting(existingProductSysID int64) error {
	if r.status != StatusUnderReview {
		return ErrInvalidTransition
	}
	if r.verifiedClassification != ClassExisting {
		return ErrInvalidTransition
	}
	if existingProductSysID <= 0 {
		return ErrExistingProductRequired
	}
	if !canTransition(r.status, StatusQuoteReady) {
		return ErrInvalidTransition
	}
	r.existingProductSysID = existingProductSysID
	r.status = StatusQuoteReady
	r.touch()
	return nil
}

// ExistingProductSysID returns the FK to cost_product_master (0 = none).
func (r *Request) ExistingProductSysID() int64 { return r.existingProductSysID }

// LinkedRouteHeadID returns the linked route head id or 0 if not linked.
func (r *Request) LinkedRouteHeadID() int64 { return r.linkedRouteHeadID }

// LinkRoute attaches a route head to this request. Allowed only while the request
// is still in a pre-terminal state. Idempotent re-link is allowed.
func (r *Request) LinkRoute(headID int64) error {
	if headID <= 0 {
		return fmt.Errorf("link route: invalid head id %d", headID)
	}
	switch r.status {
	case StatusDraft, StatusSubmitted, StatusUnderReview,
		StatusRoutingDefined, StatusParameterPending, StatusParameterComplete:
		r.linkedRouteHeadID = headID
		r.touch()
		return nil
	}
	return ErrInvalidTransition
}

// UnlinkRoute clears the linked route head. Allowed in any non-terminal state.
func (r *Request) UnlinkRoute() error {
	switch r.status {
	case StatusCostingDone, StatusRejected, StatusClosed:
		return ErrInvalidTransition
	}
	r.linkedRouteHeadID = 0
	r.touch()
	return nil
}

// MarkParameterPending advances ROUTING_DEFINED → PARAMETER_PENDING. Invoked
// automatically by PromoteHandler once at least one routing draft is promoted,
// so the request enters the per-product param-fill stage without a manual click.
func (r *Request) MarkParameterPending() error {
	if r.status != StatusRoutingDefined {
		return ErrInvalidTransition
	}
	if !canTransition(r.status, StatusParameterPending) {
		return ErrInvalidTransition
	}
	r.status = StatusParameterPending
	r.touch()
	return nil
}

// MarkParameterComplete advances PARAMETER_PENDING → PARAMETER_COMPLETE. The
// caller is responsible for verifying that no required params are missing via
// cost_product_parameter.CheckMissingRequiredParams BEFORE invoking this.
func (r *Request) MarkParameterComplete() error {
	if r.status != StatusParameterPending {
		return ErrInvalidTransition
	}
	if !canTransition(r.status, StatusParameterComplete) {
		return ErrInvalidTransition
	}
	r.status = StatusParameterComplete
	r.touch()
	return nil
}

// Reject sends to REJECTED from SUBMITTED or UNDER_REVIEW with a reason.
func (r *Request) Reject(reason string) error {
	if strings.TrimSpace(reason) == "" {
		return ErrInvalidTransition
	}
	if !canTransition(r.status, StatusRejected) {
		return ErrInvalidTransition
	}
	r.status = StatusRejected
	r.rejectReason = strings.TrimSpace(reason)
	r.touch()
	return nil
}

// Revise transitions REJECTED → SUBMITTED (re-submit after fixing).
func (r *Request) Revise() error {
	if !canTransition(r.status, StatusSubmitted) {
		return ErrInvalidTransition
	}
	r.status = StatusSubmitted
	// Clear reject reason so the new cycle is clean.
	r.rejectReason = ""
	r.touch()
	return nil
}

// Reopen moves a CLOSED request back to DRAFT so it can re-enter the lifecycle.
// Clears the closed substatus + cancel reason so the new cycle starts clean.
func (r *Request) Reopen() error {
	if !canTransition(r.status, StatusDraft) {
		return ErrInvalidTransition
	}
	r.status = StatusDraft
	r.closedSubstatus = ""
	r.cancelReason = ""
	r.touch()
	return nil
}

// Cancel from any non-CLOSED status with a reason → CLOSED:cancelled.
func (r *Request) Cancel(reason string) error {
	if strings.TrimSpace(reason) == "" {
		return ErrInvalidTransition
	}
	if !canTransition(r.status, StatusClosed) {
		return ErrInvalidTransition
	}
	r.status = StatusClosed
	r.closedSubstatus = ClosedCancelled
	r.cancelReason = strings.TrimSpace(reason)
	r.touch()
	return nil
}

// Close sets the closed substatus (won/lost/cancelled/on_hold) from non-terminal states.
func (r *Request) Close(substatus string) error {
	if _, ok := allowedSubstatus[substatus]; !ok {
		return ErrInvalidSubstatus
	}
	if !canTransition(r.status, StatusClosed) {
		return ErrInvalidTransition
	}
	r.status = StatusClosed
	r.closedSubstatus = substatus
	r.touch()
	return nil
}

// Assign sets the assignee user id. Does NOT change state.
func (r *Request) Assign(assignee string) error {
	if strings.TrimSpace(assignee) == "" {
		return ErrInvalidTransition
	}
	r.assignedToUserID = strings.TrimSpace(assignee)
	r.touch()
	return nil
}

func (r *Request) touch() { r.updatedAt = time.Now().UTC() }

// =============================================================================
// Accessors (immutable view).
// =============================================================================

// RequestID returns the request id.
func (r *Request) RequestID() int64 { return r.requestID }

// RequestNo returns the request no.
func (r *Request) RequestNo() string { return r.requestNo }

// RequestTypeID returns the request type id.
func (r *Request) RequestTypeID() int32 { return r.requestTypeID }

// Title returns the title.
func (r *Request) Title() string { return r.title }

// Description returns the description.
func (r *Request) Description() string { return r.description }

// CustomerName returns the customer name.
func (r *Request) CustomerName() string { return r.customerName }

// CustomerCode returns the customer code.
func (r *Request) CustomerCode() string { return r.customerCode }

// ProductClassification returns the product classification.
func (r *Request) ProductClassification() string { return r.productClassification }

// VerifiedClassification returns the verified classification.
func (r *Request) VerifiedClassification() string { return r.verifiedClassification }

// ClassificationOverrideReason returns the classification override reason.
func (r *Request) ClassificationOverrideReason() string { return r.classificationOverrideReason }

// TargetVolume returns the target volume.
func (r *Request) TargetVolume() string { return r.targetVolume }

// TargetPriceRange returns the target price range.
func (r *Request) TargetPriceRange() string { return r.targetPriceRange }

// UrgencyLevel returns the urgency level.
func (r *Request) UrgencyLevel() string { return r.urgencyLevel }

// NeededByDate returns the needed by date.
func (r *Request) NeededByDate() string { return r.neededByDate }

// Status returns the status.
func (r *Request) Status() string { return r.status }

// ClosedSubstatus returns the closed substatus.
func (r *Request) ClosedSubstatus() string { return r.closedSubstatus }

// FeasibilityDecision returns the feasibility decision.
func (r *Request) FeasibilityDecision() string { return r.feasibilityDecision }

// FeasibilityNote returns the feasibility note.
func (r *Request) FeasibilityNote() string { return r.feasibilityNote }

// FeasibilityBy returns the feasibility by.
func (r *Request) FeasibilityBy() string { return r.feasibilityBy }

// FeasibilityAt returns the feasibility at.
func (r *Request) FeasibilityAt() *time.Time { return r.feasibilityAt }

// RejectReason returns the reject reason.
func (r *Request) RejectReason() string { return r.rejectReason }

// CancelReason returns the cancel reason.
func (r *Request) CancelReason() string { return r.cancelReason }

// AssignedToUserID returns the assigned to user id.
func (r *Request) AssignedToUserID() string { return r.assignedToUserID }

// RequesterUserID returns the requester user id.
func (r *Request) RequesterUserID() string { return r.requesterUserID }

// CreatedAt returns the created at.
func (r *Request) CreatedAt() time.Time { return r.createdAt }

// UpdatedAt returns the updated at.
func (r *Request) UpdatedAt() time.Time { return r.updatedAt }

// Spec returns the spec.
func (r *Request) Spec() *Spec { return r.spec }
