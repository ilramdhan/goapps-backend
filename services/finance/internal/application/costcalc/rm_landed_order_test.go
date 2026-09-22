package costcalc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// ParseRMLandedOrder — pure unit tests, no DB required.
// =============================================================================

func TestParseRMLandedOrder_ValidTwoSectionFormat(t *testing.T) {
	t.Parallel()
	cfg := ParseRMLandedOrder("ACTUAL:CL,SL,FL;FORECAST:SP,PP,FP;SELLING:SP,PP,FP")
	require.Len(t, cfg, 3)
	assert.Equal(t, []string{"CL", "SL", "FL"}, cfg["ACTUAL"])
	assert.Equal(t, []string{"SP", "PP", "FP"}, cfg["FORECAST"])
	assert.Equal(t, []string{"SP", "PP", "FP"}, cfg["SELLING"])
}

func TestParseRMLandedOrder_MissingSectionIndependentFallback(t *testing.T) {
	t.Parallel()
	// Only ACTUAL configured; FORECAST/SELLING simply absent from the map so
	// callers (rmLandedOrderForCalcType) resolve those independently to their
	// own hardcoded defaults.
	cfg := ParseRMLandedOrder("ACTUAL:SL,CL,FL")
	require.Len(t, cfg, 1)
	assert.Equal(t, []string{"SL", "CL", "FL"}, cfg["ACTUAL"])
	_, ok := cfg["FORECAST"]
	assert.False(t, ok)
	_, ok = cfg["SELLING"]
	assert.False(t, ok)
}

func TestParseRMLandedOrder_Empty(t *testing.T) {
	t.Parallel()
	tests := []string{"", "   ", "\t\n"}
	for _, in := range tests {
		cfg := ParseRMLandedOrder(in)
		assert.Empty(t, cfg, "input %q must yield no sections", in)
	}
}

func TestParseRMLandedOrder_GarbageIgnoredPerSection(t *testing.T) {
	t.Parallel()
	// Garbage/malformed sections are skipped individually; well-formed
	// sections in the same expression still parse.
	cfg := ParseRMLandedOrder("NOT_A_SECTION;ACTUAL:CL,SL,FL;GARBAGE:XX,YY")
	require.Len(t, cfg, 1)
	assert.Equal(t, []string{"CL", "SL", "FL"}, cfg["ACTUAL"])
}

func TestParseRMLandedOrder_InvalidTokenForSectionDropsThatSection(t *testing.T) {
	t.Parallel()
	// FORECAST section uses an ACTUAL-only token (CL) — invalid for FORECAST,
	// so that section is dropped, but ACTUAL still parses fine.
	cfg := ParseRMLandedOrder("ACTUAL:CL,SL,FL;FORECAST:CL,PP,FP")
	require.Len(t, cfg, 1)
	assert.Equal(t, []string{"CL", "SL", "FL"}, cfg["ACTUAL"])
	_, ok := cfg["FORECAST"]
	assert.False(t, ok)
}

func TestParseRMLandedOrder_UnknownCalcTypeKeyIgnored(t *testing.T) {
	t.Parallel()
	cfg := ParseRMLandedOrder("BOGUS:CL,SL,FL;ACTUAL:CL,SL,FL")
	require.Len(t, cfg, 1)
	assert.Equal(t, []string{"CL", "SL", "FL"}, cfg["ACTUAL"])
}

func TestParseRMLandedOrder_DuplicateTokenDropsSection(t *testing.T) {
	t.Parallel()
	cfg := ParseRMLandedOrder("ACTUAL:CL,SL,CL")
	assert.Empty(t, cfg)
}

func TestParseRMLandedOrder_MixedCaseAndWhitespace(t *testing.T) {
	t.Parallel()
	cfg := ParseRMLandedOrder("  actual : sl , cl , fl  ; forecast:pp,sp,fp ")
	require.Len(t, cfg, 2)
	assert.Equal(t, []string{"SL", "CL", "FL"}, cfg["ACTUAL"])
	assert.Equal(t, []string{"PP", "SP", "FP"}, cfg["FORECAST"])
}

