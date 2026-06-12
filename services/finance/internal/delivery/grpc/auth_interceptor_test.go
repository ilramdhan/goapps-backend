package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

const testJWTSecret = "finance-test-secret-for-unit-tests"

func testJWTConfig() *config.JWTConfig {
	return &config.JWTConfig{
		AccessTokenSecret: testJWTSecret,
		Issuer:            "test-issuer",
	}
}

func signTestToken(t *testing.T, claims *JWTClaims, secret string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func validAccessClaims() *JWTClaims {
	return &JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test-issuer",
			Subject:   "user-abc-123",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			ID:        "jti-123",
		},
		TokenType:   "access",
		UserID:      "user-abc-123",
		Username:    "testuser",
		Email:       "test@example.com",
		Roles:       []string{"ADMIN"},
		Permissions: []string{"finance.master.uom.view", "finance.master.uom.create"},
	}
}

func financeCtxWithToken(token string) context.Context {
	md := metadata.New(map[string]string{
		"authorization": "Bearer " + token,
	})
	return metadata.NewIncomingContext(context.Background(), md)
}

func financeNoopHandler(_ context.Context, _ any) (any, error) {
	return "ok", nil
}

func TestFinanceAuthInterceptor_HealthBypass(t *testing.T) {
	interceptor := AuthInterceptor(testJWTConfig(), nil, nil)

	tests := []string{
		"/grpc.health.v1.Health/Check",
		"/grpc.health.v1.Health/Watch",
		"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
	}

	for _, method := range tests {
		t.Run(method, func(t *testing.T) {
			info := &grpc.UnaryServerInfo{FullMethod: method}
			resp, err := interceptor(context.Background(), nil, info, financeNoopHandler)
			assert.NoError(t, err)
			assert.Equal(t, "ok", resp)
		})
	}
}

