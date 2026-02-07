// Package shared provides common domain types used across all IAM domain packages.
package shared

import (
	"errors"
	"fmt"
)

// Common domain errors.
var (
	// Entity errors
	ErrEmptyID        = errors.New("id cannot be empty")
	ErrEmptyCode      = errors.New("code cannot be empty")
	ErrEmptyName      = errors.New("name cannot be empty")
	ErrCodeTooLong    = errors.New("code exceeds maximum length")
	ErrNameTooLong    = errors.New("name exceeds maximum length")
	ErrInvalidCode    = errors.New("code format is invalid")
	ErrAlreadyExists  = errors.New("entity already exists")
	ErrAlreadyDeleted = errors.New("entity has already been deleted")
	ErrNotActive      = errors.New("entity is not active")

	// Authorization errors
	ErrUnauthorized     = errors.New("unauthorized access")
	ErrPermissionDenied = errors.New("permission denied")

	// Not found errors
	ErrNotFound        = errors.New("entity not found")
	ErrSessionNotFound = errors.New("session not found")

	// Authentication errors
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountLocked      = errors.New("account is locked")
	ErrInvalidToken       = errors.New("token is invalid")
	ErrTokenRevoked       = errors.New("token has been revoked")
	ErrTokenExpired       = errors.New("token has expired")

	// 2FA errors
	ErrTwoFARequired       = errors.New("2FA code required")
	ErrTwoFAAlreadyEnabled = errors.New("2FA is already enabled")
	ErrTwoFANotEnabled     = errors.New("2FA is not enabled")
	ErrInvalid2FACode      = errors.New("invalid 2FA code")

	// OTP errors
	ErrInvalidOTP = errors.New("invalid OTP code")

	// Legacy aliases for compatibility
	ErrTOTPRequired = ErrTwoFARequired
	ErrTOTPInvalid  = ErrInvalid2FACode
)

// DomainError wraps domain-specific errors with additional context.
type DomainError struct {
	Code    string
	Message string
	Err     error
}

func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *DomainError) Unwrap() error {
	return e.Err
}

// NewDomainError creates a new DomainError.
func NewDomainError(code, message string, err error) *DomainError {
	return &DomainError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// ValidationError represents a validation error with field details.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewValidationError creates a new ValidationError.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}
