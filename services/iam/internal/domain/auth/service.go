// Package auth provides authentication domain logic.
package auth

import (
	"context"

	"github.com/google/uuid"
)

// Service defines the authentication service interface.
type Service interface {
	// Login authenticates a user and returns tokens.
	Login(ctx context.Context, input LoginInput) (*LoginResult, error)

	// Logout invalidates a user session.
	Logout(ctx context.Context, refreshToken string) error

	// RefreshToken refreshes the access token using a refresh token.
	RefreshToken(ctx context.Context, refreshToken string) (*RefreshResult, error)

	// ForgotPassword initiates the password reset flow.
	ForgotPassword(ctx context.Context, email string) (expiresIn int, err error)

	// VerifyResetOTP verifies the password reset OTP.
	VerifyResetOTP(ctx context.Context, email, otpCode string) (resetToken string, err error)

	// ResetPassword resets the password using a reset token.
	ResetPassword(ctx context.Context, resetToken, newPassword string) error

	// UpdatePassword changes the user's password.
	UpdatePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error

	// Enable2FA initiates 2FA setup.
	Enable2FA(ctx context.Context, userID uuid.UUID) (*Enable2FAResult, error)

	// Verify2FA verifies and activates 2FA.
	Verify2FA(ctx context.Context, userID uuid.UUID, totpCode string) error

	// Disable2FA disables 2FA for a user.
	Disable2FA(ctx context.Context, userID uuid.UUID, password, totpCode string) error
}

// LoginInput contains the login request parameters.
type LoginInput struct {
	Username   string
	Password   string
	TOTPCode   string
	DeviceInfo string
	IPAddress  string
	UserAgent  string
}

// LoginResult contains the login response data.
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	User         *UserInfo
}

// UserInfo contains basic user info for authentication context.
type UserInfo struct {
	ID               uuid.UUID
	Username         string
	Email            string
	FullName         string
	TwoFactorEnabled bool
	Roles            []string
	Permissions      []string
}

// RefreshResult contains the token refresh response data.
type RefreshResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

// Enable2FAResult contains the 2FA setup data.
type Enable2FAResult struct {
	Secret        string
	QRCodeURL     string
	RecoveryCodes []string
}
