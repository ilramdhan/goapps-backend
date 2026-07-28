package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

const testInternalSecret = "test-internal-secret"

func newTestAuthInterceptor() grpc.UnaryServerInterceptor {
	return AuthInterceptor(&config.JWTConfig{
		AccessTokenSecret: "unit-test-access-secret",
		ServiceSecret:     testInternalSecret,
	}, nil, nil)
}

// probeHandler records whether it was reached and echoes the injected roles.
func probeHandler() (grpc.UnaryHandler, *bool) {
	reached := false
	h := func(ctx context.Context, _ any) (any, error) {
		reached = true
		return GetRolesFromCtx(ctx), nil
	}
	return h, &reached
}

func TestAuthInterceptor_LookupWithInternalToken_Accepted(t *testing.T) {
	interceptor := newTestAuthInterceptor()
	handler, reached := probeHandler()

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-internal-token", testInternalSecret))
	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.CostMasterLookupService/GetCostProductMasterForPPC"}

	out, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	assert.True(t, *reached, "handler should be invoked for a valid internal-token lookup call")
	roles, ok := out.([]string)
	require.True(t, ok)
	assert.Contains(t, roles, "SUPER_ADMIN", "internal call should get synthetic SUPER_ADMIN identity")
}

func TestAuthInterceptor_LookupWithServiceSecret_Accepted(t *testing.T) {
	interceptor := newTestAuthInterceptor()
	handler, reached := probeHandler()

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-service-secret", testInternalSecret))
	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.CostMasterLookupService/ListProductGradesForPPC"}

	_, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	assert.True(t, *reached, "x-service-secret header must also be accepted for lookup calls")
}

func TestAuthInterceptor_LookupWithoutToken_Rejected(t *testing.T) {
	interceptor := newTestAuthInterceptor()
	handler, reached := probeHandler()

	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.CostMasterLookupService/GetCostProductMasterForPPC"}
	_, err := interceptor(context.Background(), nil, info, handler)

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.False(t, *reached, "handler must not be reached without a valid internal secret")
}

func TestAuthInterceptor_LookupWithWrongToken_Rejected(t *testing.T) {
	interceptor := newTestAuthInterceptor()
	handler, reached := probeHandler()

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-internal-token", "wrong-secret"))
	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.CostMasterLookupService/GetCostProductMasterForPPC"}

	_, err := interceptor(ctx, nil, info, handler)
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.False(t, *reached)
}

// A user-facing RPC must NOT be reachable via the internal token — it still
// requires a valid JWT. This proves the lookup bypass does not weaken user auth.
func TestAuthInterceptor_UserFacingRPC_InternalTokenRejected(t *testing.T) {
	interceptor := newTestAuthInterceptor()
	handler, reached := probeHandler()

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-internal-token", testInternalSecret))
	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.UOMService/ListUOMs"}

	_, err := interceptor(ctx, nil, info, handler)
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err), "internal token must not authenticate a user-facing RPC")
	assert.False(t, *reached)
}
