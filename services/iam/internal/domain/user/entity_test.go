// Package user_test provides domain layer tests for User entity.
package user_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// =============================================================================
// Helper functions
// =============================================================================

// validUser creates a valid user for testing convenience.
func validUser(t *testing.T) *user.User {
	t.Helper()
	u, err := user.NewUser("testuser", "test@example.com", "hashedpassword123", "admin")
	require.NoError(t, err)
	return u
}

// deletedUser returns a user that has been soft-deleted.
func deletedUser(t *testing.T) *user.User {
	t.Helper()
	deletedAt := time.Now().Add(-1 * time.Hour)
	deletedBy := "admin"
	return user.ReconstructUser(
		uuid.New(),
		"deleteduser", "deleted@example.com", "hashedpassword123",
		false, false, 0, nil,
		false, "",
		nil, "",
		nil,
		shared.AuditInfo{
			CreatedAt: time.Now().Add(-24 * time.Hour),
			CreatedBy: "admin",
			DeletedAt: &deletedAt,
			DeletedBy: &deletedBy,
		},
	)
}

// lockedUser returns a user that is currently locked.
func lockedUser(t *testing.T, lockedUntil *time.Time) *user.User {
	t.Helper()
	return user.ReconstructUser(
		uuid.New(),
		"lockeduser", "locked@example.com", "hashedpassword123",
		true, true, 5, lockedUntil,
		false, "",
		nil, "",
		nil,
		shared.AuditInfo{
			CreatedAt: time.Now().Add(-24 * time.Hour),
			CreatedBy: "admin",
		},
	)
}

// =============================================================================
// TestNewUser
// =============================================================================

func TestNewUser(t *testing.T) {
	tests := []struct {
		name         string
		username     string
		email        string
		passwordHash string
		createdBy    string
		wantErr      bool
		errType      error
	}{
		{
			name:         "valid creation",
			username:     "john_doe",
			email:        "john@example.com",
			passwordHash: "hashedpassword123",
			createdBy:    "admin",
			wantErr:      false,
		},
		{
			name:         "valid - minimum length username",
			username:     "abc",
			email:        "abc@example.com",
			passwordHash: "hashedpassword123",
			createdBy:    "admin",
			wantErr:      false,
		},
		{
			name:         "invalid - empty username",
			username:     "",
			email:        "john@example.com",
			passwordHash: "hashedpassword123",
			createdBy:    "admin",
			wantErr:      true,
			errType:      user.ErrInvalidUsername,
		},
		{
			name:         "invalid - username too short",
			username:     "ab",
			email:        "john@example.com",
			passwordHash: "hashedpassword123",
			createdBy:    "admin",
			wantErr:      true,
			errType:      user.ErrInvalidUsername,
		},
		{
			name:         "invalid - username with special chars",
			username:     "john@doe",
			email:        "john@example.com",
			passwordHash: "hashedpassword123",
			createdBy:    "admin",
			wantErr:      true,
			errType:      user.ErrInvalidUsername,
		},
		{
			name:         "invalid - username starts with number",
			username:     "1john",
			email:        "john@example.com",
			passwordHash: "hashedpassword123",
			createdBy:    "admin",
			wantErr:      true,
			errType:      user.ErrInvalidUsername,
		},
		{
			name:         "invalid - username with spaces",
			username:     "john doe",
			email:        "john@example.com",
			passwordHash: "hashedpassword123",
			createdBy:    "admin",
			wantErr:      true,
			errType:      user.ErrInvalidUsername,
		},
		{
			name:         "invalid - empty email",
			username:     "john_doe",
			email:        "",
			passwordHash: "hashedpassword123",
			createdBy:    "admin",
			wantErr:      true,
			errType:      user.ErrInvalidEmail,
		},
		{
			name:         "invalid - malformed email",
			username:     "john_doe",
			email:        "not-an-email",
			passwordHash: "hashedpassword123",
			createdBy:    "admin",
			wantErr:      true,
			errType:      user.ErrInvalidEmail,
		},
		{
			name:         "invalid - empty password hash",
			username:     "john_doe",
			email:        "john@example.com",
			passwordHash: "",
			createdBy:    "admin",
			wantErr:      true,
			errType:      user.ErrEmptyPassword,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := user.NewUser(tt.username, tt.email, tt.passwordHash, tt.createdBy)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				assert.Nil(t, u)
			} else {
				require.NoError(t, err)
				require.NotNil(t, u)
				assert.NotEqual(t, uuid.Nil, u.ID())
				assert.Equal(t, tt.username, u.Username())
				assert.Equal(t, tt.email, u.Email())
				assert.Equal(t, tt.passwordHash, u.PasswordHash())
				assert.True(t, u.IsActive())
				assert.False(t, u.IsLocked())
				assert.Equal(t, 0, u.FailedLoginAttempts())
				assert.False(t, u.TwoFactorEnabled())
				assert.False(t, u.IsDeleted())
			}
		})
	}
}

