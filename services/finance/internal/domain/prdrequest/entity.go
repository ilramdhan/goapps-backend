// Package prdrequest contains the ProductRequest aggregate (ticket) and supporting types.
package prdrequest

import (
	"time"

	"github.com/google/uuid"
)

// Request is the aggregate root for a product request ticket.
// All fields are private; use getters to read state.
// Pointer getters (DueDate, UpdatedAt, DeletedAt) return the stored pointer directly
// — callers must not mutate the pointed-to value.
//
//nolint:revive // Wide struct mirrors the persistence row one-for-one.
type Request struct {
	id                uuid.UUID
	ticketNo          TicketNo
	requesterID       uuid.UUID
	requesterUsername string
	requesterDeptID   uuid.UUID
	requesterDeptCode string
	title             Title
	description       Description
	targetSpecs       TargetSpecs
	status            TicketStatus
	resolvedProductID uuid.UUID     // zero UUID = none
	resolutionNote    ResolutionNote
	rejectReason      RejectReason  // zero string when not rejected
	assignedTo        uuid.UUID     // zero UUID = none
	dueDate           *time.Time
	createdAt         time.Time
	createdBy         string
	updatedAt         *time.Time
	updatedBy         string
	deletedAt         *time.Time
	deletedBy         string
}

// NewRequest creates a fresh ticket in OPEN status. ticketNo MUST be allocated by
// the caller via TicketNoGenerator before invoking this constructor.
//
//nolint:revive // Constructor takes many fields to fully initialize the aggregate.
func NewRequest(
	ticketNo TicketNo,
	requesterID uuid.UUID,
	requesterUsername string,
	requesterDeptID uuid.UUID,
	requesterDeptCode string,
	title, description, targetSpecsJSON string,
	dueDate *time.Time,
	createdBy string,
) (*Request, error) {
	if requesterID == uuid.Nil {
		return nil, ErrInvalidRequester
	}
	if requesterDeptID == uuid.Nil {
		return nil, ErrInvalidRequester
	}

	titleVO, err := NewTitle(title)
	if err != nil {
		return nil, err
	}

	descVO, err := NewDescription(description)
	if err != nil {
		return nil, err
	}

	specsVO, err := NewTargetSpecs(targetSpecsJSON)
	if err != nil {
		return nil, err
	}

	return &Request{
		id:                uuid.New(),
		ticketNo:          ticketNo,
		requesterID:       requesterID,
		requesterUsername: requesterUsername,
		requesterDeptID:   requesterDeptID,
		requesterDeptCode: requesterDeptCode,
		title:             titleVO,
		description:       descVO,
		targetSpecs:       specsVO,
		status:            StatusOpen,
		dueDate:           dueDate,
		createdAt:         time.Now().UTC(),
		createdBy:         createdBy,
	}, nil
}

// ReconstructRequest rebuilds an entity from raw persisted values.
// It does NOT re-run validation — the persistence layer is the source of truth.
//
//nolint:revive // Persistence reconstitution takes many fields by design.
func ReconstructRequest(
	id uuid.UUID,
	ticketNo string,
	requesterID uuid.UUID,
	requesterUsername string,
	requesterDeptID uuid.UUID,
	requesterDeptCode string,
	title, description, targetSpecsJSON string,
	status string,
	resolvedProductID uuid.UUID,
	resolutionNote string,
	rejectReason string,
	assignedTo uuid.UUID,
	dueDate *time.Time,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy string,
	deletedAt *time.Time,
	deletedBy string,
) *Request {
	return &Request{
		id:                id,
		ticketNo:          TicketNo{value: ticketNo},
		requesterID:       requesterID,
		requesterUsername: requesterUsername,
		requesterDeptID:   requesterDeptID,
		requesterDeptCode: requesterDeptCode,
		title:             Title{value: title},
		description:       Description{value: description},
		targetSpecs:       TargetSpecs{value: targetSpecsJSON},
		status:            TicketStatus(status),
		resolvedProductID: resolvedProductID,
		resolutionNote:    ResolutionNote{value: resolutionNote},
		rejectReason:      RejectReason{value: rejectReason},
		assignedTo:        assignedTo,
		dueDate:           dueDate,
		createdAt:         createdAt,
		createdBy:         createdBy,
		updatedAt:         updatedAt,
		updatedBy:         updatedBy,
		deletedAt:         deletedAt,
		deletedBy:         deletedBy,
	}
}

// Update mutates editable header fields on the request.
// Not allowed when the ticket is in a terminal status (RESOLVED or REJECTED).
func (r *Request) Update(
	title, description, targetSpecsJSON string,
	dueDate *time.Time,
	updatedBy string,
) error {
	if r.status.IsTerminal() {
		return ErrInvalidTransition
	}

	if err := r.applyTitle(title); err != nil {
		return err
	}
	if err := r.applyDescription(description); err != nil {
		return err
	}
	if err := r.applyTargetSpecs(targetSpecsJSON); err != nil {
		return err
	}

	r.dueDate = dueDate
	r.bumpAudit(updatedBy)
	return nil
}

// applyTitle validates and assigns the title field.
func (r *Request) applyTitle(title string) error {
	vo, err := NewTitle(title)
	if err != nil {
		return err
	}
	r.title = vo
	return nil
}

// applyDescription validates and assigns the description field.
func (r *Request) applyDescription(description string) error {
	vo, err := NewDescription(description)
	if err != nil {
		return err
	}
	r.description = vo
	return nil
}

// applyTargetSpecs validates and assigns the target specs field.
func (r *Request) applyTargetSpecs(targetSpecsJSON string) error {
	vo, err := NewTargetSpecs(targetSpecsJSON)
	if err != nil {
		return err
	}
	r.targetSpecs = vo
	return nil
}

