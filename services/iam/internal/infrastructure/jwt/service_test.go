package jwt

import (
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
)

func newTestService() *Service {
	return NewService(&config.JWTConfig{
		AccessTokenSecret:  "test-access-secret-key-32bytes!!",
		RefreshTokenSecret: "test-refresh-secret-key-32bytes!",
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    7 * 24 * time.Hour,
		Issuer:             "test-iam",
	})
}

func TestGenerateTokenPair(t *testing.T) {
	svc := newTestService()
	userID := uuid.New()
	roles := []string{"admin", "user"}
	permissions := []string{"finance.master.uom.view", "finance.master.uom.create"}
	serviceAccess := []string{"finance", "iam"}

	pair, err := svc.GenerateTokenPair(userID, "johndoe", "john@example.com", roles, permissions, serviceAccess)
	require.NoError(t, err)
	require.NotNil(t, pair)

	// Access and refresh tokens should be non-empty and different.
	assert.NotEmpty(t, pair.AccessToken)
	assert.NotEmpty(t, pair.RefreshToken)
	assert.NotEqual(t, pair.AccessToken, pair.RefreshToken)

	// Expiry times should be in the future.
	assert.True(t, pair.AccessExp.After(time.Now()))
	assert.True(t, pair.RefreshExp.After(time.Now()))

	// Refresh expiry should be later than access expiry.
	assert.True(t, pair.RefreshExp.After(pair.AccessExp))

	// Token ID should be a valid UUID.
	_, err = uuid.Parse(pair.TokenID)
	assert.NoError(t, err)
}

func TestGenerateTokenPair_ContainsExpectedClaims(t *testing.T) {
	svc := newTestService()
	userID := uuid.New()
	roles := []string{"admin"}
	permissions := []string{"iam.user.view"}
	serviceAccess := []string{"iam"}

	pair, err := svc.GenerateTokenPair(userID, "janedoe", "jane@example.com", roles, permissions, serviceAccess)
	require.NoError(t, err)

	// Validate access token claims.
	accessClaims, err := svc.ValidateAccessToken(pair.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, TokenTypeAccess, accessClaims.TokenType)
	assert.Equal(t, userID.String(), accessClaims.UserID)
	assert.Equal(t, userID.String(), accessClaims.Subject)
	assert.Equal(t, "janedoe", accessClaims.Username)
	assert.Equal(t, "jane@example.com", accessClaims.Email)
	assert.Equal(t, roles, accessClaims.Roles)
	assert.Equal(t, permissions, accessClaims.Permissions)
	assert.Equal(t, serviceAccess, accessClaims.ServiceAccess)
	assert.Equal(t, "test-iam", accessClaims.Issuer)

	// Validate refresh token claims.
	refreshClaims, err := svc.ValidateRefreshToken(pair.RefreshToken)
	require.NoError(t, err)
	assert.Equal(t, TokenTypeRefresh, refreshClaims.TokenType)
	assert.Equal(t, userID.String(), refreshClaims.UserID)
	assert.Equal(t, userID.String(), refreshClaims.Subject)
	assert.Equal(t, "janedoe", refreshClaims.Username)
	assert.Equal(t, "jane@example.com", refreshClaims.Email)

	// Refresh token should NOT contain roles, permissions, or service access.
	assert.Nil(t, refreshClaims.Roles)
	assert.Nil(t, refreshClaims.Permissions)
	assert.Nil(t, refreshClaims.ServiceAccess)

	// Refresh token JTI should match TokenID.
	assert.Equal(t, pair.TokenID, refreshClaims.ID)
}

func TestGenerateTokenPair_NilSlicesOmitted(t *testing.T) {
	svc := newTestService()
	userID := uuid.New()

	pair, err := svc.GenerateTokenPair(userID, "user1", "user1@example.com", nil, nil, nil)
	require.NoError(t, err)

	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	require.NoError(t, err)
	assert.Nil(t, claims.Roles)
	assert.Nil(t, claims.Permissions)
	assert.Nil(t, claims.ServiceAccess)
}