// =============================================================================
// TestUser_CanLogin
// =============================================================================

func TestUser_CanLogin(t *testing.T) {
	t.Run("active user can login", func(t *testing.T) {
		u := validUser(t)

		err := u.CanLogin()

		assert.NoError(t, err)
	})

	t.Run("inactive user cannot login", func(t *testing.T) {
		u := user.ReconstructUser(
			uuid.New(),
			"inactiveuser", "inactive@example.com", "hashedpassword123",
			false, false, 0, nil,
			false, "",
			nil, "",
			nil,
			shared.AuditInfo{
				CreatedAt: time.Now(),
				CreatedBy: "admin",
			},
		)

		err := u.CanLogin()

		assert.Error(t, err)
		assert.ErrorIs(t, err, user.ErrInactive)
	})

	t.Run("locked user cannot login", func(t *testing.T) {
		futureTime := time.Now().Add(1 * time.Hour)
		u := lockedUser(t, &futureTime)

		err := u.CanLogin()

		assert.Error(t, err)
		assert.ErrorIs(t, err, user.ErrAccountLocked)
	})

	t.Run("locked user with expired lockout can login", func(t *testing.T) {
		pastTime := time.Now().Add(-1 * time.Hour)
		u := lockedUser(t, &pastTime)

		err := u.CanLogin()

		assert.NoError(t, err)
		// After expired lockout, user should be unlocked
		assert.False(t, u.IsLocked())
		assert.Equal(t, 0, u.FailedLoginAttempts())
		assert.Nil(t, u.LockedUntil())
	})

	t.Run("deleted user cannot login", func(t *testing.T) {
		u := deletedUser(t)

		err := u.CanLogin()

		assert.Error(t, err)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})
}

// =============================================================================
// TestUser_RecordLoginSuccess
// =============================================================================

func TestUser_RecordLoginSuccess(t *testing.T) {
	t.Run("resets failed attempts and updates login info", func(t *testing.T) {
		u := validUser(t)
		// Simulate some failed attempts first
		u.RecordLoginFailure(5, 30*time.Minute)
		u.RecordLoginFailure(5, 30*time.Minute)
		assert.Equal(t, 2, u.FailedLoginAttempts())

		u.RecordLoginSuccess("192.168.1.100")

		assert.Equal(t, 0, u.FailedLoginAttempts())
		assert.False(t, u.IsLocked())
		assert.Nil(t, u.LockedUntil())
		assert.NotNil(t, u.LastLoginAt())
		assert.Equal(t, "192.168.1.100", u.LastLoginIP())
	})

	t.Run("updates lastLoginAt and lastLoginIP", func(t *testing.T) {
		u := validUser(t)

		u.RecordLoginSuccess("10.0.0.1")

		assert.NotNil(t, u.LastLoginAt())
		assert.Equal(t, "10.0.0.1", u.LastLoginIP())
		assert.WithinDuration(t, time.Now(), *u.LastLoginAt(), 2*time.Second)
	})
}

// =============================================================================
// TestUser_RecordLoginFailure
// =============================================================================

