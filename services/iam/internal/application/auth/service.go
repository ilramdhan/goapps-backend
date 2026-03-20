// Package auth provides authentication application services.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/audit"
	domainAuth "github.com/mutugading/goapps-backend/services/iam/internal/domain/auth"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/session"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/jwt"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/password"
	redisinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/redis"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/totp"
)

// EmailService defines the interface for sending emails.
type EmailService interface {
	// SendOTP sends a password reset OTP to the user's email.
	SendOTP(ctx context.Context, email, otp string, expiryMinutes int) error
	// Send2FANotification sends a notification about 2FA status change.
	Send2FANotification(ctx context.Context, email, action string) error
}

// Service implements domainAuth.Service interface.
type Service struct {
	userRepo       user.Repository
	sessionRepo    session.Repository
	auditRepo      audit.Repository
	jwtService     *jwt.Service
	totpService    *totp.Service
	sessionCache   *redisinfra.SessionCache
	otpCache       *redisinfra.OTPCache
	rateLimitCache *redisinfra.RateLimitCache
	emailService   EmailService
	securityCfg    *config.SecurityConfig
}

// NewService creates a new auth service.
func NewService(
	userRepo user.Repository,
	sessionRepo session.Repository,
	auditRepo audit.Repository,
	jwtService *jwt.Service,
	totpService *totp.Service,
	sessionCache *redisinfra.SessionCache,
	otpCache *redisinfra.OTPCache,
	rateLimitCache *redisinfra.RateLimitCache,
	securityCfg *config.SecurityConfig,
) *Service {
	return &Service{
		userRepo:       userRepo,
		sessionRepo:    sessionRepo,
		auditRepo:      auditRepo,
		jwtService:     jwtService,
		totpService:    totpService,
		sessionCache:   sessionCache,
		otpCache:       otpCache,
		rateLimitCache: rateLimitCache,
		securityCfg:    securityCfg,
	}
}

// SetEmailService sets the email service (optional, for dependency injection).
func (s *Service) SetEmailService(emailService EmailService) {
	s.emailService = emailService
}

