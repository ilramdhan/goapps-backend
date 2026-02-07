// Package grpc provides gRPC handlers for the IAM service.
package grpc

import (
	"context"
	"errors"

	"github.com/google/uuid"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/audit"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/auth"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/session"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
	"github.com/mutugading/goapps-backend/services/iam/pkg/safeconv"
)

// AuthHandler implements the AuthService gRPC service.
type AuthHandler struct {
	iamv1.UnimplementedAuthServiceServer
	authService      auth.Service
	userRepo         user.Repository
	sessionRepo      session.Repository
	auditRepo        audit.Repository
	validationHelper *ValidationHelper
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(
	authService auth.Service,
	userRepo user.Repository,
	sessionRepo session.Repository,
	auditRepo audit.Repository,
	validationHelper *ValidationHelper,
) *AuthHandler {
	return &AuthHandler{
		authService:      authService,
		userRepo:         userRepo,
		sessionRepo:      sessionRepo,
		auditRepo:        auditRepo,
		validationHelper: validationHelper,
	}
}

// Login authenticates a user and returns tokens.
func (h *AuthHandler) Login(ctx context.Context, req *iamv1.LoginRequest) (*iamv1.LoginResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.LoginResponse{Base: baseResp}, nil
	}

	result, err := h.authService.Login(ctx, auth.LoginInput{
		Username:   req.GetUsername(),
		Password:   req.GetPassword(),
		TOTPCode:   req.GetTotpCode(),
		DeviceInfo: req.GetDeviceInfo(),
		IPAddress:  "", // Will be extracted from metadata
		UserAgent:  "", // Will be extracted from metadata
	})

	if err != nil {
		// Special case: TOTP required returns a partial response with Requires2fa flag.
		if errors.Is(err, shared.ErrTOTPRequired) {
			return &iamv1.LoginResponse{ //nolint:nilerr // error returned in response body
				Base: domainErrorToBaseResponse(err),
				Data: &iamv1.LoginData{
					Requires_2Fa: true,
				},
			}, nil
		}
		return &iamv1.LoginResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.LoginResponse{
		Base: SuccessResponse("Login successful"),
		Data: &iamv1.LoginData{
			AccessToken:  result.AccessToken,
			RefreshToken: result.RefreshToken,
			ExpiresIn:    result.ExpiresIn,
			TokenType:    "Bearer",
			User: &iamv1.AuthUser{
				UserId:           result.User.ID.String(),
				Username:         result.User.Username,
				Email:            result.User.Email,
				FullName:         result.User.FullName,
				TwoFactorEnabled: result.User.TwoFactorEnabled,
				Roles:            result.User.Roles,
				Permissions:      result.User.Permissions,
			},
		},
	}, nil
}

// Logout invalidates the current session.
func (h *AuthHandler) Logout(ctx context.Context, req *iamv1.LogoutRequest) (*iamv1.LogoutResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.LogoutResponse{Base: baseResp}, nil
	}

	refreshToken := req.GetRefreshToken()

	if err := h.authService.Logout(ctx, refreshToken); err != nil {
		return &iamv1.LogoutResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.LogoutResponse{
		Base: SuccessResponse("logout successful"),
	}, nil
}

// RefreshToken refreshes the access token.
func (h *AuthHandler) RefreshToken(ctx context.Context, req *iamv1.RefreshTokenRequest) (*iamv1.RefreshTokenResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.RefreshTokenResponse{Base: baseResp}, nil
	}

	result, err := h.authService.RefreshToken(ctx, req.GetRefreshToken())
	if err != nil {
		return &iamv1.RefreshTokenResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.RefreshTokenResponse{
		Base: SuccessResponse("token refreshed"),
		Data: &iamv1.TokenPair{
			AccessToken:  result.AccessToken,
			RefreshToken: result.RefreshToken,
			ExpiresIn:    result.ExpiresIn,
			TokenType:    "Bearer",
		},
	}, nil
}

// ForgotPassword initiates password reset flow.
func (h *AuthHandler) ForgotPassword(ctx context.Context, req *iamv1.ForgotPasswordRequest) (*iamv1.ForgotPasswordResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ForgotPasswordResponse{Base: baseResp}, nil
	}

	expiresIn, err := h.authService.ForgotPassword(ctx, req.GetEmail())
	if err != nil {
		// Don't reveal whether email exists — always return success.
		return &iamv1.ForgotPasswordResponse{ //nolint:nilerr // error returned in response body
			Base:      SuccessResponse("If the email exists, an OTP has been sent"),
			ExpiresIn: 300,
		}, nil
	}

	return &iamv1.ForgotPasswordResponse{
		Base:      SuccessResponse("OTP sent to your email"),
		ExpiresIn: safeconv.IntToInt32(expiresIn),
	}, nil
}

// VerifyResetOTP verifies the password reset OTP.
func (h *AuthHandler) VerifyResetOTP(ctx context.Context, req *iamv1.VerifyResetOTPRequest) (*iamv1.VerifyResetOTPResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.VerifyResetOTPResponse{Base: baseResp}, nil
	}

	resetToken, err := h.authService.VerifyResetOTP(ctx, req.GetEmail(), req.GetOtpCode())
	if err != nil {
		return &iamv1.VerifyResetOTPResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.VerifyResetOTPResponse{
		Base:       SuccessResponse("OTP verified"),
		ResetToken: resetToken,
	}, nil
}