func TestValidateAccessToken_Valid(t *testing.T) {
	svc := newTestService()
	userID := uuid.New()

	pair, err := svc.GenerateTokenPair(userID, "testuser", "test@example.com", nil, nil, nil)
	require.NoError(t, err)

	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, userID.String(), claims.UserID)
	assert.Equal(t, TokenTypeAccess, claims.TokenType)
}

func TestValidateAccessToken_ExpiredToken(t *testing.T) {
	svc := &Service{
		accessSecret:  []byte("test-secret"),
		refreshSecret: []byte("test-refresh"),
		accessTTL:     -1 * time.Hour, // Already expired.
		refreshTTL:    7 * 24 * time.Hour,
		issuer:        "test-iam",
	}

	pair, err := svc.GenerateTokenPair(uuid.New(), "user", "u@example.com", nil, nil, nil)
	require.NoError(t, err)

	_, err = svc.ValidateAccessToken(pair.AccessToken)
	assert.ErrorIs(t, err, ErrExpiredToken)
}

func TestValidateAccessToken_InvalidTokenString(t *testing.T) {
	svc := newTestService()

	_, err := svc.ValidateAccessToken("not-a-valid-token")
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateAccessToken_WrongSecret(t *testing.T) {
	svc := newTestService()
	userID := uuid.New()

	pair, err := svc.GenerateTokenPair(userID, "testuser", "test@example.com", nil, nil, nil)
	require.NoError(t, err)

	// Create a service with a different secret.
	otherSvc := NewService(&config.JWTConfig{
		AccessTokenSecret:  "different-access-secret-key!!!!!",
		RefreshTokenSecret: "different-refresh-secret-key!!!!",
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    7 * 24 * time.Hour,
		Issuer:             "test-iam",
	})

	_, err = otherSvc.ValidateAccessToken(pair.AccessToken)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateRefreshToken_Valid(t *testing.T) {
	svc := newTestService()
	userID := uuid.New()

	pair, err := svc.GenerateTokenPair(userID, "testuser", "test@example.com", nil, nil, nil)
	require.NoError(t, err)

	claims, err := svc.ValidateRefreshToken(pair.RefreshToken)
	require.NoError(t, err)
	assert.Equal(t, userID.String(), claims.UserID)
	assert.Equal(t, TokenTypeRefresh, claims.TokenType)
}

func TestValidateRefreshToken_ExpiredToken(t *testing.T) {
	svc := &Service{
		accessSecret:  []byte("test-secret"),
		refreshSecret: []byte("test-refresh"),
		accessTTL:     15 * time.Minute,
		refreshTTL:    -1 * time.Hour, // Already expired.
		issuer:        "test-iam",
	}

	pair, err := svc.GenerateTokenPair(uuid.New(), "user", "u@example.com", nil, nil, nil)
	require.NoError(t, err)

	_, err = svc.ValidateRefreshToken(pair.RefreshToken)
	assert.ErrorIs(t, err, ErrExpiredToken)
}

func TestValidateToken_WrongTokenType(t *testing.T) {
	svc := newTestService()
	userID := uuid.New()

	pair, err := svc.GenerateTokenPair(userID, "testuser", "test@example.com", nil, nil, nil)
	require.NoError(t, err)

	// Validate access token as refresh should fail.
	_, err = svc.ValidateRefreshToken(pair.AccessToken)
	assert.ErrorIs(t, err, ErrInvalidToken) // Wrong secret means invalid, not type mismatch.

	// Validate refresh token as access should fail.
	_, err = svc.ValidateAccessToken(pair.RefreshToken)
	assert.ErrorIs(t, err, ErrInvalidToken) // Wrong secret means invalid.
}

func TestValidateToken_WrongTokenType_SameSecret(t *testing.T) {
	// When access and refresh share the same secret, wrong type is caught.
	svc := &Service{
		accessSecret:  []byte("shared-secret-for-both-tokens!!"),
		refreshSecret: []byte("shared-secret-for-both-tokens!!"),
		accessTTL:     15 * time.Minute,
		refreshTTL:    7 * 24 * time.Hour,
		issuer:        "test-iam",
	}

	pair, err := svc.GenerateTokenPair(uuid.New(), "testuser", "test@example.com", nil, nil, nil)
	require.NoError(t, err)

	// Access token validated as refresh should return ErrInvalidTokenType.
	_, err = svc.ValidateRefreshToken(pair.AccessToken)
	assert.ErrorIs(t, err, ErrInvalidTokenType)

	// Refresh token validated as access should return ErrInvalidTokenType.
	_, err = svc.ValidateAccessToken(pair.RefreshToken)
	assert.ErrorIs(t, err, ErrInvalidTokenType)
}

func TestValidateToken_InvalidSigningMethod(t *testing.T) {
	// Create a token with an unsupported signing method (RSA "none" attack simulation).
	claims := &Claims{
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer:    "test-iam",
			Subject:   uuid.New().String(),
			IssuedAt:  jwtlib.NewNumericDate(time.Now()),
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
		TokenType: TokenTypeAccess,
		UserID:    uuid.New().String(),
	}

	// Sign with "none" method — should be rejected.
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodNone, claims)
	tokenString, err := token.SignedString(jwtlib.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	svc := newTestService()
	_, err = svc.ValidateAccessToken(tokenString)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateToken_EmptyString(t *testing.T) {
	svc := newTestService()

	_, err := svc.ValidateAccessToken("")
	assert.ErrorIs(t, err, ErrInvalidToken)

	_, err = svc.ValidateRefreshToken("")
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestGetTTLSeconds(t *testing.T) {
	svc := newTestService()

	assert.Equal(t, int64(900), svc.GetAccessTTLSeconds())     // 15 min = 900s
	assert.Equal(t, int64(604800), svc.GetRefreshTTLSeconds()) // 7 days = 604800s
}

func TestGenerateTokenPair_UniquePerCall(t *testing.T) {
	svc := newTestService()
	userID := uuid.New()

	pair1, err := svc.GenerateTokenPair(userID, "user", "user@example.com", nil, nil, nil)
	require.NoError(t, err)

	pair2, err := svc.GenerateTokenPair(userID, "user", "user@example.com", nil, nil, nil)
	require.NoError(t, err)

	// Even for the same user, tokens should differ (unique JTI/timestamps).
	assert.NotEqual(t, pair1.AccessToken, pair2.AccessToken)
	assert.NotEqual(t, pair1.RefreshToken, pair2.RefreshToken)
	assert.NotEqual(t, pair1.TokenID, pair2.TokenID)
}

func TestNewService(t *testing.T) {
	cfg := &config.JWTConfig{
		AccessTokenSecret:  "access-secret",
		RefreshTokenSecret: "refresh-secret",
		AccessTokenTTL:     10 * time.Minute,
		RefreshTokenTTL:    24 * time.Hour,
		Issuer:             "my-issuer",
	}

	svc := NewService(cfg)
	require.NotNil(t, svc)
	assert.Equal(t, []byte("access-secret"), svc.accessSecret)
	assert.Equal(t, []byte("refresh-secret"), svc.refreshSecret)
	assert.Equal(t, 10*time.Minute, svc.accessTTL)
	assert.Equal(t, 24*time.Hour, svc.refreshTTL)
	assert.Equal(t, "my-issuer", svc.issuer)
}

func TestGenerateTokenPair_EmptySecret(t *testing.T) {
	// An empty secret should still produce tokens (HMAC allows empty key).
	svc := NewService(&config.JWTConfig{
		AccessTokenSecret:  "",
		RefreshTokenSecret: "",
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    7 * 24 * time.Hour,
		Issuer:             "test",
	})

	pair, err := svc.GenerateTokenPair(uuid.New(), "user", "u@example.com", nil, nil, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, pair.AccessToken)
	assert.NotEmpty(t, pair.RefreshToken)

	// Tokens should still validate with matching (empty) secret.
	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, "user", claims.Username)
}
