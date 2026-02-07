// Package user provides domain logic for User management.
package user

import (
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// Domain-specific errors for user package.
var (
	ErrInvalidUsername     = errors.New("username must be 3-50 characters, alphanumeric with underscores, starting with a letter")
	ErrInvalidEmail        = errors.New("invalid email format")
	ErrEmptyPassword       = errors.New("password cannot be empty")
	ErrPasswordTooShort    = errors.New("password must be at least 8 characters")
	ErrPasswordNoUppercase = errors.New("password must contain at least one uppercase letter")
	ErrPasswordNoLowercase = errors.New("password must contain at least one lowercase letter")
	ErrPasswordNoNumber    = errors.New("password must contain at least one number")
	ErrAccountLocked       = errors.New("account is locked due to too many failed login attempts")
	ErrInactive            = errors.New("user account is inactive")
	ErrTwoFARequired       = errors.New("two-factor authentication is required")
	ErrTwoFAAlreadyEnabled = errors.New("two-factor authentication is already enabled")
	ErrTwoFANotEnabled     = errors.New("two-factor authentication is not enabled")
	ErrInvalidTOTPCode     = errors.New("invalid TOTP code")
)

// Regex patterns for validation.
var (
	usernameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{2,49}$`)
	emailRegex    = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
)

// User is the aggregate root for User domain.
type User struct {
	id                  uuid.UUID
	username            string
	email               string
	passwordHash        string
	isActive            bool
	isLocked            bool
	failedLoginAttempts int
	lockedUntil         *time.Time
	twoFactorEnabled    bool
	twoFactorSecret     string
	lastLoginAt         *time.Time
	lastLoginIP         string
	passwordChangedAt   *time.Time
	audit               shared.AuditInfo
}

// NewUser creates a new User entity with validation.
func NewUser(username, email, passwordHash, createdBy string) (*User, error) {
	if !usernameRegex.MatchString(username) {
		return nil, ErrInvalidUsername
	}
	if !emailRegex.MatchString(email) {
		return nil, ErrInvalidEmail
	}
	if passwordHash == "" {
		return nil, ErrEmptyPassword
	}

	return &User{
		id:                  uuid.New(),
		username:            username,
		email:               email,
		passwordHash:        passwordHash,
		isActive:            true,
		isLocked:            false,
		failedLoginAttempts: 0,
		twoFactorEnabled:    false,
		audit:               shared.NewAuditInfo(createdBy),
	}, nil
}

// ReconstructUser reconstructs a User entity from persistence data.
func ReconstructUser(
	id uuid.UUID,
	username, email, passwordHash string,
	isActive, isLocked bool,
	failedLoginAttempts int,
	lockedUntil *time.Time,
	twoFactorEnabled bool,
	twoFactorSecret string,
	lastLoginAt *time.Time,
	lastLoginIP string,
	passwordChangedAt *time.Time,
	audit shared.AuditInfo,
) *User {
	return &User{
		id:                  id,
		username:            username,
		email:               email,
		passwordHash:        passwordHash,
		isActive:            isActive,
		isLocked:            isLocked,
		failedLoginAttempts: failedLoginAttempts,
		lockedUntil:         lockedUntil,
		twoFactorEnabled:    twoFactorEnabled,
		twoFactorSecret:     twoFactorSecret,
		lastLoginAt:         lastLoginAt,
		lastLoginIP:         lastLoginIP,
		passwordChangedAt:   passwordChangedAt,
		audit:               audit,
	}
}

// ID returns the user identifier.
func (u *User) ID() uuid.UUID { return u.id }

// Username returns the username.
func (u *User) Username() string { return u.username }

// Email returns the email address.
func (u *User) Email() string { return u.email }

// PasswordHash returns the password hash.
func (u *User) PasswordHash() string { return u.passwordHash }

// IsActive returns whether the user is active.
func (u *User) IsActive() bool { return u.isActive }

// IsLocked returns whether the user account is locked.
func (u *User) IsLocked() bool { return u.isLocked }

// FailedLoginAttempts returns the number of failed login attempts.
func (u *User) FailedLoginAttempts() int { return u.failedLoginAttempts }

// LockedUntil returns the lock expiry time.
func (u *User) LockedUntil() *time.Time { return u.lockedUntil }

// TwoFactorEnabled returns whether 2FA is enabled.
func (u *User) TwoFactorEnabled() bool { return u.twoFactorEnabled }

// TwoFactorSecret returns the 2FA secret.
func (u *User) TwoFactorSecret() string { return u.twoFactorSecret }

// LastLoginAt returns the last login time.
func (u *User) LastLoginAt() *time.Time { return u.lastLoginAt }

// LastLoginIP returns the last login IP address.
func (u *User) LastLoginIP() string { return u.lastLoginIP }

// PasswordChangedAt returns when the password was last changed.
func (u *User) PasswordChangedAt() *time.Time { return u.passwordChangedAt }

// Audit returns the audit information.
func (u *User) Audit() shared.AuditInfo { return u.audit }

// IsDeleted returns whether the user has been soft-deleted.
func (u *User) IsDeleted() bool { return u.audit.IsDeleted() }

// =============================================================================
// Domain Behavior Methods
// =============================================================================

// CanLogin checks if the user can log in.
func (u *User) CanLogin() error {
	if u.IsDeleted() {
		return shared.ErrNotFound
	}
	if !u.isActive {
		return ErrInactive
	}
	if u.isLocked {
		if u.lockedUntil != nil && time.Now().After(*u.lockedUntil) {
			// Lockout period has expired, unlock the account
			u.isLocked = false
			u.failedLoginAttempts = 0
			u.lockedUntil = nil
		} else {
			return ErrAccountLocked
		}
	}
	return nil
}

// RecordLoginSuccess records a successful login.
func (u *User) RecordLoginSuccess(ip string) {
	now := time.Now()
	u.lastLoginAt = &now
	u.lastLoginIP = ip
	u.failedLoginAttempts = 0
	u.isLocked = false
	u.lockedUntil = nil
}

// RecordLoginFailure records a failed login attempt.
func (u *User) RecordLoginFailure(maxAttempts int, lockoutDuration time.Duration) {
	u.failedLoginAttempts++
	if u.failedLoginAttempts >= maxAttempts {
		u.isLocked = true
		lockUntil := time.Now().Add(lockoutDuration)
		u.lockedUntil = &lockUntil
	}
}

// UpdatePassword updates the user's password.
func (u *User) UpdatePassword(newPasswordHash string, updatedBy string) error {
	if u.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if newPasswordHash == "" {
		return ErrEmptyPassword
	}

	u.passwordHash = newPasswordHash
	now := time.Now()
	u.passwordChangedAt = &now
	u.audit.Update(updatedBy)
	return nil
}

// Enable2FA enables two-factor authentication.
func (u *User) Enable2FA(secret string, updatedBy string) error {
	if u.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if u.twoFactorEnabled {
		return ErrTwoFAAlreadyEnabled
	}

	u.twoFactorEnabled = true
	u.twoFactorSecret = secret
	u.audit.Update(updatedBy)
	return nil
}

// Disable2FA disables two-factor authentication.
func (u *User) Disable2FA(updatedBy string) error {
	if u.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	if !u.twoFactorEnabled {
		return ErrTwoFANotEnabled
	}

	u.twoFactorEnabled = false
	u.twoFactorSecret = ""
	u.audit.Update(updatedBy)
	return nil
}

// Update updates user details.
func (u *User) Update(email *string, isActive *bool, updatedBy string) error {
	if u.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}

	if email != nil {
		if !emailRegex.MatchString(*email) {
			return ErrInvalidEmail
		}
		u.email = *email
	}

	if isActive != nil {
		u.isActive = *isActive
	}

	u.audit.Update(updatedBy)
	return nil
}

// SoftDelete marks the user as deleted.
func (u *User) SoftDelete(deletedBy string) error {
	if u.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	u.isActive = false
	u.audit.SoftDelete(deletedBy)
	return nil
}

// Unlock unlocks the user account.
func (u *User) Unlock(updatedBy string) error {
	if u.IsDeleted() {
		return shared.ErrAlreadyDeleted
	}
	u.isLocked = false
	u.failedLoginAttempts = 0
	u.lockedUntil = nil
	u.audit.Update(updatedBy)
	return nil
}
