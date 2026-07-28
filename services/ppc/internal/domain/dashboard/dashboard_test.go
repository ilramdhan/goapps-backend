package dashboard_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/dashboard"
)

func TestVariance(t *testing.T) {
	tests := []struct {
		name       string
		target     float64
		actual     float64
		wantFlag   string
		wantPctApx float64
	}{
		{"over", 100, 110, dashboard.FlagOver, 10},
		{"under", 100, 80, dashboard.FlagUnder, -20},
		{"on_target_within_band", 100, 103, dashboard.FlagOnTarget, 3},
		{"on_target_exact", 100, 100, dashboard.FlagOnTarget, 0},
		{"upper_edge_on_target", 100, 105, dashboard.FlagOnTarget, 5},
		{"just_over_edge", 100, 106, dashboard.FlagOver, 6},
		{"zero_target_zero_actual", 0, 0, dashboard.FlagOnTarget, 0},
		{"zero_target_positive_actual", 0, 50, dashboard.FlagOver, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag, pct := dashboard.Variance(tt.target, tt.actual)
			assert.Equal(t, tt.wantFlag, flag)
			assert.InDelta(t, tt.wantPctApx, pct, 0.001)
		})
	}
}

func TestMachineRow_FlagAndVariance(t *testing.T) {
	row := dashboard.MachineRow{QtyTarget: 200, QtyActual: 180}
	assert.Equal(t, dashboard.FlagUnder, row.Flag())
	assert.InDelta(t, -10, row.VariancePct(), 0.001)
}

func TestBuildKPIs_RollupFromSums(t *testing.T) {
	agg := dashboard.AreaAggregate{
		ActualToday: 900, TheoToday: 1000, WasteToday: 45,
		ActualMTD: 9000, TheoMTD: 12000, WasteMTD: 600,
		IdleToday: 3, IdleMTD: 40,
		OTToday: 8, OTMTD: 120,
	}
	kpis := dashboard.BuildKPIs(agg)
	require.Len(t, kpis, 5)

	byKey := make(map[string]dashboard.KPI, len(kpis))
	for _, k := range kpis {
		byKey[k.Key] = k
	}

	// efficiency = Σactual / Σtheoretical * 100 (re-aggregated, not averaged)
	assert.InDelta(t, 90.0, byKey["efficiency"].ValueToday, 0.001)
	assert.InDelta(t, 75.0, byKey["efficiency"].ValueMTD, 0.001)
	// waste % = Σwaste / Σactual * 100
	assert.InDelta(t, 5.0, byKey["waste_pct"].ValueToday, 0.001)
	assert.InDelta(t, 100.0*600/9000, byKey["waste_pct"].ValueMTD, 0.001)
	// pass-through counters
	assert.InDelta(t, 900, byKey["total_production"].ValueToday, 0.001)
	assert.InDelta(t, 3, byKey["idle_positions"].ValueToday, 0.001)
	assert.InDelta(t, 8, byKey["ot_hours"].ValueToday, 0.001)
}

func TestBuildKPIs_ZeroDenominatorSafe(t *testing.T) {
	kpis := dashboard.BuildKPIs(dashboard.AreaAggregate{})
	for _, k := range kpis {
		assert.False(t, math.IsNaN(k.ValueToday), "%s today NaN", k.Key)
		assert.False(t, math.IsNaN(k.ValueMTD), "%s mtd NaN", k.Key)
		assert.Zero(t, k.ValueToday)
	}
}

func TestApprovalSeverity(t *testing.T) {
	now := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		ageHours     float64
		wantSeverity string
	}{
		{"fresh", 1, dashboard.SeverityLow},         // ~23h left
		{"half_way", 13, dashboard.SeverityMedium},  // ~11h left
		{"near_window", 21, dashboard.SeverityHigh}, // ~3h left
		{"past_window", 30, dashboard.SeverityHigh}, // negative left
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated := now.Add(-time.Duration(tt.ageHours * float64(time.Hour)))
			sev, _ := dashboard.ApprovalSeverity(updated, now)
			assert.Equal(t, tt.wantSeverity, sev)
		})
	}
}
