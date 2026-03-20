// Package auth_test provides unit tests for the auth application service.
package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appauth "github.com/mutugading/goapps-backend/services/iam/internal/application/auth"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/audit"
	domainAuth "github.com/mutugading/goapps-backend/services/iam/internal/domain/auth"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/session"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/jwt"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/password"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/totp"
)

// =============================================================================
// Mocks
// =============================================================================

// MockUserRepo is a mock implementation of user.Repository.
type MockUserRepo struct{ mock.Mock }

func (m *MockUserRepo) Create(ctx context.Context, u *user.User, d *user.Detail) error {
	return m.Called(ctx, u, d).Error(0)
}
func (m *MockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}
func (m *MockUserRepo) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}
func (m *MockUserRepo) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}
func (m *MockUserRepo) Update(ctx context.Context, u *user.User) error {
	return m.Called(ctx, u).Error(0)
}
func (m *MockUserRepo) Delete(ctx context.Context, id uuid.UUID, by string) error {
	return m.Called(ctx, id, by).Error(0)
}
func (m *MockUserRepo) GetDetailByUserID(ctx context.Context, userID uuid.UUID) (*user.Detail, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.Detail), args.Error(1)
}
func (m *MockUserRepo) UpdateDetail(ctx context.Context, d *user.Detail) error {
	return m.Called(ctx, d).Error(0)
}
func (m *MockUserRepo) List(ctx context.Context, p user.ListParams) ([]*user.User, int64, error) {
	args := m.Called(ctx, p)
	return args.Get(0).([]*user.User), args.Get(1).(int64), args.Error(2)
}
func (m *MockUserRepo) ListWithDetails(ctx context.Context, p user.ListParams) ([]*user.WithDetail, int64, error) {
	args := m.Called(ctx, p)
	return args.Get(0).([]*user.WithDetail), args.Get(1).(int64), args.Error(2)
}
func (m *MockUserRepo) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	args := m.Called(ctx, username)
	return args.Bool(0), args.Error(1)
}
func (m *MockUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	args := m.Called(ctx, email)
	return args.Bool(0), args.Error(1)
}
func (m *MockUserRepo) ExistsByEmployeeCode(ctx context.Context, code string) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}
func (m *MockUserRepo) BatchCreate(ctx context.Context, users []*user.User, details []*user.Detail) (int, error) {
	args := m.Called(ctx, users, details)
	return args.Int(0), args.Error(1)
}
func (m *MockUserRepo) GetRolesAndPermissions(ctx context.Context, id uuid.UUID) ([]user.RoleRef, []user.PermissionRef, error) {
	args := m.Called(ctx, id)
	return args.Get(0).([]user.RoleRef), args.Get(1).([]user.PermissionRef), args.Error(2)
}
func (m *MockUserRepo) StoreRecoveryCodes(ctx context.Context, id uuid.UUID, codes []string) error {
	return m.Called(ctx, id, codes).Error(0)
}
func (m *MockUserRepo) DeleteRecoveryCodes(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}
func (m *MockUserRepo) UseRecoveryCode(ctx context.Context, id uuid.UUID, hash string) (bool, error) {
	args := m.Called(ctx, id, hash)
	return args.Bool(0), args.Error(1)
}

// MockSessionRepo is a minimal mock for session.Repository.
type MockSessionRepo struct{ mock.Mock }

func (m *MockSessionRepo) Create(ctx context.Context, s *session.Session) error {
	return m.Called(ctx, s).Error(0)
}
func (m *MockSessionRepo) GetByID(ctx context.Context, id uuid.UUID) (*session.Session, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*session.Session), args.Error(1)
}
func (m *MockSessionRepo) GetByRefreshToken(ctx context.Context, hash string) (*session.Session, error) {
	args := m.Called(ctx, hash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*session.Session), args.Error(1)
}
func (m *MockSessionRepo) GetByTokenID(ctx context.Context, id string) (*session.Session, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*session.Session), args.Error(1)
}
func (m *MockSessionRepo) GetActiveByUserID(ctx context.Context, userID uuid.UUID) (*session.Session, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*session.Session), args.Error(1)
}
func (m *MockSessionRepo) Update(ctx context.Context, s *session.Session) error {
	return m.Called(ctx, s).Error(0)
}
func (m *MockSessionRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}
func (m *MockSessionRepo) RevokeByTokenID(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *MockSessionRepo) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	return m.Called(ctx, userID).Error(0)
}
func (m *MockSessionRepo) ListActive(ctx context.Context, p session.ListParams) ([]*session.Info, int64, error) {
	args := m.Called(ctx, p)
	return args.Get(0).([]*session.Info), args.Get(1).(int64), args.Error(2)
}
func (m *MockSessionRepo) CleanupExpired(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}
func (m *MockSessionRepo) UpdateActivity(ctx context.Context, userID uuid.UUID) error {
	return m.Called(ctx, userID).Error(0)
}