func TestUser_RecordLoginFailure(t *testing.T) {
	t.Run("increments failed attempts", func(t *testing.T) {
		u := validUser(t)

		u.RecordLoginFailure(5, 30*time.Minute)

		assert.Equal(t, 1, u.FailedLoginAttempts())
		assert.False(t, u.IsLocked())
		assert.Nil(t, u.LockedUntil())
	})

	t.Run("locks account after max attempts", func(t *testing.T) {
		u := validUser(t)
		maxAttempts := 3
		lockoutDuration := 30 * time.Minute

		for i := 0; i < maxAttempts; i++ {
			u.RecordLoginFailure(maxAttempts, lockoutDuration)
		}

		assert.Equal(t, maxAttempts, u.FailedLoginAttempts())
		assert.True(t, u.IsLocked())
		assert.NotNil(t, u.LockedUntil())
		assert.WithinDuration(t, time.Now().Add(lockoutDuration), *u.LockedUntil(), 2*time.Second)
	})

	t.Run("does not lock before reaching max attempts", func(t *testing.T) {
		u := validUser(t)
		maxAttempts := 5

		for i := 0; i < maxAttempts-1; i++ {
			u.RecordLoginFailure(maxAttempts, 30*time.Minute)
		}

		assert.Equal(t, maxAttempts-1, u.FailedLoginAttempts())
		assert.False(t, u.IsLocked())
	})
}

// =============================================================================
// TestUser_UpdatePassword
// =============================================================================

