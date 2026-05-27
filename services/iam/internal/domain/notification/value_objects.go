// Package notification provides domain logic for the generic per-user
// notification system used by IAM and emitted-into by other services.
package notification

import "fmt"

// Type categorizes a notification.
type Type string

// Notification type values — must match chk_notif_type CHECK constraint.
const (
	// TypeExportReady signals a generated file is ready for download.
	TypeExportReady Type = "EXPORT_READY"
	// TypeAlert signals a system alert (warning/error).
	TypeAlert Type = "ALERT"
	// TypeApproval is an approval request awaiting recipient action.
	TypeApproval Type = "APPROVAL"
	// TypeChat is a direct chat or comment from another user.
	TypeChat Type = "CHAT"
	// TypeReminder is a time-based reminder.
	TypeReminder Type = "REMINDER"
	// TypeSystem is a generic system notification.
	TypeSystem Type = "SYSTEM"
	// TypeMention indicates the user was mentioned somewhere.
	TypeMention Type = "MENTION"
	// TypeAssignment indicates a task/resource was assigned to the user.
	TypeAssignment Type = "ASSIGNMENT"
	// TypeAnnouncement is a broadcast announcement.
	TypeAnnouncement Type = "ANNOUNCEMENT"
)

// IsValid reports whether t is a recognized notification type.
func (t Type) IsValid() bool {
	switch t {
	case TypeExportReady, TypeAlert, TypeApproval, TypeChat, TypeReminder,
		TypeSystem, TypeMention, TypeAssignment, TypeAnnouncement:
		return true
	default:
		return false
	}
}

// String returns the string form.
func (t Type) String() string { return string(t) }

// ParseType validates and converts a raw string into a Type.
func ParseType(s string) (Type, error) {
	v := Type(s)
	if !v.IsValid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidType, s)
	}
	return v, nil
}

// Severity drives the visual styling on the frontend.
type Severity string

// Severity values — must match chk_notif_severity CHECK constraint.
const (
	// SeverityInfo is neutral information.
	SeverityInfo Severity = "INFO"
	// SeveritySuccess is positive outcome.
	SeveritySuccess Severity = "SUCCESS"
	// SeverityWarning means caution/attention required.
	SeverityWarning Severity = "WARNING"
	// SeverityError means failure/error condition.
	SeverityError Severity = "ERROR"
)

// IsValid reports whether s is a recognized severity.
func (s Severity) IsValid() bool {
	switch s {
	case SeverityInfo, SeveritySuccess, SeverityWarning, SeverityError:
		return true
	default:
		return false
	}
}

// String returns the string form.
func (s Severity) String() string { return string(s) }

// ParseSeverity validates and converts a raw string into a Severity.
func ParseSeverity(s string) (Severity, error) {
	v := Severity(s)
	if !v.IsValid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidSeverity, s)
	}
	return v, nil
}

// ActionType controls how the frontend renders the click-action.
type ActionType string

// ActionType values — must match chk_notif_action CHECK constraint.
const (
	// ActionNone means no interactive action.
	ActionNone ActionType = "NONE"
	// ActionNavigate means in-app route navigation.
	ActionNavigate ActionType = "NAVIGATE"
	// ActionDownload means a file ready for download.
	ActionDownload ActionType = "DOWNLOAD"
	// ActionExternalLink opens an external URL.
	ActionExternalLink ActionType = "EXTERNAL_LINK"
	// ActionApproveReject is a binary approve/reject action.
	ActionApproveReject ActionType = "APPROVE_REJECT"
	// ActionAcknowledge is a single-button "I have read" confirmation.
	ActionAcknowledge ActionType = "ACKNOWLEDGE"
	// ActionMultiAction allows arbitrary buttons via payload.
	ActionMultiAction ActionType = "MULTI_ACTION"
	// ActionReply opens a chat reply box.
	ActionReply ActionType = "REPLY"
	// ActionSnooze offers postpone options for a reminder.
	ActionSnooze ActionType = "SNOOZE"
	// ActionCustom is a fallback rendered as info-only.
	ActionCustom ActionType = "CUSTOM"
)

// IsValid reports whether a is a recognized action type.
func (a ActionType) IsValid() bool {
	switch a {
	case ActionNone, ActionNavigate, ActionDownload, ActionExternalLink,
		ActionApproveReject, ActionAcknowledge, ActionMultiAction,
		ActionReply, ActionSnooze, ActionCustom:
		return true
	default:
		return false
	}
}

// String returns the string form.
func (a ActionType) String() string { return string(a) }

// ParseActionType validates and converts a raw string into an ActionType.
func ParseActionType(s string) (ActionType, error) {
	v := ActionType(s)
	if !v.IsValid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidActionType, s)
	}
	return v, nil
}

// Status is the lifecycle state of a notification.
type Status string

// Status values — must match chk_notif_status CHECK constraint.
const (
	// StatusUnread — recipient has not read yet.
	StatusUnread Status = "UNREAD"
	// StatusRead — recipient has marked read.
	StatusRead Status = "READ"
	// StatusArchived — recipient archived (kept until expires_at).
	StatusArchived Status = "ARCHIVED"
)

// IsValid reports whether s is a recognized status.
func (s Status) IsValid() bool {
	switch s {
	case StatusUnread, StatusRead, StatusArchived:
		return true
	default:
		return false
	}
}

// String returns the string form.
func (s Status) String() string { return string(s) }

// ParseStatus validates and converts a raw string into a Status.
func ParseStatus(s string) (Status, error) {
	v := Status(s)
	if !v.IsValid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidStatus, s)
	}
	return v, nil
}
