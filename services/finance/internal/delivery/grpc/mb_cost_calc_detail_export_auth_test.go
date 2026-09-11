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

const exportMBCostCalcDetailMethod = "/finance.v1.MBHeadService/ExportMBCostCalcDetail"

// TestExportMBCostCalcDetail_RequiresRecipeExportPermission pins the permission code
// guarding the 29-column calc dump.
//
// ⚠ WHY NOT finance.mb.head.view: the dump carries the RM composition lines AND every
// MB cost figure. A user who may merely view an MB head must not thereby obtain costing
// data, so this RPC requires the same narrow permission as the full recipe export. A
// regression re-pointing it at finance.mb.head.* would silently widen who can read cost.
func TestExportMBCostCalcDetail_RequiresRecipeExportPermission(t *testing.T) {
	got := getRequiredPermission(exportMBCostCalcDetailMethod)

	assert.Equal(t, "finance.mb.recipe.export", got)
	assert.NotContains(t, got, "finance.mb.head.",
		"the calc detail dump exposes composition + cost; it must not reuse an mb.head permission")
}

// TestExportMBCostCalcDetail_DeniedWithoutPermission is the 403 guard: a user holding
// every MB head permission but NOT finance.mb.recipe.export must be refused.
func TestExportMBCostCalcDetail_DeniedWithoutPermission(t *testing.T) {
	interceptor := PermissionInterceptor()

	ctx := context.WithValue(context.Background(), AuthRolesKey, []string{"FINANCE_VIEWER"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{
		"finance.mb.head.view", "finance.mb.head.create", "finance.mb.head.update",
	})

	info := &grpc.UnaryServerInfo{FullMethod: exportMBCostCalcDetailMethod}
	_, err := interceptor(ctx, nil, info, financeNoopHandler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code(),
		"holding mb.head permissions must NOT grant the cost-bearing calc detail dump")
}

// TestExportMBCostCalcDetail_NotConfiguredIsCleanError pins that an unwired reader
// yields a clean 500 rather than a nil-pointer panic — the same degradation
// ExportMBRecipeFull has when its reader is absent.
func TestExportMBCostCalcDetail_NotConfiguredIsCleanError(t *testing.T) {
	h, err := NewMBHeadHandler(nil, nil, nil)
	require.NoError(t, err)

	resp, err := h.ExportMBCostCalcDetail(context.Background(), &financev1.ExportMBCostCalcDetailRequest{})

	require.NoError(t, err, "an unconfigured export must report via BaseResponse, not a transport error")
	require.NotNil(t, resp.Base)
	assert.False(t, resp.Base.IsSuccess)
	assert.Equal(t, "500", resp.Base.StatusCode)
	assert.Empty(t, resp.FileContent)
}
