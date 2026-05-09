// Package prdrequest contains the ProductRequest aggregate (ticket) and supporting types.
package prdrequest

import (
	"regexp"
	"strconv"
	"strings"
)

// =============================================================================
// Title Value Object
// =============================================================================

// Title represents a validated product-request title.
type Title struct {
	value string
}

// NewTitle creates a validated Title value object.
// The title must be 3–200 characters (trimmed, non-whitespace-only).
func NewTitle(s string) (Title, error) {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) < 3 {
		return Title{}, ErrInvalidTitle
	}
	if len(s) > 200 {
		return Title{}, ErrInvalidTitle
	}
	return Title{value: s}, nil
}

// String returns the string representation of the title.
func (t Title) String() string { return t.value }

// =============================================================================
// Description Value Object
// =============================================================================

// Description represents an optional product-request description.
// An empty value is valid and indicates no description has been set.
type Description struct {
	value string
}

// NewDescription creates a Description value object.
// An empty string is valid. Max 5000 characters.
func NewDescription(s string) (Description, error) {
	if len(s) > 5000 {
		return Description{}, ErrInvalidDescription
	}
	return Description{value: s}, nil
}

// String returns the string representation of the description.
func (d Description) String() string { return d.value }

// IsEmpty reports whether the description is unset.
func (d Description) IsEmpty() bool { return d.value == "" }

// =============================================================================
// ResolutionNote Value Object
// =============================================================================

// ResolutionNote represents an optional note attached when a request is resolved.
// An empty value is valid.
type ResolutionNote struct {
	value string
}

// NewResolutionNote creates a ResolutionNote value object.
// An empty string is valid. Max 1000 characters.
func NewResolutionNote(s string) (ResolutionNote, error) {
	if len(s) > 1000 {
		return ResolutionNote{}, ErrInvalidResolution
	}
	return ResolutionNote{value: s}, nil
}

// String returns the string representation of the resolution note.
func (r ResolutionNote) String() string { return r.value }

// IsEmpty reports whether the resolution note is unset.
func (r ResolutionNote) IsEmpty() bool { return r.value == "" }

// =============================================================================
// RejectReason Value Object
// =============================================================================

// RejectReason represents the mandatory reason supplied when rejecting a request.
type RejectReason struct {
	value string
}

// NewRejectReason creates a RejectReason value object.
// The reason must be 5–1000 characters.
func NewRejectReason(s string) (RejectReason, error) {
	if len(strings.TrimSpace(s)) < 5 {
		return RejectReason{}, ErrInvalidRejectReason
	}
	if len(s) > 1000 {
		return RejectReason{}, ErrInvalidRejectReason
	}
	return RejectReason{value: s}, nil
}

// String returns the string representation of the reject reason.
func (r RejectReason) String() string { return r.value }

// IsEmpty reports whether the reject reason is unset.
func (r RejectReason) IsEmpty() bool { return r.value == "" }

// =============================================================================
// TargetSpecs Value Object
// =============================================================================

// TargetSpecs wraps a raw JSON string describing the desired product specifications.
// The domain does not parse the JSON — that is the caller's concern.
type TargetSpecs struct {
	value string
}

// NewTargetSpecs creates a TargetSpecs value object.
// An empty string is valid (no specs provided). Max 10000 characters.
func NewTargetSpecs(s string) (TargetSpecs, error) {
	if len(s) > 10000 {
		return TargetSpecs{}, ErrInvalidTargetSpecs
	}
	return TargetSpecs{value: s}, nil
}

// String returns the raw JSON string.
func (ts TargetSpecs) String() string { return ts.value }

// IsEmpty reports whether no target specs have been provided.
func (ts TargetSpecs) IsEmpty() bool { return ts.value == "" }

// =============================================================================
// TicketStatus Value Object
// =============================================================================

// TicketStatus is the lifecycle status of a product-request ticket.
type TicketStatus string

const (
	// StatusOpen indicates the request has been created and is awaiting triage.
	StatusOpen TicketStatus = "OPEN"

	// StatusInReview indicates the request is under active review by an assignee.
	StatusInReview TicketStatus = "IN_REVIEW"

	// StatusProductProposed indicates a matching product has been proposed.
	StatusProductProposed TicketStatus = "PRODUCT_PROPOSED"

	// StatusResolved indicates the request has been closed by linking a product.
	StatusResolved TicketStatus = "RESOLVED"

	// StatusRejected indicates the request has been declined.
	StatusRejected TicketStatus = "REJECTED"
)

// NewTicketStatus parses and validates a TicketStatus from its string representation.
func NewTicketStatus(s string) (TicketStatus, error) {
	ts := TicketStatus(s)
	if !ts.IsValid() {
		return "", ErrInvalidStatus
	}
	return ts, nil
}

// IsValid reports whether the TicketStatus is one of the recognized values.
func (ts TicketStatus) IsValid() bool {
	switch ts {
	case StatusOpen, StatusInReview, StatusProductProposed, StatusResolved, StatusRejected:
		return true
	default:
		return false
	}
}

// String returns the canonical string form of the ticket status.
func (ts TicketStatus) String() string { return string(ts) }

// IsTerminal reports whether the status is a terminal state (no further transitions).
// RESOLVED and REJECTED are terminal.
func (ts TicketStatus) IsTerminal() bool {
	return ts == StatusResolved || ts == StatusRejected
}

// CanAssign reports whether an assignee may be set in this status.
// Any non-terminal status allows assignment.
func (ts TicketStatus) CanAssign() bool { return !ts.IsTerminal() }

// CanResolve reports whether the request may be resolved in this status.
// Any non-terminal status allows resolution.
func (ts TicketStatus) CanResolve() bool { return !ts.IsTerminal() }

// CanReject reports whether the request may be rejected in this status.
// Any non-terminal status allows rejection.
func (ts TicketStatus) CanReject() bool { return !ts.IsTerminal() }

// =============================================================================
// TicketNo Value Object
// =============================================================================

// ticketNoPattern matches PR-YYYYMM-NNN (allowing 3+ digit sequence numbers).
var ticketNoPattern = regexp.MustCompile(`^PR-\d{6}-\d{3,}$`)

// TicketNo is the unique human-readable ticket identifier (e.g., PR-202504-001).
type TicketNo struct {
	value string
}

// NewTicketNo creates a TicketNo value object.
// The string must match the format PR-YYYYMM-NNN (3 or more sequence digits).
func NewTicketNo(s string) (TicketNo, error) {
	if !ticketNoPattern.MatchString(s) {
		return TicketNo{}, ErrInvalidTicketNo
	}
	return TicketNo{value: s}, nil
}

// String returns the string representation of the ticket number.
func (tn TicketNo) String() string { return tn.value }

// periodPattern matches a 6-digit period string (YYYYMM).
var periodPattern = regexp.MustCompile(`^\d{6}$`)

// ParseTicketNo extracts the period (YYYYMM) and sequence number from a ticket number string.
// Returns ErrInvalidTicketNo if the format is not valid.
func ParseTicketNo(s string) (period string, seq int, err error) {
	if !ticketNoPattern.MatchString(s) {
		return "", 0, ErrInvalidTicketNo
	}
	// Format: PR-YYYYMM-NNN
	// s[3:9]  = YYYYMM
	// s[10:]  = NNN...
	period = s[3:9]
	seq, err = strconv.Atoi(s[10:])
	if err != nil {
		return "", 0, ErrInvalidTicketNo
	}
	return period, seq, nil
}
