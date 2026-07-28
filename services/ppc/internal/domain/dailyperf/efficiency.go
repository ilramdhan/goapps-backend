// Package dailyperf provides the daily-performance domain: shift entries, area
// shift logs, downtime/waste actuals, shift-log notes, and the efficiency engine
// that turns recorded production into efficiency snapshots (PRD v1.2).
package dailyperf

// Efficiency-engine constants (PRD v1.2).
const (
	// FullShiftMinutes is the theoretical length of one production shift, used as
	// the 100% (unadjusted) running time in the theoretical-yield formula.
	FullShiftMinutes = 480

	// theoreticalDivisor converts positions × minutes × speed(mpm) × denier into
	// kilograms: denier is grams per 9000 m, and speed is meters per minute, so
	// dividing the product by 9,000,000 yields kilograms.
	theoreticalDivisor = 9_000_000.0

	// kgPerTon converts kilograms to metric tons for the breaks-per-ton metric.
	kgPerTon = 1000.0

	// pctScale scales a ratio to a percentage.
	pctScale = 100.0
)

// TheoreticalInput carries the pinned well-known parameters and shift geometry
// used to compute a theoretical (100%-yield) production quantity in kilograms.
type TheoreticalInput struct {
	Positions      float64 // number of running positions
	RunningMinutes float64 // minutes the positions ran (480 for the 100% variant)
	Speed          float64 // yarn speed in meters per minute
	Denier         float64 // linear density in grams per 9000 m
}

// EffComponent is one additive efficiency component (actual vs theoretical) used
// to re-aggregate rollups. Month-to-date and day rollups must sum the components
// and divide, never average the per-row percentages.
type EffComponent struct {
	Actual      float64
	Theoretical float64
}

// EfficiencyCalculator performs the pure efficiency arithmetic (PRD v1.2). It
// holds no state and never touches I/O, so it is trivially testable.
type EfficiencyCalculator struct{}

// NewEfficiencyCalculator returns a stateless efficiency calculator.
func NewEfficiencyCalculator() EfficiencyCalculator { return EfficiencyCalculator{} }

// Theoretical returns the theoretical production in kilograms for the input:
// positions × running_minutes × speed × denier / 9,000,000.
func (EfficiencyCalculator) Theoretical(in TheoreticalInput) float64 {
	return in.Positions * in.RunningMinutes * in.Speed * in.Denier / theoreticalDivisor
}

// ProductionEff returns the production efficiency percentage (actual over the
// 100% theoretical). A non-positive theoretical yields 0 (guarded).
func (EfficiencyCalculator) ProductionEff(qtyActual, theoretical100 float64) float64 {
	if theoretical100 <= 0 {
		return 0
	}
	return qtyActual / theoretical100 * pctScale
}

// RunningEff returns the running efficiency percentage (actual over the
// running-adjusted theoretical). A non-positive theoretical yields 0 (guarded).
func (EfficiencyCalculator) RunningEff(qtyActual, theoreticalRng float64) float64 {
	if theoreticalRng <= 0 {
		return 0
	}
	return qtyActual / theoreticalRng * pctScale
}

// PlantEff returns the SPG plant efficiency percentage: actual DOFFED output
// (GROSS_BOBBINS × weight, carried as qty_actual for SPG) over the 100% nominal
// theoretical. It can exceed 100% because bobbin weight/denier varies. A
// non-positive theoretical yields 0 (guarded). Same arithmetic as ProductionEff;
// the distinct name pins the doffed basis at the call site.
func (c EfficiencyCalculator) PlantEff(qtyDoffed, theoretical100 float64) float64 {
	return c.ProductionEff(qtyDoffed, theoretical100)
}

// Yield returns the yield percentage: the non-waste fraction of gross output,
// i.e. 100 − waste%. Floored at 0.
func (c EfficiencyCalculator) Yield(qtyActual, qtyWaste float64) float64 {
	v := pctScale - c.WastePct(qtyActual, qtyWaste)
	if v < 0 {
		return 0
	}
	return v
}

// ChangeoverPct returns the changeover success percentage for a shift:
// (total_doff − co_failure) / total_doff × 100. A non-positive total yields 0.
func (EfficiencyCalculator) ChangeoverPct(totalDoff, coFailure int) float64 {
	if totalDoff <= 0 {
		return 0
	}
	return float64(totalDoff-coFailure) / float64(totalDoff) * pctScale
}

// WastePct returns waste as a percentage of gross output (actual + waste). A
// non-positive denominator yields 0 (guarded).
func (EfficiencyCalculator) WastePct(qtyActual, qtyWaste float64) float64 {
	gross := qtyActual + qtyWaste
	if gross <= 0 {
		return 0
	}
	return qtyWaste / gross * pctScale
}

// BreaksPerTon returns the number of breaks per metric ton of actual output. A
// non-positive quantity yields 0 (guarded).
func (EfficiencyCalculator) BreaksPerTon(breaks int, qtyActualKg float64) float64 {
	if qtyActualKg <= 0 {
		return 0
	}
	return float64(breaks) / (qtyActualKg / kgPerTon)
}

// LossKg returns the auto-computed lost kilograms for a downtime of durationMin
// minutes on the given positions/speed/denier: it is the theoretical output that
// would have been produced during the downtime (rate-per-minute × duration).
func (c EfficiencyCalculator) LossKg(durationMin int, positions, speed, denier float64) float64 {
	return c.Theoretical(TheoreticalInput{
		Positions:      positions,
		RunningMinutes: float64(durationMin),
		Speed:          speed,
		Denier:         denier,
	})
}

// MTDEff re-aggregates efficiency across components by summing actual and
// theoretical quantities and dividing (Σactual / Σtheoretical × 100), which is
// the correct rollup and differs from averaging per-component percentages.
func (EfficiencyCalculator) MTDEff(components []EffComponent) float64 {
	var sumActual, sumTheoretical float64
	for _, c := range components {
		sumActual += c.Actual
		sumTheoretical += c.Theoretical
	}
	if sumTheoretical <= 0 {
		return 0
	}
	return sumActual / sumTheoretical * pctScale
}