// ResetPassword resets the password using a reset token.
func (h *AuthHandler) ResetPassword(ctx context.Context, req *iamv1.ResetPasswordRequest) (*iamv1.ResetPasswordResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.ResetPasswordResponse{Base: baseResp}, nil
	}

	if err := h.authService.ResetPassword(ctx, req.GetResetToken(), req.GetNewPassword()); err != nil {
		return &iamv1.ResetPasswordResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.ResetPasswordResponse{
		Base: SuccessResponse("Password reset successful"),
	}, nil
}

// UpdatePassword changes the current user's password.
func (h *AuthHandler) UpdatePassword(ctx context.Context, req *iamv1.UpdatePasswordRequest) (*iamv1.UpdatePasswordResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.UpdatePasswordResponse{Base: baseResp}, nil
	}

	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return &iamv1.UpdatePasswordResponse{Base: UnauthorizedResponse("not authenticated")}, nil //nolint:nilerr // error returned in response body
	}

	if err := h.authService.UpdatePassword(ctx, userID, req.GetCurrentPassword(), req.GetNewPassword()); err != nil {
		return &iamv1.UpdatePasswordResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.UpdatePasswordResponse{
		Base: SuccessResponse("Password updated successfully"),
	}, nil
}

// Enable2FA enables two-factor authentication.
func (h *AuthHandler) Enable2FA(ctx context.Context, _ *iamv1.Enable2FARequest) (*iamv1.Enable2FAResponse, error) {
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return &iamv1.Enable2FAResponse{Base: UnauthorizedResponse("not authenticated")}, nil //nolint:nilerr // error returned in response body
	}

	result, err := h.authService.Enable2FA(ctx, userID)
	if err != nil {
		return &iamv1.Enable2FAResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.Enable2FAResponse{
		Base: SuccessResponse("2FA setup initiated"),
		Data: &iamv1.TwoFactorSetup{
			Secret:        result.Secret,
			QrCodeUrl:     result.QRCodeURL,
			RecoveryCodes: result.RecoveryCodes,
		},
	}, nil
}

// Verify2FA verifies and activates 2FA.
func (h *AuthHandler) Verify2FA(ctx context.Context, req *iamv1.Verify2FARequest) (*iamv1.Verify2FAResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.Verify2FAResponse{Base: baseResp}, nil
	}

	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return &iamv1.Verify2FAResponse{Base: UnauthorizedResponse("not authenticated")}, nil //nolint:nilerr // error returned in response body
	}

	if err := h.authService.Verify2FA(ctx, userID, req.GetTotpCode()); err != nil {
		return &iamv1.Verify2FAResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.Verify2FAResponse{
		Base: SuccessResponse("2FA enabled successfully"),
	}, nil
}

// Disable2FA disables two-factor authentication.
func (h *AuthHandler) Disable2FA(ctx context.Context, req *iamv1.Disable2FARequest) (*iamv1.Disable2FAResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &iamv1.Disable2FAResponse{Base: baseResp}, nil
	}

	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return &iamv1.Disable2FAResponse{Base: UnauthorizedResponse("not authenticated")}, nil //nolint:nilerr // error returned in response body
	}

	if err := h.authService.Disable2FA(ctx, userID, req.GetPassword(), req.GetVerificationCode()); err != nil {
		return &iamv1.Disable2FAResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	return &iamv1.Disable2FAResponse{
		Base: SuccessResponse("2FA disabled successfully"),
	}, nil
}

// GetCurrentUser returns the currently authenticated user.
func (h *AuthHandler) GetCurrentUser(ctx context.Context, _ *iamv1.GetCurrentUserRequest) (*iamv1.GetCurrentUserResponse, error) {
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return &iamv1.GetCurrentUserResponse{Base: UnauthorizedResponse("not authenticated")}, nil //nolint:nilerr // error returned in response body
	}

	u, err := h.userRepo.GetByID(ctx, userID)
	if err != nil {
		return &iamv1.GetCurrentUserResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	// Get user roles and permissions.
	roles, permissions, err := h.userRepo.GetRolesAndPermissions(ctx, userID)
	if err != nil {
		return &iamv1.GetCurrentUserResponse{Base: domainErrorToBaseResponse(err)}, nil //nolint:nilerr // error returned in response body
	}

	roleNames := make([]string, len(roles))
	for i, r := range roles {
		roleNames[i] = r.Code()
	}

	permissionNames := make([]string, len(permissions))
	for i, p := range permissions {
		permissionNames[i] = p.Code()
	}

	return &iamv1.GetCurrentUserResponse{
		Base: SuccessResponse("User retrieved successfully"),
		Data: &iamv1.AuthUser{
			UserId:           u.ID().String(),
			Username:         u.Username(),
			Email:            u.Email(),
			TwoFactorEnabled: u.TwoFactorEnabled(),
			Roles:            roleNames,
			Permissions:      permissionNames,
		},
	}, nil
}

// getUserIDFromContext extracts the user ID from the context.
func getUserIDFromContext(ctx context.Context) (uuid.UUID, error) {
	// This would typically use grpc metadata or a custom context key
	// set by an auth interceptor
	userIDVal := ctx.Value("user_id")
	if userIDVal == nil {
		return uuid.Nil, shared.ErrUnauthorized
	}

	switch v := userIDVal.(type) {
	case string:
		return uuid.Parse(v)
	case uuid.UUID:
		return v, nil
	default:
		return uuid.Nil, shared.ErrUnauthorized
	}
}