// MockAuditRepo is a minimal mock for audit.Repository.
type MockAuditRepo struct{ mock.Mock }

func (m *MockAuditRepo) Create(ctx context.Context, l *audit.Log) error {
	return m.Called(ctx, l).Error(0)
}
func (m *MockAuditRepo) GetByID(ctx context.Context, id uuid.UUID) (*audit.Log, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*audit.Log), args.Error(1)
}
func (m *MockAuditRepo) List(ctx context.Context, p audit.ListParams) ([]*audit.Log, int64, error) {
	args := m.Called(ctx, p)
	return args.Get(0).([]*audit.Log), args.Get(1).(int64), args.Error(2)
}
func (m *MockAuditRepo) GetSummary(ctx context.Context, timeRange string, serviceName string) (*audit.Summary, error) {
	args := m.Called(ctx, timeRange, serviceName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*audit.Summary), args.Error(1)
}

// =============================================================================
// Test helpers
// =============================================================================

// testSecurityCfg returns a SecurityConfig suitable for unit tests.
func testSecurityCfg() *config.SecurityConfig {
	return &config.SecurityConfig{
		MaxLoginAttempts: 5,
		LockoutDuration:  15 * time.Minute,
	}
}

// testJWTService builds a minimal JWT service using in-memory test secrets.
func testJWTService() *jwt.Service {
	cfg := &config.JWTConfig{
		AccessTokenSecret:  "test-access-secret-32byteslong!!!!",
		RefreshTokenSecret: "test-refresh-secret-32byteslng!!!",
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    7 * 24 * time.Hour,
		Issuer:             "goapps-test",
	}
	return jwt.NewService(cfg)
}

// testTOTPService builds a minimal TOTP service.
func testTOTPService() *totp.Service {
	return totp.NewService(&config.TOTPConfig{
		Issuer:    "goapps-test",
		Digits:    6,
		Period:    30,
		Algorithm: "SHA1",
	})
}

// activeUser builds a valid, active user with an Argon2id password hash.
func activeUser(t *testing.T, username, email, plainPassword string) *user.User {
	t.Helper()
	hash, err := password.Hash(plainPassword)
	require.NoError(t, err)
	u, err := user.NewUser(username, email, hash, "test")
	require.NoError(t, err)
	return u
}

// newTestService wires up an appauth.Service with mock repos and no Redis caches.
func newTestService(t *testing.T, userRepo *MockUserRepo, sessionRepo *MockSessionRepo, auditRepo *MockAuditRepo) *appauth.Service {
	t.Helper()
	return appauth.NewService(
		userRepo,
		sessionRepo,
		auditRepo,
		testJWTService(),
		testTOTPService(),
		nil, // sessionCache
		nil, // otpCache
		nil, // rateLimitCache
		testSecurityCfg(),
	)
}

// =============================================================================
// Tests — authenticateUser routing (email vs username)
// =============================================================================

