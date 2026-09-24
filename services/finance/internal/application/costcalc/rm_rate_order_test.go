package costcalc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// ParseRMRateOrder — pure unit tests, no DB required.
// =============================================================================

func TestParseRMRateOrder_ValidOrder(t *testing.T) {
	t.Parallel()
	order, ok := ParseRMRateOrder("CR,SR,PR")
	require.True(t, ok)
	assert.Equal(t, []string{"CR", "SR", "PR"}, order)
}

func TestParseRMRateOrder_CustomOrder(t *testing.T) {
	t.Parallel()
	order, ok := ParseRMRateOrder("PR,SR,CR")
	require.True(t, ok)
	assert.Equal(t, []string{"PR", "SR", "CR"}, order)
}

func TestParseRMRateOrder_PartialOrder(t *testing.T) {
	t.Parallel()
	// Fewer than 3 tokens is valid: an operator may deliberately want the
	// cascade to only ever consider CR then SR, never falling to PR.
	order, ok := ParseRMRateOrder("SR,CR")
	require.True(t, ok)
	assert.Equal(t, []string{"SR", "CR"}, order)
}

func TestParseRMRateOrder_MixedCaseAndWhitespace(t *testing.T) {
	t.Parallel()
	order, ok := ParseRMRateOrder("  pr , Sr ,cr  ")
	require.True(t, ok)
	assert.Equal(t, []string{"PR", "SR", "CR"}, order)
}

func TestParseRMRateOrder_Empty(t *testing.T) {
	t.Parallel()
	tests := []string{"", "   ", "\t\n"}
	for _, in := range tests {
		order, ok := ParseRMRateOrder(in)
		assert.False(t, ok, "input %q must be rejected", in)
		assert.Nil(t, order)
	}
}

func TestParseRMRateOrder_GarbageToken(t *testing.T) {
	t.Parallel()
	tests := []string{"CR,SR,XX", "NOT_A_TOKEN", "CR;SR;PR", "CR SR PR"}
	for _, in := range tests {
		order, ok := ParseRMRateOrder(in)
		assert.False(t, ok, "input %q must be rejected", in)
		assert.Nil(t, order)
	}
}

func TestParseRMRateOrder_DuplicateToken(t *testing.T) {
	t.Parallel()
	order, ok := ParseRMRateOrder("CR,SR,CR")
	assert.False(t, ok)
	assert.Nil(t, order)
}

func TestParseRMRateOrder_TrailingComma(t *testing.T) {
	t.Parallel()
	// A trailing comma produces an empty final token, which is invalid.
	order, ok := ParseRMRateOrder("CR,SR,")
	assert.False(t, ok)
	assert.Nil(t, order)
}

// =============================================================================
// rmRateCandidates — maps a parsed order (or the default) onto RMCostRates.
// =============================================================================

func TestRMRateCandidates_EmptyOrderFallsBackToDefault(t *testing.T) {
	t.Parallel()
	rates := RMCostRates{CrRate: 1, SrRate: 2, PrRate: 3}
	got := rmRateCandidates(nil, rates)
	want := []struct {
		value float64
		label string
	}{{1, "CR"}, {2, "SR"}, {3, "PR"}}
	require.Len(t, got, len(want))
	for i, w := range want {
		assert.Equal(t, w.value, got[i].Value)
		assert.Equal(t, w.label, got[i].Label)
	}
}

func TestRMRateCandidates_CustomOrderRespected(t *testing.T) {
	t.Parallel()
	rates := RMCostRates{CrRate: 1, SrRate: 2, PrRate: 3}
	got := rmRateCandidates([]string{"PR", "SR", "CR"}, rates)
	require.Len(t, got, 3)
	assert.Equal(t, "PR", got[0].Label)
	assert.Equal(t, "SR", got[1].Label)
	assert.Equal(t, "CR", got[2].Label)
	assert.InDelta(t, 3.0, got[0].Value, 1e-9)
	assert.InDelta(t, 2.0, got[1].Value, 1e-9)
	assert.InDelta(t, 1.0, got[2].Value, 1e-9)
}

// TestResolveOilRate_HonorsCustomRMRateOrder proves OIL_RATE shares the
// GROUP-RM cascade config (F_YARN_RM_RATE expression, 000518) rather than a
// hardcoded CR->SR->PR: with PR,SR,CR the PR value wins even though CR and SR
// are non-zero, and the default order still picks CR.
func TestResolveOilRate_HonorsCustomRMRateOrder(t *testing.T) {
	t.Parallel()
	base := ComputeInput{
		Oil:     &OilInput{Class: OilClassPTY, TypeCode: "PTY", GroupCode: "G1", DefaultGroup: "G1", Allowed: map[string]bool{"G1": true}},
		RMCosts: map[string]RMCostRates{"G1|": {CrRate: 1, SrRate: 2, PrRate: 3}},
	}

	custom := base
	custom.RMRateOrder = []string{"PR", "SR", "CR"}
	rate, label, applied, err := resolveOilRate(custom)
	require.NoError(t, err)
	require.True(t, applied)
	assert.InDelta(t, 3.0, rate, 1e-9)
	assert.Equal(t, "PR", label)

	rate, label, applied, err = resolveOilRate(base)
	require.NoError(t, err)
	require.True(t, applied)
	assert.InDelta(t, 1.0, rate, 1e-9)
	assert.Equal(t, "CR", label)
}
