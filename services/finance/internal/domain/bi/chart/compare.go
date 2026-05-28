package chart

import (
	"time"
)

// CompareMode is the canonical compare-mode key stored in bi_dashboard.compare_modes.
type CompareMode string

// All supported compare modes.
const (
	CompareNone CompareMode = "none"
	CompareMoM  CompareMode = "MoM"
	CompareQoQ  CompareMode = "QoQ"
	CompareYoY  CompareMode = "YoY"
	CompareYTD  CompareMode = "YTD"
	CompareR12  CompareMode = "R12"
)

// PeriodGrain mirrors bi_fact_metric.periode_grain.
type PeriodGrain string

// All supported period grains.
const (
	GrainDaily     PeriodGrain = "DAILY"
	GrainMonthly   PeriodGrain = "MONTHLY"
	GrainQuarterly PeriodGrain = "QUARTERLY"
	GrainYearly    PeriodGrain = "YEARLY"
)

// IsValidCompareMode reports whether the given string is a known compare mode.
func IsValidCompareMode(s string) bool {
	switch CompareMode(s) {
	case CompareNone, CompareMoM, CompareQoQ, CompareYoY, CompareYTD, CompareR12:
		return true
	}
	return false
}

// IsValidPeriodGrain reports whether the given string is a known period grain.
func IsValidPeriodGrain(s string) bool {
	switch PeriodGrain(s) {
	case GrainDaily, GrainMonthly, GrainQuarterly, GrainYearly:
		return true
	}
	return false
}

// ShiftPeriod returns t shifted backward by one comparison step, given the dashboard's grain.
//
// The shift is:
//   - MoM → 1 period unit (1 day for DAILY, 1 month for MONTHLY, 1 quarter for QUARTERLY, 1 year for YEARLY)
//   - QoQ → 3 months (only meaningful for MONTHLY/QUARTERLY; behavior is "3 periods" for other grains)
//   - YoY → 12 months for MONTHLY, 4 quarters for QUARTERLY, 1 year for YEARLY, 365 days for DAILY
//   - YTD → returns t itself (YTD compares year-to-date, not a shifted point)
//   - R12 → returns t shifted back 12 months (rolling 12-month window start)
//   - None → returns t unchanged
//
// Returns a zero time only if t is zero.
func ShiftPeriod(t time.Time, mode CompareMode, grain PeriodGrain) time.Time {
	if t.IsZero() {
		return t
	}
	switch mode {
	case CompareNone, CompareYTD:
		return t
	case CompareMoM:
		return shiftOnePeriod(t, grain)
	case CompareQoQ:
		return shiftThreePeriods(t, grain)
	case CompareYoY:
		return shiftYear(t, grain)
	case CompareR12:
		return t.AddDate(-1, 0, 0)
	}
	return t
}

// shiftOnePeriod shifts back exactly one period of the given grain.
func shiftOnePeriod(t time.Time, grain PeriodGrain) time.Time {
	switch grain {
	case GrainDaily:
		return t.AddDate(0, 0, -1)
	case GrainMonthly:
		return t.AddDate(0, -1, 0)
	case GrainQuarterly:
		return t.AddDate(0, -3, 0)
	case GrainYearly:
		return t.AddDate(-1, 0, 0)
	}
	return t.AddDate(0, -1, 0)
}

// shiftThreePeriods shifts back 3 units (one quarter for monthly grain).
func shiftThreePeriods(t time.Time, grain PeriodGrain) time.Time {
	switch grain {
	case GrainDaily:
		return t.AddDate(0, 0, -3)
	case GrainMonthly:
		return t.AddDate(0, -3, 0)
	case GrainQuarterly:
		return t.AddDate(0, -9, 0)
	case GrainYearly:
		return t.AddDate(-3, 0, 0)
	}
	return t.AddDate(0, -3, 0)
}

// shiftYear shifts back one year's worth of periods.
func shiftYear(t time.Time, grain PeriodGrain) time.Time {
	switch grain {
	case GrainDaily:
		return t.AddDate(0, 0, -365)
	case GrainMonthly:
		return t.AddDate(-1, 0, 0)
	case GrainQuarterly:
		return t.AddDate(-1, 0, 0)
	case GrainYearly:
		return t.AddDate(-1, 0, 0)
	}
	return t.AddDate(-1, 0, 0)
}
