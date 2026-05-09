// Package prdrequest contains the ProductRequest aggregate (ticket) and supporting types.
package prdrequest

import "errors"

// Sentinel errors returned by the prdrequest domain.
var (
	// ErrNotFound is returned when a product request is not found.
	ErrNotFound = errors.New("product request not found")

	// ErrInvalidTitle is returned when the request title is invalid.
	ErrInvalidTitle = errors.New("invalid request title")

	// ErrInvalidDescription is returned when the request description is invalid.
	ErrInvalidDescription = errors.New("invalid request description")

	// ErrInvalidTargetSpecs is returned when the target specs JSON is invalid.
	ErrInvalidTargetSpecs = errors.New("invalid target specs")

	// ErrInvalidStatus is returned when the ticket status value is invalid.
	ErrInvalidStatus = errors.New("invalid request status")

	// ErrInvalidTransition is returned when a state transition is not allowed.
	ErrInvalidTransition = errors.New("invalid status transition")

	// ErrInvalidTicketNo is returned when the ticket number format is invalid.
	ErrInvalidTicketNo = errors.New("invalid ticket number")

	// ErrInvalidPeriod is returned when the period string format is invalid.
	ErrInvalidPeriod = errors.New("invalid period")

	// ErrAlreadyResolved is returned when the request is already in RESOLVED state.
	ErrAlreadyResolved = errors.New("request already resolved")

	// ErrAlreadyRejected is returned when the request is already in REJECTED state.
	ErrAlreadyRejected = errors.New("request already rejected")

	// ErrCannotAssign is returned when an assignee cannot be set in the current status.
	ErrCannotAssign = errors.New("cannot assign in current status")

	// ErrCannotResolve is returned when the request cannot be resolved in the current status.
	ErrCannotResolve = errors.New("cannot resolve in current status")

	// ErrCannotReject is returned when the request cannot be rejected in the current status.
	ErrCannotReject = errors.New("cannot reject in current status")

	// ErrInvalidResolution is returned when the resolution note is invalid.
	ErrInvalidResolution = errors.New("invalid resolution note")

	// ErrInvalidRejectReason is returned when the reject reason is invalid.
	ErrInvalidRejectReason = errors.New("invalid reject reason")

	// ErrInvalidRequester is returned when the requester identity is invalid.
	ErrInvalidRequester = errors.New("invalid requester")

	// ErrInvalidAssignee is returned when the assignee identity is invalid.
	ErrInvalidAssignee = errors.New("invalid assignee")

	// ErrInvalidProductLink is returned when the resolved product ID is invalid.
	ErrInvalidProductLink = errors.New("invalid product link")
)