// Login authenticates a user and returns tokens.
func (s *Service) Login(ctx context.Context, input domainAuth.LoginInput) (*domainAuth.LoginResult, error) {
	if err := s.checkRateLimit(ctx, input.Username); err != nil {
		return nil, err
	}

	u, err := s.authenticateUser(ctx, input)
	if err != nil {
		return nil, err
	}

	roleNames, permNames, err := s.getUserRolePermNames(ctx, u.ID())
	if err != nil {
		return nil, err
	}

	tokenPair, err := s.jwtService.GenerateTokenPair(u.ID(), u.Username(), u.Email(), roleNames, permNames, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	if err := s.recordSuccessfulLogin(ctx, u, input); err != nil {
		return nil, err
	}

	sess := session.NewSession(u.ID(), tokenPair.TokenID, input.DeviceInfo, input.IPAddress, "goapps", tokenPair.RefreshExp)
	if err := s.sessionRepo.Create(ctx, sess); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	s.cacheSession(ctx, sess.ID(), u.ID())

	fullName := s.getFullName(ctx, u)
	s.logAudit(ctx, u.ID(), "LOGIN", "User logged in", input.IPAddress, input.UserAgent)

	return &domainAuth.LoginResult{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    s.jwtService.GetAccessTTLSeconds(),
		User: &domainAuth.UserInfo{
			ID:               u.ID(),
			Username:         u.Username(),
			Email:            u.Email(),
			FullName:         fullName,
			TwoFactorEnabled: u.TwoFactorEnabled(),
			Roles:            roleNames,
			Permissions:      permNames,
		},
	}, nil
}

func (s *Service) checkRateLimit(ctx context.Context, username string) error {
	if s.rateLimitCache != nil {
		attempts, err := s.rateLimitCache.GetLoginAttempts(ctx, username)
		if err == nil && attempts >= int64(s.securityCfg.MaxLoginAttempts) {
			return shared.ErrAccountLocked
		}
	}
	return nil
}

func (s *Service) authenticateUser(ctx context.Context, input domainAuth.LoginInput) (*user.User, error) {
	var u *user.User
	var err error

	// Auto-detect: if the identifier contains '@', treat it as an email address.
	if strings.Contains(input.Username, "@") {
		u, err = s.userRepo.GetByEmail(ctx, input.Username)
	} else {
		u, err = s.userRepo.GetByUsername(ctx, input.Username)
	}

	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			s.recordFailedLogin(ctx, input.Username)
			return nil, shared.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if err := u.CanLogin(); err != nil {
		return nil, err
	}

	if err := s.verifyPassword(u.PasswordHash(), input.Password); err != nil {
		s.recordFailedLogin(ctx, input.Username)
		u.RecordLoginFailure(s.securityCfg.MaxLoginAttempts, s.securityCfg.LockoutDuration)
		if updateErr := s.userRepo.Update(ctx, u); updateErr != nil {
			log.Warn().Err(updateErr).Msg("failed to update user after login failure")
		}
		return nil, shared.ErrInvalidCredentials
	}

	if u.TwoFactorEnabled() {
		if input.TOTPCode == "" {
			return nil, shared.ErrTwoFARequired
		}
		if !s.totpService.Validate(u.TwoFactorSecret(), input.TOTPCode) {
			return nil, shared.ErrInvalid2FACode
		}
	}

	return u, nil
}

func (s *Service) getUserRolePermNames(ctx context.Context, userID uuid.UUID) ([]string, []string, error) {
	roles, permissions, err := s.userRepo.GetRolesAndPermissions(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get roles and permissions: %w", err)
	}

	roleNames := make([]string, len(roles))
	for i, r := range roles {
		roleNames[i] = r.Code()
	}
	permNames := make([]string, len(permissions))
	for i, p := range permissions {
		permNames[i] = p.Code()
	}
	return roleNames, permNames, nil
}

func (s *Service) recordSuccessfulLogin(ctx context.Context, u *user.User, input domainAuth.LoginInput) error {
	u.RecordLoginSuccess(input.IPAddress)
	if err := s.userRepo.Update(ctx, u); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	if s.rateLimitCache != nil {
		if err := s.rateLimitCache.ResetLoginAttempts(ctx, input.Username); err != nil {
			log.Warn().Err(err).Msg("failed to reset login attempts")
		}
	}
	return nil
}

func (s *Service) cacheSession(ctx context.Context, sessID, userID uuid.UUID) {
	if s.sessionCache != nil {
		if err := s.sessionCache.StoreSession(ctx, sessID, userID, s.jwtService.GetRefreshTTLSeconds()); err != nil {
			log.Warn().Err(err).Msg("failed to cache session")
		}
	}
}

func (s *Service) getFullName(ctx context.Context, u *user.User) string {
	detail, err := s.userRepo.GetDetailByUserID(ctx, u.ID())
	if err != nil {
		log.Warn().Err(err).Msg("failed to get user detail for login")
	}
	if detail != nil {
		return detail.FullName()
	}
	return u.Username()
}

// Logout invalidates a user session.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	claims, err := s.jwtService.ValidateRefreshToken(refreshToken)
	if err != nil {
		return shared.ErrInvalidToken
	}

	s.blacklistToken(ctx, claims.ID, claims.ExpiresAt.Unix())

	// Invalidate session in database
	if err := s.sessionRepo.RevokeByTokenID(ctx, claims.ID); err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	userID, parseErr := uuid.Parse(claims.UserID)
	if parseErr != nil {
		log.Warn().Err(parseErr).Msg("failed to parse user ID on logout")
	}
	s.logAudit(ctx, userID, "LOGOUT", "User logged out", "", "")

	return nil
}

