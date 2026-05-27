package notification

import "errors"

// Sentinel errors for the notification domain.
var (
	// ErrNotFound is returned when a notification cannot be located.
	ErrNotFound = errors.New("notification not found")
	// ErrForbidden is returned when a non-owner tries to access a notification.
	ErrForbidden = errors.New("notification access forbidden")
	// ErrInvalidType is returned for an unrecognized Type value.
	ErrInvalidType = errors.New("invalid notification type")
	// ErrInvalidSeverity is returned for an unrecognized Severity value.
	ErrInvalidSeverity = errors.New("invalid notification severity")
	// ErrInvalidActionType is returned for an unrecognized ActionType value.
	ErrInvalidActionType = errors.New("invalid notification action type")
	// ErrInvalidStatus is returned for an unrecognized Status value.
	ErrInvalidStatus = errors.New("invalid notification status")
	// ErrEmptyTitle is returned when the title is empty.
	ErrEmptyTitle = errors.New("notification title must not be empty")
	// ErrEmptyRecipient is returned when recipient_user_id is the zero UUID.
	ErrEmptyRecipient = errors.New("recipient user id must be provided")
	// ErrAlreadyArchived is returned when archiving an already-archived notification.
	ErrAlreadyArchived = errors.New("notification already archived")
)
