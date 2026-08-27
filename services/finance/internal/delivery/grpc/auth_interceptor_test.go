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

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
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
		// nama metode dibetulkan ke bentuk generated (jamak) seiring perbaikan kunci basi, K-36
		{"/finance.v1.UOMService/ImportUOMs", "finance.master.uom.create"},
		{"/finance.v1.UOMService/ExportUOMs", "finance.master.uom.view"},
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
		// SubmitAndDecide merges Submit+StartReview+VerifyClassification+DecideFeasibility+
		// LinkRoute and is gated SOLELY by the review permission (design.md §3 B3
		// permission narrowing) -- not also by .submit or .resolve.
		{"/finance.v1.CostProductRequestService/SubmitAndDecideCostProductRequest", "finance.product.request.review"},
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

// TestFinancePermissionInterceptor_SubmitAndDecide_AllowedWithReviewOnly verifies
// the merged SubmitAndDecide action is gated solely by finance.product.request.review --
// holding ONLY that permission (no .submit, no .resolve) is sufficient.
func TestFinancePermissionInterceptor_SubmitAndDecide_AllowedWithReviewOnly(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := context.WithValue(context.Background(), AuthRolesKey, []string{"CPR_REVIEWER"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{"finance.product.request.review"})

	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.CostProductRequestService/SubmitAndDecideCostProductRequest"}
	resp, err := interceptor(ctx, nil, info, financeNoopHandler)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

// TestFinancePermissionInterceptor_SubmitAndDecide_DeniedWithSubmitOnly verifies that
// holding ONLY the old .submit permission (and not .review) is NOT sufficient for the
// merged action -- this is the intentional permission narrowing from design.md §3 B3.
func TestFinancePermissionInterceptor_SubmitAndDecide_DeniedWithSubmitOnly(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := context.WithValue(context.Background(), AuthRolesKey, []string{"CPR_SUBMITTER"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{"finance.product.request.submit"})

	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.CostProductRequestService/SubmitAndDecideCostProductRequest"}
	_, err := interceptor(ctx, nil, info, financeNoopHandler)

	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

// TestFinanceGetRequiredPermission_MBDozing pins the exact permission codes for
// the MB dozing RPCs.
func TestFinanceGetRequiredPermission_MBDozing(t *testing.T) {
	tests := []struct {
		method string
		want   string
	}{
		{"/finance.v1.MBDozingService/CalculateDozing", "finance.mb.dozing.calculate"},
		{"/finance.v1.MBDozingService/PreviewDozingImpact", "finance.mb.dozing.preview"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			assert.Equal(t, tt.want, getRequiredPermission(tt.method))
		})
	}
}

// TestMBDozingServiceRPCsAllRequirePermission guards against fail-open.
//
// getRequiredPermission returns "" for any unmapped method, and the interceptor
// treats "" as "authenticated access is sufficient" (auth_interceptor.go:331-334).
// An MB dozing RPC that is forgotten in the permission map would therefore be
// silently reachable by every logged-in user. This test enumerates the RPCs from
// the GENERATED service descriptor rather than a hand-written list, so adding a
// new RPC to MBDozingService fails here until it is mapped.
//
// NOTE (K-34 c): this per-service guard is now SUPERSET by
// permission_coverage_test.go, which applies the same check to all 50
// registered services. Kept deliberately: it carries the original intent and
// history for this service, and a focused failure here names the service
// directly. Do not delete.
func TestMBDozingServiceRPCsAllRequirePermission(t *testing.T) {
	desc := financev1.MBDozingService_ServiceDesc
	require.Equal(t, "finance.v1.MBDozingService", desc.ServiceName)
	require.NotEmpty(t, desc.Methods)

	for _, m := range desc.Methods {
		fullMethod := "/" + desc.ServiceName + "/" + m.MethodName
		t.Run(fullMethod, func(t *testing.T) {
			got := getRequiredPermission(fullMethod)
			assert.NotEmpty(t, got,
				"RPC %s has no entry in the permission map; unmapped methods fail OPEN to any authenticated user", fullMethod)
		})
	}
}

// TestMbCrossSectionFactorServiceRPCsAllRequirePermission guards against
// fail-open for the MB cross-section conversion-factor CRUD.
//
// getRequiredPermission returns "" for any unmapped method, and the interceptor
// treats "" as "authenticated access is sufficient" (auth_interceptor.go:331-334).
// A factor RPC forgotten in the permission map would therefore be silently
// writable by every logged-in user — and factors feed the LDR calculation. Like
// the MBDozingService test above, this enumerates the RPCs from the GENERATED
// service descriptor rather than a hand-written list, so a new RPC added to
// MbCrossSectionFactorService fails here until it is mapped.
//
// NOTE (K-34 c): this per-service guard is now SUPERSET by
// permission_coverage_test.go, which applies the same check to all 50
// registered services. Kept deliberately: it carries the original intent and
// history for this service, and a focused failure here names the service
// directly. Do not delete.
func TestMbCrossSectionFactorServiceRPCsAllRequirePermission(t *testing.T) {
	desc := financev1.MbCrossSectionFactorService_ServiceDesc
	require.Equal(t, "finance.v1.MbCrossSectionFactorService", desc.ServiceName)
	require.NotEmpty(t, desc.Methods)

	for _, m := range desc.Methods {
		fullMethod := "/" + desc.ServiceName + "/" + m.MethodName
		t.Run(fullMethod, func(t *testing.T) {
			got := getRequiredPermission(fullMethod)
			assert.NotEmpty(t, got,
				"RPC %s has no entry in the permission map; unmapped methods fail OPEN to any authenticated user", fullMethod)
		})
	}
}

// TestMbCrossSectionFactorPermissionsAreNotTheMasterOnes pins user decision
// K-21 (gate G17-FAKTOR-PERM, 2026-08-22): the factor RPCs must NOT piggy-back
// on the finance.master.mbxsection.* master-code permissions. Before K-21 they
// did (auth_interceptor.go, 5 entries). This test makes a regression back to
// piggy-backing fail loudly instead of passing silently.
func TestMbCrossSectionFactorPermissionsAreNotTheMasterOnes(t *testing.T) {
	want := map[string]string{
		"CreateMbCrossSectionFactor": "finance.master.mbxsectionfactor.create",
		"UpdateMbCrossSectionFactor": "finance.master.mbxsectionfactor.update",
		"DeleteMbCrossSectionFactor": "finance.master.mbxsectionfactor.delete",
		"GetMbCrossSectionFactor":    "finance.master.mbxsectionfactor.view",
		"ListMbCrossSectionFactor":   "finance.master.mbxsectionfactor.view",
	}

	desc := financev1.MbCrossSectionFactorService_ServiceDesc
	require.Len(t, desc.Methods, len(want),
		"MbCrossSectionFactorService gained or lost an RPC; update this K-21 expectation map")

	for _, m := range desc.Methods {
		fullMethod := "/" + desc.ServiceName + "/" + m.MethodName
		t.Run(m.MethodName, func(t *testing.T) {
			expected, ok := want[m.MethodName]
			require.True(t, ok, "RPC %s is not covered by the K-21 expectation map", m.MethodName)

			got := getRequiredPermission(fullMethod)
			assert.Equal(t, expected, got, "RPC %s maps to the wrong permission", fullMethod)
			assert.NotContains(t, got, "finance.master.mbxsection.",
				"K-21 forbids piggy-backing factor RPC %s on the mbxsection master permissions", fullMethod)
		})
	}
}

// TestRejectMBHeadRequiresRejectPermission pins the permission gating the
// SUBMITTED → REJECTED transition (user decision K-2).
//
// getRequiredPermission returns "" for any unmapped method and the interceptor
// treats "" as "authenticated access is sufficient" (auth_interceptor.go:331-334),
// so dropping this entry would let ANY logged-in user reject a submitted MB head.
func TestRejectMBHeadRequiresRejectPermission(t *testing.T) {
	got := getRequiredPermission("/finance.v1.MBHeadService/RejectMBHead")

	assert.Equal(t, "finance.mb.head.reject", got,
		"RejectMBHead must gate on the dedicated reject permission seeded by iam migration 000083")
}

// TestRejectMBHeadDeniedWithoutPermission is the 403 guard: holding the other MB
// head workflow permissions must not confer the right to reject.
func TestRejectMBHeadDeniedWithoutPermission(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := context.WithValue(context.Background(), AuthRolesKey, []string{"FINANCE_USER"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{
		"finance.mb.head.view", "finance.mb.head.submit", "finance.mb.head.approve",
	})

	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.MBHeadService/RejectMBHead"}
	_, err := interceptor(ctx, nil, info, financeNoopHandler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

// TestReturnMBHeadToDraftRequiresSubmitPermission pins the permission gating the
// REJECTED → DRAFT transition (user decision K-30 option A).
//
// The mapping deliberately REUSES finance.mb.head.submit rather than introducing a
// dedicated permission: the author allowed to submit an MB may pull their own MB back
// to DRAFT. Changing this to a new code would require an IAM migration + seed.
//
// getRequiredPermission returns "" for any unmapped method and the interceptor treats
// "" as "authenticated access is sufficient" (auth_interceptor.go:331-334), so dropping
// this entry would let ANY logged-in user reopen a rejected MB head.
func TestReturnMBHeadToDraftRequiresSubmitPermission(t *testing.T) {
	got := getRequiredPermission("/finance.v1.MBHeadService/ReturnMBHeadToDraft")

	assert.Equal(t, "finance.mb.head.submit", got,
		"ReturnMBHeadToDraft must gate on the existing submit permission (K-30 option A)")
}

// TestReturnMBHeadToDraftDeniedWithoutPermission is the 403 guard: holding the other MB
// head workflow permissions must not confer the right to reopen a rejected MB head.
// TestFinanceGetRequiredPermission_Shade pins the exact permission codes for
// all six ShadeService RPCs (iam migration 000089 seeds these codes).
func TestFinanceGetRequiredPermission_Shade(t *testing.T) {
	tests := []struct {
		method string
		want   string
	}{
		{"/finance.v1.ShadeService/CreateShade", "finance.master.shade.create"},
		{"/finance.v1.ShadeService/GetShade", "finance.master.shade.view"},
		{"/finance.v1.ShadeService/UpdateShade", "finance.master.shade.update"},
		{"/finance.v1.ShadeService/DeactivateShade", "finance.master.shade.delete"},
		{"/finance.v1.ShadeService/ListShades", "finance.master.shade.view"},
		{"/finance.v1.ShadeService/SyncShades", "finance.master.shade.sync"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			assert.Equal(t, tt.want, getRequiredPermission(tt.method))
		})
	}
}

// TestShadeServiceRPCsAllRequirePermission guards against fail-open, following
// the same pattern as TestMBDozingServiceRPCsAllRequirePermission: it enumerates
// RPCs from the GENERATED ShadeService descriptor rather than a hand-written
// list, so a new RPC added to ShadeService fails here until it is mapped.
func TestShadeServiceRPCsAllRequirePermission(t *testing.T) {
	desc := financev1.ShadeService_ServiceDesc
	require.Equal(t, "finance.v1.ShadeService", desc.ServiceName)
	require.NotEmpty(t, desc.Methods)

	for _, m := range desc.Methods {
		fullMethod := "/" + desc.ServiceName + "/" + m.MethodName
		t.Run(fullMethod, func(t *testing.T) {
			got := getRequiredPermission(fullMethod)
			assert.NotEmpty(t, got,
				"RPC %s has no entry in the permission map; unmapped methods fail OPEN to any authenticated user", fullMethod)
		})
	}
}

func TestReturnMBHeadToDraftDeniedWithoutPermission(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := context.WithValue(context.Background(), AuthRolesKey, []string{"FINANCE_USER"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{
		"finance.mb.head.view", "finance.mb.head.reject", "finance.mb.head.approve",
	})

	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.MBHeadService/ReturnMBHeadToDraft"}
	_, err := interceptor(ctx, nil, info, financeNoopHandler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}