// RefreshToken refreshes the access token using a refresh token.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*domainAuth.RefreshResult, error) {
	claims, err := s.jwtService.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, shared.ErrInvalidToken
	}

	if err := s.checkTokenBlacklist(ctx, claims.ID); err != nil {
		return nil, err
	}

	sess, err := s.sessionRepo.GetByTokenID(ctx, claims.ID)
	if err != nil || sess == nil || sess.IsRevoked() {
		return nil, shared.ErrSessionNotFound
	}

	// Check idle timeout: reject refresh if user has been inactive too long
	if s.securityCfg.SessionIdleTimeout > 0 && sess.IsIdle(s.securityCfg.SessionIdleTimeout) {
		sess.Revoke()
		if updateErr := s.sessionRepo.Update(ctx, sess); updateErr != nil {
			log.Warn().Err(updateErr).Msg("failed to revoke idle session")
		}
		s.blacklistToken(ctx, claims.ID, claims.ExpiresAt.Unix())
		return nil, shared.ErrSessionExpired
	}

	userID, parseErr := uuid.Parse(claims.UserID)
	if parseErr != nil {
		return nil, fmt.Errorf("failed to parse user ID: %w", parseErr)
	}
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	roleNames, permNames, err := s.getUserRolePermNames(ctx, u.ID())
	if err != nil {
		log.Warn().Err(err).Str("userID", u.ID().String()).Msg("failed to get user role/permission names during token refresh")
	}

	tokenPair, err := s.jwtService.GenerateTokenPair(u.ID(), u.Username(), u.Email(), roleNames, permNames, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	s.blacklistToken(ctx, claims.ID, claims.ExpiresAt.Unix())

	sess.UpdateTokenID(tokenPair.TokenID, tokenPair.RefreshExp)
	sess.TouchActivity()
	if err := s.sessionRepo.Update(ctx, sess); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	return &domainAuth.RefreshResult{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    s.jwtService.GetAccessTTLSeconds(),
	}, nil
}

func (s *Service) checkTokenBlacklist(ctx context.Context, tokenID string) error {
	if s.sessionCache != nil {
		blacklisted, err := s.sessionCache.IsBlacklisted(ctx, tokenID)
		if err != nil {
			log.Warn().Err(err).Msg("failed to check token blacklist")
		}
		if blacklisted {
			return shared.ErrTokenRevoked
		}
	}
	return nil
}

func (s *Service) blacklistToken(ctx context.Context, tokenID string, expiresAtUnix int64) {
	if s.sessionCache != nil {
		expiresIn := expiresAtUnix - time.Now().Unix()
		if expiresIn > 0 {
			if err := s.sessionCache.BlacklistToken(ctx, tokenID, expiresIn); err != nil {
				log.Warn().Err(err).Msg("failed to blacklist token")
			}
		}
	}
}

// ForgotPassword initiates the password reset flow.
func (s *Service) ForgotPassword(ctx context.Context, email string) (expiresIn int, err error) {
	// OTP cache is required for password reset flow
	if s.otpCache == nil {
		return 0, errors.New("password reset service unavailable")
	}

	u, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		// Return success even if user not found (prevent email enumeration)
		if errors.Is(err, shared.ErrNotFound) {
			return int(s.securityCfg.OTPExpiry.Seconds()), nil
		}
		return 0, fmt.Errorf("failed to get user: %w", err)
	}

	// Generate OTP
	otp := generateOTP(6)
	if err := s.otpCache.StoreOTP(ctx, u.ID(), otp); err != nil {
		return 0, fmt.Errorf("failed to store OTP: %w", err)
	}

	// Send OTP via email (if email service is configured)
	if s.emailService != nil {
		if err := s.emailService.SendOTP(ctx, u.Email(), otp, int(s.securityCfg.OTPExpiry.Minutes())); err != nil {
			log.Warn().Err(err).Str("email", u.Email()).Msg("failed to send OTP email")
			// Don't fail the request — OTP is stored, user can retry
		}
	} else {
		log.Warn().Str("otp", otp).Str("email", u.Email()).Msg("Email service not configured, OTP logged for development")
	}

	s.logAudit(ctx, u.ID(), "FORGOT_PASSWORD", "Password reset requested", "", "")

	return int(s.securityCfg.OTPExpiry.Seconds()), nil
}

// VerifyResetOTP verifies the password reset OTP.
func (s *Service) VerifyResetOTP(ctx context.Context, email, otpCode string) (resetToken string, err error) {
	// OTP cache is required — never skip verification
	if s.otpCache == nil {
		return "", errors.New("password reset service unavailable")
	}

	u, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return "", shared.ErrInvalidCredentials
	}

	// Verify OTP — this MUST always be checked
	valid, verifyErr := s.otpCache.VerifyOTP(ctx, u.ID(), otpCode)
	if verifyErr != nil || !valid {
		return "", shared.ErrInvalidOTP
	}

	// Generate reset token
	resetToken = generateResetToken()
	if err := s.otpCache.StoreResetToken(ctx, resetToken, u.ID(), s.securityCfg.ResetTokenExpiry); err != nil {
		return "", fmt.Errorf("failed to store reset token: %w", err)
	}

	return resetToken, nil
}