func TestParseRMLandedOrder_PartialOrderValid(t *testing.T) {
	t.Parallel()
	// Fewer than 3 tokens is valid, mirroring ParseRMRateOrder's partial-order
	// allowance.
	cfg := ParseRMLandedOrder("ACTUAL:SL,CL")
	require.Len(t, cfg, 1)
	assert.Equal(t, []string{"SL", "CL"}, cfg["ACTUAL"])
}

// =============================================================================
// rmLandedOrderForCalcType — resolves the effective order, with fallback.
// =============================================================================

func TestRMLandedOrderForCalcType_UsesConfiguredActual(t *testing.T) {
	t.Parallel()
	cfg := map[string][]string{"ACTUAL": {"FL", "SL", "CL"}}
	order := rmLandedOrderForCalcType(cfg, "ACTUAL")
	assert.Equal(t, []string{"FL", "SL", "CL"}, order)
}

func TestRMLandedOrderForCalcType_MissingActualFallsBackToDefault(t *testing.T) {
	t.Parallel()
	order := rmLandedOrderForCalcType(map[string][]string{}, "ACTUAL")
	assert.Equal(t, DefaultRMLandedOrderActual, order)
}

func TestRMLandedOrderForCalcType_MissingForecastFallsBackToDefault(t *testing.T) {
	t.Parallel()
	order := rmLandedOrderForCalcType(map[string][]string{}, "FORECAST")
	assert.Equal(t, DefaultRMLandedOrderForecast, order)
}

func TestRMLandedOrderForCalcType_SellingFallsBackToForecastDefault(t *testing.T) {
	t.Parallel()
	// No SELLING entry configured, and no ACTUAL-style default exists for
	// SELLING — it must resolve to the FORECAST default, per the deliberate
	// placeholder documented on DefaultRMLandedOrderForecast / migration 000519.
	order := rmLandedOrderForCalcType(map[string][]string{}, "SELLING")
	assert.Equal(t, DefaultRMLandedOrderForecast, order)
}

func TestRMLandedOrderForCalcType_UsesConfiguredSellingWhenPresent(t *testing.T) {
	t.Parallel()
	cfg := map[string][]string{"SELLING": {"FP", "PP", "SP"}}
	order := rmLandedOrderForCalcType(cfg, "SELLING")
	assert.Equal(t, []string{"FP", "PP", "SP"}, order)
}

func TestRMLandedOrderForCalcType_UnrecognizedCalcTypeFallsBackToForecastDefault(t *testing.T) {
	t.Parallel()
	order := rmLandedOrderForCalcType(map[string][]string{}, "BOGUS")
	assert.Equal(t, DefaultRMLandedOrderForecast, order)
}

// =============================================================================
// rmLandedCandidates — maps an order onto RMCostRates.
// =============================================================================

func TestRMLandedCandidates_ActualOrderRespected(t *testing.T) {
	t.Parallel()
	rates := RMCostRates{ClRate: 1, SlRate: 2, FlRate: 3}
	got := rmLandedCandidates([]string{"CL", "SL", "FL"}, rates)
	require.Len(t, got, 3)
	assert.Equal(t, "CL", got[0].Label)
	assert.Equal(t, "SL", got[1].Label)
	assert.Equal(t, "FL", got[2].Label)
	assert.InDelta(t, 1.0, got[0].Value, 1e-9)
	assert.InDelta(t, 2.0, got[1].Value, 1e-9)
	assert.InDelta(t, 3.0, got[2].Value, 1e-9)
}

func TestRMLandedCandidates_ForecastOrderRespected(t *testing.T) {
	t.Parallel()
	rates := RMCostRates{SpRate: 10, PpRate: 20, FpRate: 30}
	got := rmLandedCandidates([]string{"SP", "PP", "FP"}, rates)
	require.Len(t, got, 3)
	assert.Equal(t, "SP", got[0].Label)
	assert.Equal(t, "PP", got[1].Label)
	assert.Equal(t, "FP", got[2].Label)
	assert.InDelta(t, 10.0, got[0].Value, 1e-9)
	assert.InDelta(t, 20.0, got[1].Value, 1e-9)
	assert.InDelta(t, 30.0, got[2].Value, 1e-9)
}
