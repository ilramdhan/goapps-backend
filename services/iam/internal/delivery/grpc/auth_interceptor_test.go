package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
	iamjwt "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/jwt"
)

const testSecret = "test-secret-for-unit-tests-only"

func newTestJWTService() *iamjwt.Service {
	return iamjwt.NewService(&config.JWTConfig{
		AccessTokenSecret:  testSecret,
		RefreshTokenSecret: "test-refresh-secret",
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    7 * 24 * time.Hour,
		Issuer:             "test-issuer",
	})
}

func generateTestAccessToken(t *testing.T, jwtSvc *iamjwt.Service) string {
	t.Helper()
	pair, err := jwtSvc.GenerateTokenPair(
		uuid.New(), "testuser", "test@example.com",
		[]string{"ADMIN"}, []string{"iam.user.account.view", "iam.rbac.role.view"}, nil,
	)
	require.NoError(t, err)
	return pair.AccessToken
}

func ctxWithToken(token string) context.Context {
	md := metadata.New(map[string]string{
		"authorization": "Bearer " + token,
	})
	return metadata.NewIncomingContext(context.Background(), md)
}

func noopHandler(_ context.Context, _ any) (any, error) {
	return "ok", nil
}

func TestIsPublicMethod(t *testing.T) {
	tests := []struct {
		method   string
		isPublic bool
	}{
		{"/iam.v1.AuthService/Login", true},
		{"/iam.v1.AuthService/RefreshToken", true},
		{"/iam.v1.AuthService/ForgotPassword", true},
		{"/iam.v1.AuthService/Logout", true},
		{"/grpc.health.v1.Health/Check", true},
		{"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo", true},
		{"/iam.v1.UserService/ListUsers", false},
		{"/iam.v1.RoleService/CreateRole", false},
		{"/iam.v1.AuthService/GetCurrentUser", false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			assert.Equal(t, tt.isPublic, isPublicMethod(tt.method))
		})
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no metadata",
			ctx:     context.Background(),
			wantErr: true,
		},
		{
			name: "no authorization header",
			ctx: metadata.NewIncomingContext(context.Background(),
				metadata.New(map[string]string{"other": "value"})),
			wantErr: true,
		},
		{
			name: "invalid format (no Bearer prefix)",
			ctx: metadata.NewIncomingContext(context.Background(),
				metadata.New(map[string]string{"authorization": "Basic abc123"})),
			wantErr: true,
		},
		{
			name: "empty token after Bearer",
			ctx: metadata.NewIncomingContext(context.Background(),
				metadata.New(map[string]string{"authorization": "Bearer "})),
			wantErr: true,
		},
		{
			name:    "valid bearer token",
			ctx:     ctxWithToken("some-token"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := extractBearerToken(tt.ctx)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, "some-token", token)
			}
		})
	}
}

