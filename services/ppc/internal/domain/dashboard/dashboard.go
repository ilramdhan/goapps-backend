// Package dashboard holds the pure aggregation logic for the PPC read
// dashboards (daily performance + morning review). It computes flags, KPI
// rollups, and issue severity from already-gathered numbers; all I/O lives in
// the application and infrastructure layers.
package dashboard

import "time"

// Variance flag values for actual-vs-target comparison.
const (
	FlagOver     = "OVER"
	FlagUnder    = "UNDER"
	FlagOnTarget = "ON_TARGET"
)

// onTargetBandPct is the ± band (percent of target) treated as on-target.
const onTargetBandPct = 5.0

// Variance compares an actual quantity against its target and returns the flag
// plus the signed variance as a percentage of target. A zero target yields
// ON_TARGET at 0% when actual is also zero, else OVER (no meaningful ratio).
func Variance(target, actual float64) (flag string, pct float64) {
	if target == 0 {
		if actual == 0 {
			return FlagOnTarget, 0
		}
		return FlagOver, 0
	}
	pct = (actual - target) / target * 100
	switch {
	case pct > onTargetBandPct:
		return FlagOver, pct
	case pct < -onTargetBandPct:
		return FlagUnder, pct
	default:
		return FlagOnTarget, pct
	}
}

// MachineRow is one machine's actual-vs-plan line for the morning review.
type MachineRow struct {
	MachineID    int64
	MachineNo    string
	Area         string
	QtyTarget    float64
	QtyActual    float64
	IsChangeover bool
}

// Flag classifies the row's actual against its target.
func (r MachineRow) Flag() string {
	flag, _ := Variance(r.QtyTarget, r.QtyActual)
	return flag
}

// VariancePct returns the signed variance percentage of the row.
func (r MachineRow) VariancePct() float64 {
	_, pct := Variance(r.QtyTarget, r.QtyActual)
	return pct
}

// AreaAggregate carries the summed daily-performance components for one area
// (or all areas), both for the requested day (Today) and month-to-date (MTD),
// for a single Excluding/Including variant. Efficiency and waste percentages are
// derived from the sums (Σactual / Σtheoretical), never averaged.
type AreaAggregate struct {
	ActualToday float64
	TheoToday   float64
	WasteToday  float64
	ActualMTD   float64
	TheoMTD     float64
	WasteMTD    float64
	IdleToday   int
	IdleMTD     int
	OTToday     float64
	OTMTD       float64
}

// KPI is one dashboard KPI card with a Today and an MTD value.
type KPI struct {
	Key        string
	Label      string
	ValueToday float64
	ValueMTD   float64
	Unit       string
}

// KPI keys/labels/units.
const (
	unitKg  = "kg"
	unitPct = "%"
	unitCnt = "count"
	unitHr  = "hours"
)

// pct returns numerator/denominator*100, guarding a zero denominator.
func pct(num, den float64) float64 {
	if den == 0 {
		return 0
	}
	return num / den * 100
}

// BuildKPIs derives the standard daily-performance KPI cards from an area
// aggregate. Efficiency = Σactual/Σtheoretical(100%); waste % = Σwaste/Σactual.
func BuildKPIs(a AreaAggregate) []KPI {
	return []KPI{
		{Key: "total_production", Label: "Total Production", ValueToday: a.ActualToday, ValueMTD: a.ActualMTD, Unit: unitKg},
		{Key: "efficiency", Label: "Production Efficiency", ValueToday: pct(a.ActualToday, a.TheoToday), ValueMTD: pct(a.ActualMTD, a.TheoMTD), Unit: unitPct},
		{Key: "waste_pct", Label: "Waste %", ValueToday: pct(a.WasteToday, a.ActualToday), ValueMTD: pct(a.WasteMTD, a.ActualMTD), Unit: unitPct},
		{Key: "idle_positions", Label: "Idle Positions", ValueToday: float64(a.IdleToday), ValueMTD: float64(a.IdleMTD), Unit: unitCnt},
		{Key: "ot_hours", Label: "OT Hours", ValueToday: a.OTToday, ValueMTD: a.OTMTD, Unit: unitHr},
	}
}

// McEffCell is one machine×shift efficiency cell for the MC-EFF heatmap.
type McEffCell struct {
	MachineID int64
	MachineNo string
	Date      time.Time
	Shift     string
	EffPct    float64
}

// Issue severity values.
const (
	SeverityHigh   = "HIGH"
	SeverityMedium = "MEDIUM"
	SeverityLow    = "LOW"
)

// Issue types.
const (
	IssuePendingApproval = "PENDING_APPROVAL"
	IssueRMNearLimit     = "RM_NEAR_LIMIT"
)

// autoApproveWindow is the WO auto-approve delay (PRD: 24h). An unapproved WO
// whose age approaches this window is an open issue to act on.
const autoApproveWindow = 24 * time.Hour

// ApprovalSeverity grades how urgent a pending-approval WO is by how little time
// remains before its auto-approve window elapses. Elapsed/near windows are HIGH.
func ApprovalSeverity(updatedAt, now time.Time) (severity string, hoursLeft float64) {
	hoursLeft = autoApproveWindow.Hours() - now.Sub(updatedAt).Hours()
	switch {
	case hoursLeft <= 4:
		return SeverityHigh, hoursLeft
	case hoursLeft <= 12:
		return SeverityMedium, hoursLeft
	default:
		return SeverityLow, hoursLeft
	}
}

// PendingWO is a work order awaiting approval, used to build open-issue lines.
type PendingWO struct {
	WoID      int64
	WoNo      string
	Status    string
	UpdatedAt time.Time
}

// Issue is one open-issue line for the morning review.
type Issue struct {
	IssueType string
	RefID     int64
	Title     string
	Detail    string
	Severity  string
}

// Priority is one "must run today" work order for the morning review.
type Priority struct {
	WoID        int64
	WoNo        string
	ProductCode string
	MachineNo   string
	Deadline    time.Time
	QtyTarget   float64
}

// QuickStats are the morning-review headline counters.
type QuickStats struct {
	MachinesRunning    int32
	MachinesTotal      int32
	WOsPendingApproval int32
	UnmatchedSOCount   int32
}