// TestLogin_ByUsername_Success verifies that a plain username (no @) routes to GetByUsername.
func TestLogin_ByUsername_Success(t *testing.T) {
	ctx := context.Background()
	const username = "johndoe"
	const plainPwd = "Password123"

	u := activeUser(t, username, "john@example.com", plainPwd)

	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	auditRepo := new(MockAuditRepo)

	// Expect username lookup — NOT email lookup.
	userRepo.On("GetByUsername", ctx, username).Return(u, nil)
	userRepo.On("Update", ctx, mock.Anything).Return(nil)          // recordSuccessfulLogin updates lastLoginAt
	userRepo.On("GetDetailByUserID", ctx, u.ID()).Return(nil, nil) // getFullName — no detail available
	userRepo.On("GetRolesAndPermissions", ctx, u.ID()).Return([]user.RoleRef{}, []user.PermissionRef{}, nil)
	sessionRepo.On("Create", ctx, mock.AnythingOfType("*session.Session")).Return(nil)
	auditRepo.On("Create", ctx, mock.Anything).Return(nil).Maybe()

	svc := newTestService(t, userRepo, sessionRepo, auditRepo)

	result, err := svc.Login(ctx, domainAuth.LoginInput{
		Username:   username,
		Password:   plainPwd,
		DeviceInfo: "unit-test",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, result.AccessToken)
	assert.Equal(t, username, result.User.Username)

	userRepo.AssertCalled(t, "GetByUsername", ctx, username)
	userRepo.AssertNotCalled(t, "GetByEmail", mock.Anything, mock.Anything)
}

// TestLogin_ByEmail_Success verifies that an identifier containing '@' routes to GetByEmail.
func TestLogin_ByEmail_Success(t *testing.T) {
	ctx := context.Background()
	const email = "john@example.com"
	const plainPwd = "Password123"

	u := activeUser(t, "johndoe", email, plainPwd)

	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	auditRepo := new(MockAuditRepo)

	// Expect email lookup — NOT username lookup.
	userRepo.On("GetByEmail", ctx, email).Return(u, nil)
	userRepo.On("Update", ctx, mock.Anything).Return(nil)          // recordSuccessfulLogin updates lastLoginAt
	userRepo.On("GetDetailByUserID", ctx, u.ID()).Return(nil, nil) // getFullName — no detail available
	userRepo.On("GetRolesAndPermissions", ctx, u.ID()).Return([]user.RoleRef{}, []user.PermissionRef{}, nil)
	sessionRepo.On("Create", ctx, mock.AnythingOfType("*session.Session")).Return(nil)
	auditRepo.On("Create", ctx, mock.Anything).Return(nil).Maybe()

	svc := newTestService(t, userRepo, sessionRepo, auditRepo)

	result, err := svc.Login(ctx, domainAuth.LoginInput{
		Username:   email, // passing email in the "Username" field
		Password:   plainPwd,
		DeviceInfo: "unit-test",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, result.AccessToken)
	assert.Equal(t, "johndoe", result.User.Username)

	userRepo.AssertCalled(t, "GetByEmail", ctx, email)
	userRepo.AssertNotCalled(t, "GetByUsername", mock.Anything, mock.Anything)
}

// TestLogin_ByEmailNotFound_ReturnsInvalidCredentials verifies that a missing email → ErrInvalidCredentials.
func TestLogin_ByEmailNotFound_ReturnsInvalidCredentials(t *testing.T) {
	ctx := context.Background()
	const email = "notfound@example.com"

	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	auditRepo := new(MockAuditRepo)

	userRepo.On("GetByEmail", ctx, email).Return(nil, shared.ErrNotFound)

	svc := newTestService(t, userRepo, sessionRepo, auditRepo)

	_, err := svc.Login(ctx, domainAuth.LoginInput{
		Username: email,
		Password: "anything",
	})

	assert.ErrorIs(t, err, shared.ErrInvalidCredentials)
	userRepo.AssertNotCalled(t, "GetByUsername", mock.Anything, mock.Anything)
}

// TestLogin_ByUsernameNotFound_ReturnsInvalidCredentials verifies missing username → ErrInvalidCredentials.
func TestLogin_ByUsernameNotFound_ReturnsInvalidCredentials(t *testing.T) {
	ctx := context.Background()
	const username = "ghost"

	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	auditRepo := new(MockAuditRepo)

	userRepo.On("GetByUsername", ctx, username).Return(nil, shared.ErrNotFound)

	svc := newTestService(t, userRepo, sessionRepo, auditRepo)

	_, err := svc.Login(ctx, domainAuth.LoginInput{
		Username: username,
		Password: "anything",
	})

	assert.ErrorIs(t, err, shared.ErrInvalidCredentials)
	userRepo.AssertNotCalled(t, "GetByEmail", mock.Anything, mock.Anything)
}

// TestLogin_WrongPassword_ReturnsInvalidCredentials covers wrong password path.
func TestLogin_WrongPassword_ReturnsInvalidCredentials(t *testing.T) {
	ctx := context.Background()
	const username = "johndoe"

	u := activeUser(t, username, "john@example.com", "correctPassword123")

	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	auditRepo := new(MockAuditRepo)

	userRepo.On("GetByUsername", ctx, username).Return(u, nil)
	userRepo.On("Update", ctx, mock.Anything).Return(nil) // RecordLoginFailure triggers Update

	svc := newTestService(t, userRepo, sessionRepo, auditRepo)

	_, err := svc.Login(ctx, domainAuth.LoginInput{
		Username: username,
		Password: "wrongPassword",
	})

	assert.ErrorIs(t, err, shared.ErrInvalidCredentials)
}