// ResetPassword resets the password using a reset token.
func (s *Service) ResetPassword(ctx context.Context, resetToken, newPassword string) error {
	if s.otpCache == nil {
		return errors.New("password reset service unavailable")
	}

	userID, err := s.otpCache.GetResetToken(ctx, resetToken)
	if err != nil || userID == uuid.Nil {
		return shared.ErrInvalidToken
	}

	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Hash new password using Argon2id
	hash, err := password.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := u.UpdatePassword(hash, "system"); err != nil {
		return err
	}

	if err := s.userRepo.Update(ctx, u); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	s.logAudit(ctx, u.ID(), "RESET_PASSWORD", "Password reset completed", "", "")

	return nil
}

// UpdatePassword changes the user's password.
func (s *Service) UpdatePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Verify current password
	if err := s.verifyPassword(u.PasswordHash(), currentPassword); err != nil {
		return shared.ErrInvalidCredentials
	}

	// Hash new password using Argon2id
	hash, err := password.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := u.UpdatePassword(hash, userID.String()); err != nil {
		return err
	}

	if err := s.userRepo.Update(ctx, u); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	s.logAudit(ctx, u.ID(), "CHANGE_PASSWORD", "Password changed", "", "")

	return nil
}

// Enable2FA initiates 2FA setup.
func (s *Service) Enable2FA(ctx context.Context, userID uuid.UUID) (*domainAuth.Enable2FAResult, error) {
	if s.otpCache == nil {
		return nil, errors.New("2FA setup service unavailable")
	}

	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if u.TwoFactorEnabled() {
		return nil, shared.ErrTwoFAAlreadyEnabled
	}

	// Generate secret
	secret, err := s.totpService.GenerateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}

	// Generate QR code URL
	qrURL := s.totpService.GenerateQRURI(secret, u.Email())

	// Generate recovery codes
	recoveryCodes := generateRecoveryCodes(8)

	// Store pending secret in Redis (NOT activated yet — user must verify first)
	if err := s.otpCache.Store2FASetup(ctx, userID, secret, recoveryCodes, 10*time.Minute); err != nil {
		return nil, fmt.Errorf("failed to store 2FA setup: %w", err)
	}

	return &domainAuth.Enable2FAResult{
		Secret:        secret,
		QRCodeURL:     qrURL,
		RecoveryCodes: recoveryCodes,
	}, nil
}

// Verify2FA verifies and activates 2FA.
func (s *Service) Verify2FA(ctx context.Context, userID uuid.UUID, totpCode string) error {
	if s.otpCache == nil {
		return errors.New("2FA setup service unavailable")
	}

	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if u.TwoFactorEnabled() {
		return shared.ErrTwoFAAlreadyEnabled
	}

	// Retrieve pending secret from Redis (stored by Enable2FA)
	secret, recoveryCodes, err := s.otpCache.Get2FASetup(ctx, userID)
	if err != nil || secret == "" {
		return fmt.Errorf("2FA setup expired or not initiated, please call Enable2FA first")
	}

	// Validate TOTP code against the pending secret
	if !s.totpService.Validate(secret, totpCode) {
		return shared.ErrInvalid2FACode
	}

	// Activate 2FA on the user entity
	if err := u.Enable2FA(secret, userID.String()); err != nil {
		return err
	}

	if err := s.userRepo.Update(ctx, u); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	// Store hashed recovery codes in database
	if err := s.userRepo.StoreRecoveryCodes(ctx, userID, hashRecoveryCodes(recoveryCodes)); err != nil {
		log.Warn().Err(err).Msg("failed to store recovery codes")
	}

	// Clean up the pending setup from Redis
	if err := s.otpCache.Delete2FASetup(ctx, userID); err != nil {
		log.Warn().Err(err).Msg("failed to clean up 2FA setup from cache")
	}

	s.logAudit(ctx, u.ID(), "ENABLE_2FA", "Two-factor authentication enabled", "", "")

	return nil
}

