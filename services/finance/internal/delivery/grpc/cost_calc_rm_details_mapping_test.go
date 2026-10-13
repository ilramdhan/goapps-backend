package grpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

func TestRMDetailsToProto_DenierAndRatioSource(t *testing.T) {
	got := rmDetailsToProto([]costcalcdom.RMDetailSummary{
		{RMType: "GROUP", RefCode: "A", Ratio: 0.5, Denier: 40, RatioSource: "DENIER"},
		{RMType: "ITEM", RefCode: "B", Ratio: 1},
	})
	require.Len(t, got, 2)
	assert.InDelta(t, 40, got[0].Denier, 1e-9)
	assert.Equal(t, "DENIER", got[0].RatioSource)
	assert.Zero(t, got[1].Denier)
	assert.Empty(t, got[1].RatioSource)
}

func TestRMDetailSummary_JSONRoundTrip(t *testing.T) {
	var d []costcalcdom.RMDetailSummary
	require.NoError(t, json.Unmarshal([]byte(`[{"ref_code":"A","denier":40,"ratio_source":"DENIER"},{"ref_code":"B"}]`), &d))
	assert.InDelta(t, 40, d[0].Denier, 1e-9)
	assert.Equal(t, "DENIER", d[0].RatioSource)
	assert.Empty(t, d[1].RatioSource)
}