// Assign sets the assignee for this request.
// Only allowed when the status is non-terminal. Auto-transitions OPEN to IN_REVIEW.
func (r *Request) Assign(assigneeID uuid.UUID, updatedBy string) error {
	if !r.status.CanAssign() {
		return ErrCannotAssign
	}
	if assigneeID == uuid.Nil {
		return ErrInvalidAssignee
	}

	r.assignedTo = assigneeID
	if r.status == StatusOpen {
		r.status = StatusInReview
	}
	r.bumpAudit(updatedBy)
	return nil
}

// Resolve links the request to a resolved product and closes it as RESOLVED.
// Returns ErrAlreadyResolved if already resolved, ErrAlreadyRejected if rejected,
// ErrCannotResolve if in another terminal state (future-proofing), or ErrInvalidProductLink
// if productID is zero.
func (r *Request) Resolve(productID uuid.UUID, note string, updatedBy string) error {
	if r.status == StatusResolved {
		return ErrAlreadyResolved
	}
	if r.status == StatusRejected {
		return ErrAlreadyRejected
	}
	if !r.status.CanResolve() {
		return ErrCannotResolve
	}
	if productID == uuid.Nil {
		return ErrInvalidProductLink
	}

	rn, err := NewResolutionNote(note)
	if err != nil {
		return err
	}

	r.status = StatusResolved
	r.resolvedProductID = productID
	r.resolutionNote = rn
	r.bumpAudit(updatedBy)
	return nil
}

// Reject closes the request as REJECTED with a mandatory reason.
// Returns ErrAlreadyRejected if already rejected, ErrAlreadyResolved if resolved, or
// ErrCannotReject if in another terminal state (future-proofing).
func (r *Request) Reject(reason string, updatedBy string) error {
	if r.status == StatusRejected {
		return ErrAlreadyRejected
	}
	if r.status == StatusResolved {
		return ErrAlreadyResolved
	}
	if !r.status.CanReject() {
		return ErrCannotReject
	}

	rr, err := NewRejectReason(reason)
	if err != nil {
		return err
	}

	r.status = StatusRejected
	r.rejectReason = rr
	r.bumpAudit(updatedBy)
	return nil
}

// SoftDelete marks the request as deleted. Returns ErrNotFound if already deleted.
func (r *Request) SoftDelete(deletedBy string) error {
	if r.deletedAt != nil {
		return ErrNotFound
	}

	now := time.Now().UTC()
	r.deletedAt = &now
	r.deletedBy = deletedBy
	return nil
}

// bumpAudit sets updatedAt to now and updatedBy to the supplied identity.
func (r *Request) bumpAudit(updatedBy string) {
	now := time.Now().UTC()
	r.updatedAt = &now
	r.updatedBy = updatedBy
}

// =============================================================================
// Read-only getters
// =============================================================================

// ID returns the request's unique identifier.
func (r *Request) ID() uuid.UUID { return r.id }

// TicketNo returns the human-readable ticket number.
func (r *Request) TicketNo() TicketNo { return r.ticketNo }

// RequesterID returns the UUID of the user who created the request.
func (r *Request) RequesterID() uuid.UUID { return r.requesterID }

// RequesterUsername returns the username of the requester.
func (r *Request) RequesterUsername() string { return r.requesterUsername }

// RequesterDeptID returns the UUID of the requester's department.
func (r *Request) RequesterDeptID() uuid.UUID { return r.requesterDeptID }

// RequesterDeptCode returns the code of the requester's department.
func (r *Request) RequesterDeptCode() string { return r.requesterDeptCode }

// Title returns the request title value object.
func (r *Request) Title() Title { return r.title }

// Description returns the request description value object.
func (r *Request) Description() Description { return r.description }

// TargetSpecs returns the raw JSON target specifications value object.
func (r *Request) TargetSpecs() TargetSpecs { return r.targetSpecs }

// Status returns the current lifecycle status of the ticket.
func (r *Request) Status() TicketStatus { return r.status }

// ResolvedProductID returns the UUID of the product linked at resolution.
// Returns uuid.Nil when the request has not been resolved.
func (r *Request) ResolvedProductID() uuid.UUID { return r.resolvedProductID }

// ResolutionNote returns the note attached at resolution.
func (r *Request) ResolutionNote() ResolutionNote { return r.resolutionNote }

// RejectReason returns the reason supplied when the request was rejected.
func (r *Request) RejectReason() RejectReason { return r.rejectReason }

// AssignedTo returns the UUID of the current assignee.
// Returns uuid.Nil when no assignee has been set.
func (r *Request) AssignedTo() uuid.UUID { return r.assignedTo }

// DueDate returns the optional due-date pointer.
// Callers must not mutate the pointed-to value.
func (r *Request) DueDate() *time.Time { return r.dueDate }

// CreatedAt returns the creation timestamp.
func (r *Request) CreatedAt() time.Time { return r.createdAt }

// CreatedBy returns the identity that created the request.
func (r *Request) CreatedBy() string { return r.createdBy }

// UpdatedAt returns the last-update timestamp, or nil if never updated.
// Callers must not mutate the pointed-to value.
func (r *Request) UpdatedAt() *time.Time { return r.updatedAt }

// UpdatedBy returns the identity that last updated the request.
func (r *Request) UpdatedBy() string { return r.updatedBy }

// DeletedAt returns the soft-delete timestamp, or nil if not deleted.
// Callers must not mutate the pointed-to value.
func (r *Request) DeletedAt() *time.Time { return r.deletedAt }

// DeletedBy returns the identity that soft-deleted the request.
func (r *Request) DeletedBy() string { return r.deletedBy }

// IsDeleted reports whether the request has been soft-deleted.
func (r *Request) IsDeleted() bool { return r.deletedAt != nil }