// Disable2FA disables 2FA for a user.
func (s *Service) Disable2FA(ctx context.Context, userID uuid.UUID, pwd, verificationCode string) error {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if !u.TwoFactorEnabled() {
		return shared.ErrTwoFANotEnabled
	}

	// Verify password
	if err := s.verifyPassword(u.PasswordHash(), pwd); err != nil {
		return shared.ErrInvalidCredentials
	}

	// Verify TOTP code or recovery code
	if !s.totpService.Validate(u.TwoFactorSecret(), verificationCode) {
		// Try recovery code
		if !s.verifyRecoveryCode(ctx, userID, verificationCode) {
			return shared.ErrInvalid2FACode
		}
	}

	if err := u.Disable2FA(userID.String()); err != nil {
		return err
	}

	if err := s.userRepo.Update(ctx, u); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	// Delete recovery codes
	if err := s.userRepo.DeleteRecoveryCodes(ctx, userID); err != nil {
		log.Warn().Err(err).Msg("failed to delete recovery codes")
	}

	s.logAudit(ctx, u.ID(), "DISABLE_2FA", "Two-factor authentication disabled", "", "")

	return nil
}

// Helper functions

// verifyPassword verifies a password against a stored hash.
// Supports both Argon2id (new) and bcrypt (legacy) formats.
func (s *Service) verifyPassword(storedHash, plainPassword string) error {
	// Check if hash is Argon2id format
	if strings.HasPrefix(storedHash, "$argon2id$") {
		match, err := password.Verify(plainPassword, storedHash)
		if err != nil {
			return fmt.Errorf("password verification failed: %w", err)
		}
		if !match {
			return shared.ErrInvalidCredentials
		}
		return nil
	}

	// Legacy bcrypt support: try bcrypt for old hashes.
	// This allows gradual migration from bcrypt to argon2id.
	if strings.HasPrefix(storedHash, "$2a$") || strings.HasPrefix(storedHash, "$2b$") || strings.HasPrefix(storedHash, "$2y$") {
		match, err := password.VerifyBcryptLegacy(plainPassword, storedHash)
		if err != nil {
			return fmt.Errorf("bcrypt verification failed: %w", err)
		}
		if !match {
			return shared.ErrInvalidCredentials
		}
		return nil
	}

	return shared.ErrInvalidCredentials
}

// verifyRecoveryCode checks if a recovery code is valid and marks it as used.
func (s *Service) verifyRecoveryCode(ctx context.Context, userID uuid.UUID, code string) bool {
	codeHash := hashSingle(code)
	used, err := s.userRepo.UseRecoveryCode(ctx, userID, codeHash)
	if err != nil {
		log.Warn().Err(err).Msg("failed to verify recovery code")
		return false
	}
	return used
}

func (s *Service) recordFailedLogin(ctx context.Context, username string) {
	if s.rateLimitCache != nil {
		if _, err := s.rateLimitCache.IncrementLoginAttempt(ctx, username, s.securityCfg.LockoutDuration); err != nil {
			log.Warn().Err(err).Msg("failed to increment login attempt counter")
		}
	}
}

func (s *Service) logAudit(ctx context.Context, userID uuid.UUID, action, description, ipAddress, userAgent string) {
	if s.auditRepo != nil {
		entry := audit.NewLog(
			audit.EventType(action),
			"mst_user",
			&userID,
			&userID,
			"", // username - will be populated if needed
			description,
			ipAddress,
			userAgent,
			"iam",
		)
		if err := s.auditRepo.Create(ctx, entry); err != nil {
			log.Warn().Err(err).Msg("failed to create audit log")
		}
	}
}

func generateOTP(length int) string {
	const digits = "0123456789"
	b := make([]byte, length)
	_, _ = rand.Read(b) //nolint:errcheck // crypto/rand.Read never returns error on supported platforms
	for i := range b {
		b[i] = digits[int(b[i])%len(digits)]
	}
	return string(b)
}

func generateResetToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) //nolint:errcheck // crypto/rand.Read never returns error on supported platforms
	return hex.EncodeToString(b)
}

func generateRecoveryCodes(count int) []string {
	codes := make([]string, count)
	for i := range count {
		b := make([]byte, 5)
		_, _ = rand.Read(b) //nolint:errcheck // crypto/rand.Read never returns error on supported platforms
		codes[i] = hex.EncodeToString(b)
	}
	return codes
}

// hashRecoveryCodes hashes recovery codes with SHA256 for secure storage.
func hashRecoveryCodes(codes []string) []string {
	hashed := make([]string, len(codes))
	for i, code := range codes {
		hashed[i] = hashSingle(code)
	}
	return hashed
}

func hashSingle(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
