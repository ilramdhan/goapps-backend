package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

// TestGetLookupFillValues_RMGroupOil_ReturnsEmptySuccess asserts oil-cost-rm-group
// D14: selecting an RM_GROUP_OIL option succeeds with no fills (and needs no repos).
func TestGetLookupFillValues_RMGroupOil_ReturnsEmptySuccess(t *testing.T) {
	h := &YarnLookupFillHandler{}
	resp, err := h.GetLookupFillValues(context.Background(), &financev1.GetLookupFillValuesRequest{
		LookupMasterCode: "RM_GROUP_OIL",
		SelectedKey:      "202006101",
		SourceParamCode:  "OIL_NAME",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.GetBase())
	assert.True(t, resp.GetBase().GetIsSuccess())
	assert.Equal(t, "200", resp.GetBase().GetStatusCode())
	assert.Empty(t, resp.GetNumericFills())
	assert.Empty(t, resp.GetTextFills())
	assert.Equal(t, "202006101", resp.GetDisplayLabel())
}
