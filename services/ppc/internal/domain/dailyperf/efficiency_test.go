package dailyperf_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
)

func TestTheoretical_HandComputed(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	// 48 positions × 480 min × 3000 mpm × 150 denier / 9,000,000 = 1152.0 kg.
	got := calc.Theoretical(dailyperf.TheoreticalInput{
		Positions:      48,
		RunningMinutes: 480,
		Speed:          3000,
		Denier:         150,
	})
	assert.InDelta(t, 1152.0, got, 1e-9)
}

func TestProductionEff(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	// actual 1000 over theoretical 1152 = 86.805...%
	got := calc.ProductionEff(1000, 1152.0)
	assert.InDelta(t, 86.80555555, got, 1e-6)
}

func TestProductionEff_DivideByZeroGuard(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	assert.Equal(t, 0.0, calc.ProductionEff(1000, 0))
	assert.Equal(t, 0.0, calc.ProductionEff(1000, -5))
}

func TestRunningEff(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	// Running variant: positions ran only 400 min → theoretical 960 kg.
	theoRng := calc.Theoretical(dailyperf.TheoreticalInput{
		Positions:      48,
		RunningMinutes: 400,
		Speed:          3000,
		Denier:         150,
	})
	assert.InDelta(t, 960.0, theoRng, 1e-9)
	got := calc.RunningEff(900, theoRng)
	assert.InDelta(t, 93.75, got, 1e-9)
}

func TestRunningEff_DivideByZeroGuard(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	assert.Equal(t, 0.0, calc.RunningEff(900, 0))
}

func TestWastePct(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	// 50 waste over gross 1050 → 4.7619%.
	assert.InDelta(t, 4.76190476, calc.WastePct(1000, 50), 1e-6)
	assert.Equal(t, 0.0, calc.WastePct(0, 0))
}

func TestBreaksPerTon(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	// 6 breaks over 1.152 tons → 5.208333...
	assert.InDelta(t, 5.20833333, calc.BreaksPerTon(6, 1152.0), 1e-6)
}

func TestBreaksPerTon_DivideByZeroGuard(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	assert.Equal(t, 0.0, calc.BreaksPerTon(6, 0))
}

func TestLossKg(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	// 80 min downtime on 48 pos @ 3000 mpm, 150 den → 192 kg lost.
	got := calc.LossKg(80, 48, 3000, 150)
	assert.InDelta(t, 192.0, got, 1e-9)
}

func TestMTDEff_ReaggregatesNotAverages(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	// Component A: 900/1000 = 90%. Component B: 100/1000 = 10%.
	// Averaging the percentages gives 50%. Correct re-aggregation is
	// (900+100)/(1000+1000) = 50%... choose asymmetric weights to prove it differs.
	comps := []dailyperf.EffComponent{
		{Actual: 900, Theoretical: 1000}, // 90%
		{Actual: 100, Theoretical: 5000}, // 2%
	}
	mtd := calc.MTDEff(comps)
	// Re-aggregation: (900+100)/(1000+5000) = 1000/6000 = 16.6667%.
	assert.InDelta(t, 16.66666667, mtd, 1e-6)
	// Averaging percentages would be (90+2)/2 = 46% — prove MTD is NOT that.
	assert.Greater(t, 46.0-mtd, 1.0)
}

func TestMTDEff_ZeroTheoreticalGuard(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	assert.Equal(t, 0.0, calc.MTDEff(nil))
	assert.Equal(t, 0.0, calc.MTDEff([]dailyperf.EffComponent{{Actual: 5, Theoretical: 0}}))
}

func TestPlantEff_DoffedBasis(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	// SPG plant eff uses DOFFED qty (GROSS_BOBBINS × weight), NOT transferred.
	// 100% nominal theoretical = 1152 kg. Doffed 1200 kg → 104.17% (>100% allowed).
	theo100 := calc.Theoretical(dailyperf.TheoreticalInput{Positions: 48, RunningMinutes: 480, Speed: 3000, Denier: 150})
	got := calc.PlantEff(1200, theo100)
	assert.InDelta(t, 104.16666667, got, 1e-6)
	// Guards.
	assert.Equal(t, 0.0, calc.PlantEff(1200, 0))
}

func TestPlantEff_DiffersFromTransferredBasis(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	theo100 := 1152.0
	// Same shift, two bases: doffed 1200 (efficiency) vs transferred 1080 (fulfillment).
	// SPG efficiency must use the doffed number, which is the larger, distinct result.
	doffed := calc.PlantEff(1200, theo100)
	transferred := calc.PlantEff(1080, theo100)
	assert.Greater(t, doffed, transferred)
	assert.InDelta(t, 93.75, transferred, 1e-9)
}

func TestYield(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	// waste% = 50/1050 = 4.7619%; yield = 95.2381%.
	assert.InDelta(t, 95.23809524, calc.Yield(1000, 50), 1e-6)
	assert.Equal(t, 100.0, calc.Yield(1000, 0))
}

func TestChangeoverPct(t *testing.T) {
	calc := dailyperf.NewEfficiencyCalculator()
	// 40 doffs, 2 CO failures → 95%.
	assert.InDelta(t, 95.0, calc.ChangeoverPct(40, 2), 1e-9)
	assert.Equal(t, 0.0, calc.ChangeoverPct(0, 0))
}
