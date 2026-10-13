package costcalc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

func TestResolveResultDisplay_CarriesDenierAndRatioSource(t *testing.T) {
	svc := &Service{loader: &sheetFakeLoader{products: map[int64]*costproductmaster.CostProductMaster{}}}
	detail := []RMCostDetail{
		{RMType: "GROUP", RefCode: "SPD0000006", Contribution: 10, Ratio: 0.5, Denier: 40, RatioSource: "DENIER"},
		{RMType: "GROUP", RefCode: "OTHER", Contribution: 5, Ratio: 0.5},
	}
	disp, err := svc.resolveResultDisplay(context.Background(), 1, detail)
	require.NoError(t, err)
	require.Len(t, disp.RMDetails, 2)
	assert.InDelta(t, 40, disp.RMDetails[0].Denier, 1e-9)
	assert.Equal(t, "DENIER", disp.RMDetails[0].RatioSource)
	// INPUT-mode line: zero / empty.
	assert.Zero(t, disp.RMDetails[1].Denier)
	assert.Empty(t, disp.RMDetails[1].RatioSource)
}
