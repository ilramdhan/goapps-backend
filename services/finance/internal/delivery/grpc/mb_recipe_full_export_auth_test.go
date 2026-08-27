package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

const exportMBRecipeFullMethod = "/finance.v1.MBHeadService/ExportMBRecipeFull"

// TestExportMBRecipeFull_RequiresRecipeExportPermission pins the exact permission
// code guarding the full recipe export.
//
// ⚠ WHY NOT finance.mb.head.view: the full export denormalizes the recipe
// COMPOSITION and the COST block into one sheet. A user who may merely view an MB
// head must not thereby obtain costing data, so this RPC deliberately requires a
// separate, narrower permission. A regression that re-points it at
// finance.mb.head.* would silently widen who can read cost data.
func TestExportMBRecipeFull_RequiresRecipeExportPermission(t *testing.T) {
	got := getRequiredPermission(exportMBRecipeFullMethod)

	assert.Equal(t, "finance.mb.recipe.export", got)
	assert.NotContains(t, got, "finance.mb.head.",
		"the full recipe export exposes composition + cost; it must not reuse an mb.head permission")
}

// TestExportMBRecipeFull_DeniedWithoutPermission is the 403 guard: a user holding
// every MB head permission but NOT finance.mb.recipe.export must be refused.
func TestExportMBRecipeFull_DeniedWithoutPermission(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := context.WithValue(context.Background(), AuthRolesKey, []string{"FINANCE_VIEWER"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{
		"finance.mb.head.view", "finance.mb.head.create", "finance.mb.head.update",
	})

	info := &grpc.UnaryServerInfo{FullMethod: exportMBRecipeFullMethod}
	_, err := interceptor(ctx, nil, info, financeNoopHandler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code(),
		"holding mb.head permissions must NOT grant the cost-bearing full recipe export")
}

// TestExportMBRecipeFull_DeniedWithNoPermissions covers the bare authenticated
// user: authentication alone must not reach a mapped RPC.
func TestExportMBRecipeFull_DeniedWithNoPermissions(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := context.WithValue(context.Background(), AuthRolesKey, []string{"USER"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{})

	info := &grpc.UnaryServerInfo{FullMethod: exportMBRecipeFullMethod}
	_, err := interceptor(ctx, nil, info, financeNoopHandler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

// TestExportMBRecipeFull_AllowedWithPermission is the positive counterpart, so a
// map entry that is merely misspelled (denying everyone) also fails loudly.
func TestExportMBRecipeFull_AllowedWithPermission(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := context.WithValue(context.Background(), AuthRolesKey, []string{"FINANCE_ADMIN"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{"finance.mb.recipe.export"})

	info := &grpc.UnaryServerInfo{FullMethod: exportMBRecipeFullMethod}
	resp, err := interceptor(ctx, nil, info, financeNoopHandler)

	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

// TestMBHeadServiceRPCsAllRequirePermission guards against fail-open across the
// whole service, enumerating RPCs from the GENERATED descriptor rather than a
// hand-written list. getRequiredPermission returns "" for any unmapped method and
// the interceptor treats "" as "authenticated access is sufficient", so an MB head
// RPC forgotten in the map would be reachable by every logged-in user.
//
// NOTE (K-34 c): this per-service guard is now SUPERSET by
// permission_coverage_test.go, which applies the same check to all 50
// registered services. Kept deliberately: it carries the original intent and
// history for this service, and a focused failure here names the service
// directly. Do not delete.
func TestMBHeadServiceRPCsAllRequirePermission(t *testing.T) {
	desc := financev1.MBHeadService_ServiceDesc
	require.Equal(t, "finance.v1.MBHeadService", desc.ServiceName)
	require.NotEmpty(t, desc.Methods)

	for _, m := range desc.Methods {
		fullMethod := "/" + desc.ServiceName + "/" + m.MethodName
		t.Run(m.MethodName, func(t *testing.T) {
			assert.NotEmpty(t, getRequiredPermission(fullMethod),
				"RPC %s has no entry in the permission map; unmapped methods fail OPEN to any authenticated user",
				fullMethod)
		})
	}
}