func TestFinanceAuthInterceptor_MissingToken(t *testing.T) {
	interceptor := AuthInterceptor(testJWTConfig(), nil, nil)

	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.UOMService/ListUOMs"}
	_, err := interceptor(context.Background(), nil, info, financeNoopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestFinanceAuthInterceptor_InvalidToken(t *testing.T) {
	interceptor := AuthInterceptor(testJWTConfig(), nil, nil)

	ctx := financeCtxWithToken("garbage-token")
	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.UOMService/ListUOMs"}
	_, err := interceptor(ctx, nil, info, financeNoopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestFinanceAuthInterceptor_ExpiredToken(t *testing.T) {
	claims := validAccessClaims()
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-1 * time.Hour))
	claims.IssuedAt = jwt.NewNumericDate(time.Now().Add(-2 * time.Hour))

	token := signTestToken(t, claims, testJWTSecret)
	interceptor := AuthInterceptor(testJWTConfig(), nil, nil)

	ctx := financeCtxWithToken(token)
	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.UOMService/ListUOMs"}
	_, err := interceptor(ctx, nil, info, financeNoopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestFinanceAuthInterceptor_RefreshTokenRejected(t *testing.T) {
	claims := validAccessClaims()
	claims.TokenType = "refresh" // Should be rejected.

	token := signTestToken(t, claims, testJWTSecret)
	interceptor := AuthInterceptor(testJWTConfig(), nil, nil)

	ctx := financeCtxWithToken(token)
	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.UOMService/ListUOMs"}
	_, err := interceptor(ctx, nil, info, financeNoopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestFinanceAuthInterceptor_WrongSecret(t *testing.T) {
	claims := validAccessClaims()
	token := signTestToken(t, claims, "wrong-secret")

	interceptor := AuthInterceptor(testJWTConfig(), nil, nil)

	ctx := financeCtxWithToken(token)
	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.UOMService/ListUOMs"}
	_, err := interceptor(ctx, nil, info, financeNoopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestFinanceAuthInterceptor_ValidToken(t *testing.T) {
	claims := validAccessClaims()
	token := signTestToken(t, claims, testJWTSecret)

	interceptor := AuthInterceptor(testJWTConfig(), nil, nil)

	ctx := financeCtxWithToken(token)
	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.UOMService/ListUOMs"}

	handlerCalled := false
	handler := func(ctx context.Context, _ any) (any, error) {
		handlerCalled = true

		userID, ok := GetUserIDFromCtx(ctx)
		assert.True(t, ok)
		assert.Equal(t, "user-abc-123", userID)

		roles := GetRolesFromCtx(ctx)
		assert.Contains(t, roles, "ADMIN")

		perms := GetPermissionsFromCtx(ctx)
		assert.Contains(t, perms, "finance.master.uom.view")

		return "ok", nil
	}

	resp, err := interceptor(ctx, nil, info, handler)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.True(t, handlerCalled)
}

func TestFinancePermissionInterceptor_SuperAdminBypass(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := context.WithValue(context.Background(), AuthRolesKey, []string{"SUPER_ADMIN"})
	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.UOMService/DeleteUOM"}

	resp, err := interceptor(ctx, nil, info, financeNoopHandler)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestFinancePermissionInterceptor_HasPermission(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := context.WithValue(context.Background(), AuthRolesKey, []string{"FINANCE_ADMIN"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{"finance.master.uom.view"})

	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.UOMService/ListUOMs"}
	resp, err := interceptor(ctx, nil, info, financeNoopHandler)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestFinancePermissionInterceptor_MissingPermission(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := context.WithValue(context.Background(), AuthRolesKey, []string{"VIEWER"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{"finance.master.uom.view"})

	// User has view but tries to create.
	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.UOMService/CreateUOM"}
	_, err := interceptor(ctx, nil, info, financeNoopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestFinancePermissionInterceptor_HealthBypass(t *testing.T) {
	interceptor := PermissionInterceptor()

	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	resp, err := interceptor(context.Background(), nil, info, financeNoopHandler)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestFinanceGetRequiredPermission(t *testing.T) {
	tests := []struct {
		method string
		want   string
	}{
		{"/finance.v1.UOMService/CreateUOM", "finance.master.uom.create"},
		{"/finance.v1.UOMService/GetUOM", "finance.master.uom.view"},
		{"/finance.v1.UOMService/ListUOMs", "finance.master.uom.view"},
		{"/finance.v1.UOMService/UpdateUOM", "finance.master.uom.update"},
		{"/finance.v1.UOMService/DeleteUOM", "finance.master.uom.delete"},
		{"/finance.v1.UOMService/ImportUOM", "finance.master.uom.create"},
		{"/finance.v1.UOMService/ExportUOM", "finance.master.uom.view"},
		{"/finance.v1.UnknownService/Unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			assert.Equal(t, tt.want, getRequiredPermission(tt.method))
		})
	}
}

func TestFinanceContextHelpers(t *testing.T) {
	ctx := context.Background()

	// Empty context.
	_, ok := GetUserIDFromCtx(ctx)
	assert.False(t, ok)
	assert.Nil(t, GetRolesFromCtx(ctx))
	assert.Nil(t, GetPermissionsFromCtx(ctx))
	assert.False(t, IsSuperAdmin(ctx))

	// Populated context.
	ctx = context.WithValue(ctx, AuthUserIDKey, "uid-1")
	ctx = context.WithValue(ctx, AuthRolesKey, []string{"SUPER_ADMIN"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{"finance.master.uom.view"})

	userID, ok := GetUserIDFromCtx(ctx)
	assert.True(t, ok)
	assert.Equal(t, "uid-1", userID)
	assert.True(t, IsSuperAdmin(ctx))
	assert.True(t, HasPermission(ctx, "finance.master.uom.view"))
	assert.False(t, HasPermission(ctx, "finance.master.uom.delete"))
}

func TestFinanceGetRequiredPermission_CPR(t *testing.T) {
	tests := []struct {
		method string
		want   string
	}{
		// CPR transitions that require explicit permissions.
		{"/finance.v1.CostProductRequestService/SubmitCostProductRequest", "finance.product.request.submit"},
		{"/finance.v1.CostProductRequestService/ReopenCostProductRequest", "finance.product.request.reopen"},
		{"/finance.v1.CostProductRequestService/StartCostProductRequestReview", "finance.product.request.review"},
		{"/finance.v1.CostProductRequestService/RejectCostProductRequest", "finance.product.request.reject"},
		{"/finance.v1.CostProductRequestService/CreateCostProductRequest", "finance.product.request.create"},
		// CPR transitions open to any authenticated user (empty string).
		{"/finance.v1.CostProductRequestService/CancelCostProductRequest", ""},
		{"/finance.v1.CostProductRequestService/CloseCostProductRequest", ""},
		// Route RPCs.
		{"/finance.v1.CostRouteService/CreateRouteFromProduct", "finance.product.route.create"},
		{"/finance.v1.CostRouteService/GetRouteByProduct", "finance.product.route.view"},
		{"/finance.v1.CostRouteService/LockRoute", "finance.product.route.update"},
		// Fill-task RPCs open to any authenticated user.
		{"/finance.v1.CostFillTaskService/ClaimFillTask", ""},
		{"/finance.v1.CostFillTaskService/SubmitFillTask", ""},
		{"/finance.v1.CostFillTaskService/ApproveFillTask", ""},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			assert.Equal(t, tt.want, getRequiredPermission(tt.method))
		})
	}
}

func TestFinancePermissionInterceptor_CPRSubmit_Allowed(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := context.WithValue(context.Background(), AuthRolesKey, []string{"CPR_SUBMITTER"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{"finance.product.request.submit"})

	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.CostProductRequestService/SubmitCostProductRequest"}
	resp, err := interceptor(ctx, nil, info, financeNoopHandler)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestFinancePermissionInterceptor_CPRSubmit_Denied(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := context.WithValue(context.Background(), AuthRolesKey, []string{"CPR_REQUESTER"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{"finance.product.request.view"})

	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.CostProductRequestService/SubmitCostProductRequest"}
	_, err := interceptor(ctx, nil, info, financeNoopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}