func TestUser_UpdatePassword(t *testing.T) {
	t.Run("valid update", func(t *testing.T) {
		u := validUser(t)

		err := u.UpdatePassword("newhashedpassword456", "admin")

		require.NoError(t, err)
		assert.Equal(t, "newhashedpassword456", u.PasswordHash())
		assert.NotNil(t, u.PasswordChangedAt())
		assert.WithinDuration(t, time.Now(), *u.PasswordChangedAt(), 2*time.Second)
	})

	t.Run("empty password hash", func(t *testing.T) {
		u := validUser(t)

		err := u.UpdatePassword("", "admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, user.ErrEmptyPassword)
	})

	t.Run("deleted user", func(t *testing.T) {
		u := deletedUser(t)

		err := u.UpdatePassword("newhashedpassword456", "admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, shared.ErrAlreadyDeleted)
	})
}

// =============================================================================
// TestUser_Enable2FA
// =============================================================================

func TestUser_Enable2FA(t *testing.T) {
	t.Run("enable successfully", func(t *testing.T) {
		u := validUser(t)

		err := u.Enable2FA("JBSWY3DPEHPK3PXP", "admin")

		require.NoError(t, err)
		assert.True(t, u.TwoFactorEnabled())
		assert.Equal(t, "JBSWY3DPEHPK3PXP", u.TwoFactorSecret())
	})

	t.Run("already enabled error", func(t *testing.T) {
		u := validUser(t)
		err := u.Enable2FA("JBSWY3DPEHPK3PXP", "admin")
		require.NoError(t, err)

		err = u.Enable2FA("ANOTHERSECRET", "admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, user.ErrTwoFAAlreadyEnabled)
	})

	t.Run("deleted user", func(t *testing.T) {
		u := deletedUser(t)

		err := u.Enable2FA("JBSWY3DPEHPK3PXP", "admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, shared.ErrAlreadyDeleted)
	})
}

// =============================================================================
// TestUser_Disable2FA
// =============================================================================

func TestUser_Disable2FA(t *testing.T) {
	t.Run("disable successfully", func(t *testing.T) {
		u := validUser(t)
		err := u.Enable2FA("JBSWY3DPEHPK3PXP", "admin")
		require.NoError(t, err)

		err = u.Disable2FA("admin")

		require.NoError(t, err)
		assert.False(t, u.TwoFactorEnabled())
		assert.Empty(t, u.TwoFactorSecret())
	})

	t.Run("not enabled error", func(t *testing.T) {
		u := validUser(t)

		err := u.Disable2FA("admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, user.ErrTwoFANotEnabled)
	})

	t.Run("deleted user", func(t *testing.T) {
		u := deletedUser(t)

		err := u.Disable2FA("admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, shared.ErrAlreadyDeleted)
	})
}

// =============================================================================
// TestUser_Update
// =============================================================================

func TestUser_Update(t *testing.T) {
	t.Run("update email", func(t *testing.T) {
		u := validUser(t)
		newEmail := "newemail@example.com"

		err := u.Update(&newEmail, nil, "editor")

		require.NoError(t, err)
		assert.Equal(t, "newemail@example.com", u.Email())
	})

	t.Run("update isActive", func(t *testing.T) {
		u := validUser(t)
		inactive := false

		err := u.Update(nil, &inactive, "editor")

		require.NoError(t, err)
		assert.False(t, u.IsActive())
	})

	t.Run("update both email and isActive", func(t *testing.T) {
		u := validUser(t)
		newEmail := "updated@example.com"
		active := true

		err := u.Update(&newEmail, &active, "editor")

		require.NoError(t, err)
		assert.Equal(t, "updated@example.com", u.Email())
		assert.True(t, u.IsActive())
	})

	t.Run("invalid email", func(t *testing.T) {
		u := validUser(t)
		badEmail := "not-valid"

		err := u.Update(&badEmail, nil, "editor")

		assert.Error(t, err)
		assert.ErrorIs(t, err, user.ErrInvalidEmail)
	})

	t.Run("deleted user", func(t *testing.T) {
		u := deletedUser(t)
		newEmail := "newemail@example.com"

		err := u.Update(&newEmail, nil, "editor")

		assert.Error(t, err)
		assert.ErrorIs(t, err, shared.ErrAlreadyDeleted)
	})
}

// =============================================================================
// TestUser_SoftDelete
// =============================================================================

func TestUser_SoftDelete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		u := validUser(t)

		err := u.SoftDelete("admin")

		require.NoError(t, err)
		assert.True(t, u.IsDeleted())
		assert.False(t, u.IsActive())
	})

	t.Run("already deleted error", func(t *testing.T) {
		u := deletedUser(t)

		err := u.SoftDelete("admin")

		assert.Error(t, err)
		assert.ErrorIs(t, err, shared.ErrAlreadyDeleted)
	})
}

// =============================================================================
// TestReconstructUser
// =============================================================================

func TestReconstructUser(t *testing.T) {
	id := uuid.New()
	lastLogin := time.Now().Add(-1 * time.Hour)
	passwordChanged := time.Now().Add(-12 * time.Hour)
	lockedUntil := time.Now().Add(30 * time.Minute)
	updatedAt := time.Now().Add(-30 * time.Minute)
	updatedBy := "updater"

	audit := shared.AuditInfo{
		CreatedAt: time.Now().Add(-48 * time.Hour),
		CreatedBy: "creator",
		UpdatedAt: &updatedAt,
		UpdatedBy: &updatedBy,
	}

	u := user.ReconstructUser(
		id,
		"reconstructed", "recon@example.com", "hashedpassword123",
		true, true, 3, &lockedUntil,
		true, "TOTP_SECRET_123",
		&lastLogin, "10.0.0.50",
		&passwordChanged,
		audit,
	)

	require.NotNil(t, u)
	assert.Equal(t, id, u.ID())
	assert.Equal(t, "reconstructed", u.Username())
	assert.Equal(t, "recon@example.com", u.Email())
	assert.Equal(t, "hashedpassword123", u.PasswordHash())
	assert.True(t, u.IsActive())
	assert.True(t, u.IsLocked())
	assert.Equal(t, 3, u.FailedLoginAttempts())
	assert.NotNil(t, u.LockedUntil())
	assert.Equal(t, lockedUntil, *u.LockedUntil())
	assert.True(t, u.TwoFactorEnabled())
	assert.Equal(t, "TOTP_SECRET_123", u.TwoFactorSecret())
	assert.NotNil(t, u.LastLoginAt())
	assert.Equal(t, lastLogin, *u.LastLoginAt())
	assert.Equal(t, "10.0.0.50", u.LastLoginIP())
	assert.NotNil(t, u.PasswordChangedAt())
	assert.Equal(t, passwordChanged, *u.PasswordChangedAt())
	assert.Equal(t, audit.CreatedAt, u.Audit().CreatedAt)
	assert.Equal(t, "creator", u.Audit().CreatedBy)
	assert.NotNil(t, u.Audit().UpdatedAt)
	assert.Equal(t, "updater", *u.Audit().UpdatedBy)
	assert.False(t, u.IsDeleted())
}