func TestAuthInterceptor_PublicMethod(t *testing.T) {
	jwtSvc := newTestJWTService()
	interceptor := AuthInterceptor(jwtSvc, nil, nil)

	// Public methods should pass without any token.
	info := &grpc.UnaryServerInfo{FullMethod: "/iam.v1.AuthService/Login"}
	resp, err := interceptor(context.Background(), nil, info, noopHandler)

	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestAuthInterceptor_MissingToken(t *testing.T) {
	jwtSvc := newTestJWTService()
	interceptor := AuthInterceptor(jwtSvc, nil, nil)

	info := &grpc.UnaryServerInfo{FullMethod: "/iam.v1.UserService/ListUsers"}
	_, err := interceptor(context.Background(), nil, info, noopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestAuthInterceptor_InvalidToken(t *testing.T) {
	jwtSvc := newTestJWTService()
	interceptor := AuthInterceptor(jwtSvc, nil, nil)

	ctx := ctxWithToken("invalid-jwt-token")
	info := &grpc.UnaryServerInfo{FullMethod: "/iam.v1.UserService/ListUsers"}
	_, err := interceptor(ctx, nil, info, noopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestAuthInterceptor_ExpiredToken(t *testing.T) {
	// Create a token that is already expired.
	claims := &iamjwt.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test-issuer",
			Subject:   uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			ID:        uuid.New().String(),
		},
		TokenType: "access",
		UserID:    uuid.New().String(),
		Username:  "expired-user",
		Email:     "expired@example.com",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(testSecret))
	require.NoError(t, err)

	jwtSvc := newTestJWTService()
	interceptor := AuthInterceptor(jwtSvc, nil, nil)

	ctx := ctxWithToken(tokenStr)
	info := &grpc.UnaryServerInfo{FullMethod: "/iam.v1.UserService/ListUsers"}
	_, err = interceptor(ctx, nil, info, noopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestAuthInterceptor_ValidToken(t *testing.T) {
	jwtSvc := newTestJWTService()
	accessToken := generateTestAccessToken(t, jwtSvc)

	interceptor := AuthInterceptor(jwtSvc, nil, nil)

	ctx := ctxWithToken(accessToken)
	info := &grpc.UnaryServerInfo{FullMethod: "/iam.v1.UserService/ListUsers"}

	handlerCalled := false
	handler := func(ctx context.Context, _ any) (any, error) {
		handlerCalled = true

		// Verify context values were populated.
		userID, ok := GetUserIDFromCtx(ctx)
		assert.True(t, ok)
		assert.NotEmpty(t, userID)

		username := GetUsernameFromCtx(ctx)
		assert.Equal(t, "testuser", username)

		roles := GetRolesFromCtx(ctx)
		assert.Contains(t, roles, "ADMIN")

		perms := GetPermissionsFromCtx(ctx)
		assert.Contains(t, perms, "iam.user.account.view")

		return "ok", nil
	}

	resp, err := interceptor(ctx, nil, info, handler)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.True(t, handlerCalled)
}

func TestAuthInterceptor_WrongSigningSecret(t *testing.T) {
	// Generate token with a different secret.
	wrongSvc := iamjwt.NewService(&config.JWTConfig{
		AccessTokenSecret:  "wrong-secret",
		RefreshTokenSecret: "wrong-refresh",
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    7 * 24 * time.Hour,
		Issuer:             "test-issuer",
	})
	pair, err := wrongSvc.GenerateTokenPair(
		uuid.New(), "testuser", "test@example.com",
		[]string{"ADMIN"}, nil, nil,
	)
	require.NoError(t, err)

	// Validate with the correct service (different secret).
	jwtSvc := newTestJWTService()
	interceptor := AuthInterceptor(jwtSvc, nil, nil)

	ctx := ctxWithToken(pair.AccessToken)
	info := &grpc.UnaryServerInfo{FullMethod: "/iam.v1.UserService/ListUsers"}
	_, err = interceptor(ctx, nil, info, noopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()

	// Empty context returns defaults.
	_, ok := GetUserIDFromCtx(ctx)
	assert.False(t, ok)
	assert.Equal(t, "", GetUsernameFromCtx(ctx))
	assert.Nil(t, GetRolesFromCtx(ctx))
	assert.Nil(t, GetPermissionsFromCtx(ctx))
	assert.False(t, IsSuperAdmin(ctx))

	// Populated context.
	ctx = context.WithValue(ctx, UserIDKey, "user-123")
	ctx = context.WithValue(ctx, UsernameKey, "admin")
	ctx = context.WithValue(ctx, RolesKey, []string{"SUPER_ADMIN", "ADMIN"})
	ctx = context.WithValue(ctx, PermissionsKey, []string{"iam.user.account.view"})

	userID, ok := GetUserIDFromCtx(ctx)
	assert.True(t, ok)
	assert.Equal(t, "user-123", userID)
	assert.Equal(t, "admin", GetUsernameFromCtx(ctx))
	assert.True(t, HasRole(ctx, "SUPER_ADMIN"))
	assert.True(t, HasRole(ctx, "ADMIN"))
	assert.False(t, HasRole(ctx, "VIEWER"))
	assert.True(t, HasPermission(ctx, "iam.user.account.view"))
	assert.False(t, HasPermission(ctx, "iam.user.account.delete"))
	assert.True(t, IsSuperAdmin(ctx))
}
